import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import UserList from "./UserList.svelte";
import type { Item, UserListResponse } from "../lib/types.js";

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
  return {
    api: vi.fn(),
    userStore: store(null),
    loadedStore: store(false),
    copyToClipboard: vi.fn<(text: string) => Promise<void>>(() => Promise.resolve()),
    seasonResult: { kind: "none", days: 0, label: "" },
  };
});

vi.mock("../lib/api.js", () => ({ api: mocks.api }));
vi.mock("../lib/auth.js", () => ({
  user: mocks.userStore,
  loaded: mocks.loadedStore,
  openSignIn: vi.fn(),
  signOut: vi.fn(),
}));
vi.mock("../lib/theme.svelte.js", () => ({
  theme: { choice: "system" },
  season: { kind: "none", days: 0, label: "" },
  seasonOverride: { value: "auto" },
  prefs: { style: "aurora", events: true, useOwnerTheme: true },
  ownerStyleOverride: { value: null },
  ALL_STYLES: ["aurora"],
  resolveSeason: () => mocks.seasonResult,
  setSeasonOverride: vi.fn(),
  setTheme: vi.fn(),
  apply: vi.fn(),
}));
vi.mock("../lib/listText.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../lib/listText.js")>();
  return { ...actual, copyToClipboard: mocks.copyToClipboard };
});

const OWNER = {
  id: "u7",
  username: "dave",
  displayName: "Dave Dragonfruit",
  birthday: "12-25",
  themeStyle: "aurora",
};

function item(revealed: boolean): Item {
  return {
    id: "i1",
    title: "Mug",
    url: "",
    notes: "",
    removedAt: null,
    claimed: true,
    claimedByMe: false,
    ...(revealed ? { claimedByName: "Bob Banana" } : {}),
  };
}

function response(revealed: boolean, isOwner = false): UserListResponse {
  return { user: OWNER, items: [item(revealed)], isOwner };
}

const ADA = {
  id: "u100",
  username: "ada",
  displayName: "Ada Root",
  isAdmin: true,
  birthday: "12-25",
};
const PLEB = {
  id: "u101",
  username: "pam",
  displayName: "Pam Peach",
  isAdmin: false,
  birthday: "",
};

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.copyToClipboard.mockReset();
  mocks.seasonResult = { kind: "none", days: 0, label: "" };
  mocks.userStore.set(ADA);
});

afterEach(() => {
  cleanup();
  mocks.userStore.set(null);
});

describe("UserList admin reveal-claimers", () => {
  it("loads without revealing claimers for admins by default", async () => {
    mocks.api.mockImplementation((path: string) =>
      Promise.resolve(response(path.includes("revealClaimers=1"))),
    );
    const { container } = render(UserList, { userId: "u7" });
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/users/u7/list");
    expect(within(container, "View who claimed items")).not.toBeNull();
    // Claimer name not exposed until the admin opts in.
    expect(screen.queryByText(/claimed by/)).toBeNull();
  });

  it("opts in via the toggle, re-fetches with revealClaimers=1 and shows the claimer", async () => {
    mocks.api.mockImplementation((path: string) =>
      Promise.resolve(response(path.includes("revealClaimers=1"))),
    );
    const { container } = render(UserList, { userId: "u7" });
    await flush();

    fireEvent.click(screen.getByRole("button", { name: "View who claimed items" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/users/u7/list?revealClaimers=1");
    expect(screen.getByRole("button", { name: "Hide who claimed items" })).toBeTruthy();
    expect(within(container, "claimed by Bob Banana")).not.toBeNull();
  });

  it("toggles back off and hides the claimers again", async () => {
    mocks.api.mockImplementation((path: string) =>
      Promise.resolve(response(path.includes("revealClaimers=1"))),
    );
    const { container } = render(UserList, { userId: "u7" });
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "View who claimed items" }));
    await flush();
    expect(within(container, "claimed by Bob Banana")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Hide who claimed items" }));
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/users/u7/list");
    expect(screen.queryByText(/claimed by/)).toBeNull();
    expect(screen.queryByRole("button", { name: "View who claimed items" })).not.toBeNull();
  });

  it("has no toggle and no claimer names for a non-admin viewer", async () => {
    mocks.userStore.set(PLEB);
    mocks.api.mockImplementation((path: string) =>
      Promise.resolve(response(path.includes("revealClaimers=1"))),
    );
    render(UserList, { userId: "u7" });
    await flush();

    expect(screen.queryByRole("button", { name: /who claimed items/ })).toBeNull();
    expect(screen.queryByText(/claimed by/)).toBeNull();
  });

  it("hides the toggle when viewing your own list", async () => {
    mocks.api.mockImplementation(() => Promise.resolve(response(false, true)));
    const { container } = render(UserList, { userId: "u7" });
    await flush();
    expect(screen.queryByRole("button", { name: /who claimed items/ })).toBeNull();
    expect(container.querySelector("button.reveal-toggle")).toBeNull();
  });
});

function within(container: HTMLElement, text: string): HTMLElement | null {
  const all = container.querySelectorAll("*");
  for (const el of all) {
    if (el.textContent?.trim() === text) return el as HTMLElement;
  }
  return null;
}

function mkItem(partial: Partial<Item>): Item {
  return {
    id: "i1",
    title: "Mug",
    url: "",
    notes: "",
    removedAt: null,
    claimed: false,
    claimedByMe: false,
    ...partial,
  } as Item;
}

describe("UserList copy to clipboard", () => {
  it("omits claimed and archived items when the viewer is not the author", async () => {
    mocks.api.mockImplementation(() =>
      Promise.resolve({
        user: OWNER,
        items: [
          mkItem({ id: "i1", title: "Free mug" }),
          mkItem({ id: "i2", title: "Taken mug", claimed: true, claimedByMe: true }),
          mkItem({ id: "i3", title: "Archived mug", removedAt: "2026-01-01" }),
        ],
        isOwner: false,
      }),
    );
    render(UserList, { userId: "u7" });
    await flush();

    fireEvent.click(screen.getByRole("button", { name: /Copy list/ }));
    expect(mocks.copyToClipboard).toHaveBeenCalledWith("Dave Dragonfruit's list:\n\n- Free mug");
  });

  it("keeps claimed items but drops archived items when the viewer is the author", async () => {
    mocks.api.mockImplementation(() =>
      Promise.resolve({
        user: OWNER,
        items: [
          mkItem({ id: "i1", title: "Mug", claimed: true, claimedByName: "Bob Banana" }),
          mkItem({ id: "i2", title: "Archived", removedAt: "2026-01-01" }),
        ],
        isOwner: true,
      }),
    );
    render(UserList, { userId: "u7" });
    await flush();

    fireEvent.click(screen.getByRole("button", { name: /Copy list/ }));
    expect(mocks.copyToClipboard).toHaveBeenCalledWith("Dave Dragonfruit's list:\n\n- Mug");
  });

  it("uses the owner season label as the event in the heading", async () => {
    mocks.seasonResult = { kind: "birthday", days: 3, label: "birthday" };
    mocks.api.mockImplementation(() =>
      Promise.resolve({
        user: OWNER,
        items: [mkItem({ id: "i1", title: "Mug" })],
        isOwner: false,
      }),
    );
    render(UserList, { userId: "u7" });
    await flush();

    fireEvent.click(screen.getByRole("button", { name: /Copy list/ }));
    const text = mocks.copyToClipboard.mock.calls[0]?.[0] ?? "";
    expect(text).toMatch(/^Dave Dragonfruit's birthday list:\n\n- Mug$/);
  });

  it("shows no text inside the copy button after copying", async () => {
    mocks.api.mockImplementation(() => Promise.resolve(response(false)));
    render(UserList, { userId: "u7" });
    await flush();

    const btn = screen.getByRole("button", { name: /Copy list/ });
    fireEvent.click(btn);
    await flush();
    expect(btn.textContent?.trim()).toBe("");
    expect(btn.getAttribute("aria-label")).toBe("Copy list to clipboard");
  });

  it("shows an error when the clipboard write fails", async () => {
    mocks.copyToClipboard.mockRejectedValue(new Error("nope"));
    mocks.api.mockImplementation(() => Promise.resolve(response(false)));
    render(UserList, { userId: "u7" });
    await flush();

    fireEvent.click(screen.getByRole("button", { name: /Copy list/ }));
    await flush();
    expect(screen.queryByText("nope")).not.toBeNull();
  });
});
