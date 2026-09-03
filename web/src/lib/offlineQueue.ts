import { api } from "./api.js";

export interface PendingItem {
  localId: string;
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
    return Array.isArray(parsed) ? parsed : [];
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

/** True when the error looks like a connectivity failure rather than an API rejection. */
export function isOfflineError(err: unknown): boolean {
  if (typeof navigator !== "undefined" && navigator.onLine === false) return true;
  return err instanceof TypeError; // fetch network failures surface as TypeError
}

/**
 * Attempts to push every queued item to the server. Successfully synced items
 * are removed from the queue; failures stay for the next attempt. Returns the
 * number of items still pending.
 */
export async function syncPendingItems(): Promise<number> {
  const queue = read();
  const remaining: PendingItem[] = [];
  for (const item of queue) {
    try {
      await api("/api/my/items", {
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
