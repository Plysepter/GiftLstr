import { api } from "./api.js";

export type PendingKind = "own" | "private";

export interface PendingItem {
  localId: string;
  kind: PendingKind;
  /** Set when kind === "private": the list owner's user id. */
  ownerId?: string;
  title: string;
  url: string;
  notes: string;
  createdAt: number;
}

const STORAGE_KEY = "pending_items";

function read(): PendingItem[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    // Items queued before the off-line sync split have no kind; they are own-list ideas.
    return parsed.map((i) => (i && typeof i.kind === "undefined" ? { ...i, kind: "own" } : i));
  } catch {
    return [];
  }
}

function write(items: PendingItem[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
  } catch {
    // Storage full/unavailable; nothing else we can do.
  }
}

export function enqueueItem(title: string, url: string, notes: string): PendingItem {
  const item: PendingItem = {
    localId: `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    kind: "own",
    title,
    url,
    notes,
    createdAt: Date.now(),
  };
  write([...read(), item]);
  return item;
}

/** Queues a private idea for someone else's list, created while offline. */
export function enqueuePrivateItem(
  ownerId: string,
  title: string,
  url: string,
  notes: string,
): PendingItem {
  const item: PendingItem = {
    localId: `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    kind: "private",
    ownerId,
    title,
    url,
    notes,
    createdAt: Date.now(),
  };
  write([...read(), item]);
  return item;
}

export function pendingItems(): PendingItem[] {
  return read();
}

/** Pending items shown on a given list: own-list ideas, or the viewer's
 * private ideas for one specific owner. */
export function pendingForList(kind: PendingKind, ownerId?: string): PendingItem[] {
  return read().filter((i) => i.kind === kind && (ownerId === undefined || i.ownerId === ownerId));
}

/** True when the error looks like a connectivity failure rather than an API rejection. */
export function isOfflineError(err: unknown): boolean {
  if (typeof navigator !== "undefined" && navigator.onLine === false) return true;
  return err instanceof TypeError; // fetch network failures surface as TypeError
}

/** The endpoint an offline item pushes to once the device is back online. */
function syncPath(item: PendingItem): string {
  return item.kind === "private" && item.ownerId
    ? `/api/users/${item.ownerId}/private-items`
    : "/api/my/items";
}

let inFlight: Promise<number> | null = null;

/**
 * Attempts to push every queued item to the server. Successfully synced items
 * are removed from the queue; failures stay for the next attempt. Returns the
 * number of items still pending. Concurrent callers share one run, so a
 * reconnect never POSTs the same item twice.
 */
export async function syncPendingItems(): Promise<number> {
  if (inFlight) return inFlight;
  const run = (async () => {
    const queue = read();
    const remaining: PendingItem[] = [];
    for (const item of queue) {
      try {
        await api(syncPath(item), {
          method: "POST",
          body: { title: item.title, url: item.url, notes: item.notes },
        });
      } catch (err) {
        // A 4xx means the server rejected the item permanently (e.g. bad
        // URL after validation tightened); drop it instead of retrying
        // forever. Network errors stay queued.
        if (err instanceof Error && "status" in err && typeof err.status === "number") {
          continue;
        }
        remaining.push(item);
      }
    }
    write(remaining);
    return remaining.length;
  })().finally(() => {
    inFlight = null;
  });
  inFlight = run;
  return run;
}

/** Removes all queued items (used on sign-out). */
export function clearQueue(): void {
  write([]);
}

let started = false;

/** Retries the queue on connectivity changes and periodically while online. */
export function startAutoSync(): void {
  if (started || typeof window === "undefined") return;
  started = true;
  window.addEventListener("online", () => void syncPendingItems());
  window.addEventListener("load", () => void syncPendingItems());
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") void syncPendingItems();
  });
  void syncPendingItems();
}
