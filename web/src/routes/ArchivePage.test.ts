import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import ArchivePage from "./ArchivePage.svelte";
import type { ArchiveIndexResponse } from "../lib/types.js";

const mocks = vi.hoisted(() => {
  type Cb = (v: unknown) => void;
  function store(init: unknown) {
    let value: unknown = init;
    const subs = new Set<Cb>();
    return {
      subscribe(cb: Cb): () => void {
        cb(value);
        subs.add(cb);
        return () => {
          subs.delete(cb);
        };
      },
      set(next: unknown): void {
        value = next;
        for (const cb of subs) cb(value);
      },
      update(fn: (v: unknown) => unknown): void {
        this.set(fn(value));
      },
    };
  }
  return { api: vi.fn(), userStore: store(null) };
});

vi.mock("../lib/api.js", () => ({ api: mocks.api }));
vi.mock("../lib/auth.js", () => ({
  user: mocks.userStore,
  loaded: { subscribe: () => () => {} },
  openSignIn: vi.fn(),
  signOut: vi.fn(),
}));
vi.mock("../lib/offlineQueue.js", () => ({ isOfflineError: () => false }));
vi.mock("../lib/network.svelte.js", () => ({ network: { offline: false, synced: 0 } }));
vi.mock("../lib/theme.svelte.js", () => ({
  theme: { choice: "system" },
  season: { kind: "none", days: 0, label: "" },
  seasonOverride: { value: "auto" },
  prefs: { style: "aurora", events: true, useOwnerTheme: true },
  ownerStyleOverride: { value: null },
  ALL_STYLES: ["aurora"],
  resolveSeason: () => ({ kind: "none", days: 0, label: "" }),
  setSeasonOverride: vi.fn(),
  setTheme: vi.fn(),
  apply: vi.fn(),
}));

const ARCHIVED: ArchiveIndexResponse = {
  lists: [
    { id: "a1", label: "2026 Birthday", archivedAt: "2026-07-09", itemCount: 3, claimedCount: 1 },
  ],
  page: 1,
  hasMore: false,
  total: 1,
};

const PRIVATE: ArchiveIndexResponse = {
  lists: [
    {
      id: "p1",
      label: "2026 Birthday",
      archivedAt: "2026-07-09",
      itemCount: 2,
      claimedCount: 0,
      firstName: "Robin",
      lastName: "Reed",
      birthday: "07-01",
      theme: "aurora",
    },
  ],
  page: 1,
  hasMore: false,
  total: 1,
};

function empty(): ArchiveIndexResponse {
  return { lists: [], page: 1, hasMore: false, total: 0 };
}

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

beforeEach(() => {
  mocks.api.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("ArchivePage tabs", () => {
  it("defaults to the archived tab", async () => {
    mocks.api.mockImplementation((path: string) =>
      Promise.resolve(path.includes("type=private") ? PRIVATE : ARCHIVED),
    );
    render(ArchivePage);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/archive?page=1");
    expect(screen.getByText("2026 Birthday")).toBeTruthy();
    expect(screen.queryByText("Robin Reed")).toBeNull();
  });

  it("loads rolled-over private lists on the private tab", async () => {
    mocks.api.mockImplementation((path: string) =>
      Promise.resolve(path.includes("type=private") ? PRIVATE : ARCHIVED),
    );
    render(ArchivePage);
    await flush();

    await fireEvent.click(screen.getByRole("tab", { name: "Private lists" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/archive?page=1&type=private");
    expect(screen.getByText("Robin Reed")).toBeTruthy();
    expect(screen.getByText(/2026 Birthday/)).toBeTruthy();
  });

  it("shows a private-specific empty state", async () => {
    mocks.api.mockImplementation((path: string) =>
      Promise.resolve(path.includes("type=private") ? empty() : ARCHIVED),
    );
    render(ArchivePage);
    await flush();

    await fireEvent.click(screen.getByRole("tab", { name: "Private lists" }));
    await flush();

    expect(screen.getByText("No private lists rolled over yet")).toBeTruthy();
  });
});
