import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup, within } from "@testing-library/svelte";
import MyList from "./MyList.svelte";
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
    copiedText: "",
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
  season: { kind: "birthday", days: 3, label: "birthday" },
  seasonOverride: { value: "auto" },
  prefs: { style: "aurora", events: true, useOwnerTheme: true },
  ALL_STYLES: ["aurora"],
  resolveSeason: () => ({ kind: "birthday", days: 3, label: "birthday" }),
  setSeasonOverride: vi.fn(),
  setTheme: vi.fn(),
}));
vi.mock("../lib/confetti.js", () => ({ confetti: vi.fn() }));
vi.mock("../lib/offlineQueue.js", () => ({
  enqueueItem: vi.fn(),
  isOfflineError: () => false,
  pendingItems: () => [],
  syncPendingItems: () => Promise.resolve(),
  startAutoSync: vi.fn(),
}));
vi.mock("../lib/network.svelte.js", () => ({ network: { offline: false } }));
vi.mock("../lib/metadata.js", () => ({ getMetadata: () => Promise.resolve(null) }));
vi.mock("../lib/listText.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../lib/listText.js")>();
  return { ...actual, copyToClipboard: mocks.copyToClipboard };
});

const ME = {
  id: "u7",
  username: "dave",
  displayName: "Dave Dragonfruit",
  isAdmin: false,
  birthday: "12-25",
};

function item(partial: Partial<Item> & { title: string }): Item {
  return {
    id: "i1",
    url: "",
    notes: "",
    removedAt: null,
    claimed: false,
    claimedByMe: false,
    ...partial,
  } as Item;
}

function response(items: Item[]): UserListResponse {
  return {
    user: {
      id: ME.id,
      username: ME.username,
      displayName: ME.displayName,
      birthday: ME.birthday,
      themeStyle: "aurora",
    },
    items,
    isOwner: true,
  };
}

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.copyToClipboard.mockReset();
  mocks.copyToClipboard.mockImplementation((text: string) => {
    mocks.copiedText = text;
    return Promise.resolve();
  });
  mocks.userStore.set(ME);
});

afterEach(() => {
  cleanup();
  mocks.userStore.set(null);
});

describe("MyList copy to clipboard", () => {
  it("copies the list as the author: keeps claims, drops archived, URL over name", async () => {
    mocks.api.mockImplementation(() =>
      Promise.resolve(
        response([
          item({ title: "Mug", url: "" }),
          item({
            id: "i2",
            title: "Kettle",
            url: "https://example.com/kettle",
            notes: "Shiny one",
          }),
          item({ id: "i3", title: "Taken", claimed: true }),
          item({ id: "i4", title: "Archived", removedAt: "2026-01-01" }),
        ]),
      ),
    );
    render(MyList);
    await flush();

    fireEvent.click(screen.getByRole("button", { name: /Copy list/ }));

    expect(mocks.copyToClipboard).toHaveBeenCalledWith(
      "Dave Dragonfruit's birthday list:\n\n" +
        "- Mug\n\n\n" +
        "- https://example.com/kettle\n\nNotes: Shiny one\n\n\n" +
        "- Taken",
    );
  });

  it("shows no text inside the copy button after copying", async () => {
    mocks.api.mockImplementation(() => Promise.resolve(response([item({ title: "Mug" })])));
    render(MyList);
    await flush();

    const btn = screen.getByRole("button", { name: /Copy list/ });
    fireEvent.click(btn);
    await flush();

    expect(btn.textContent?.trim()).toBe("");
    expect(btn.getAttribute("aria-label")).toBe("Copy list to clipboard");
  });

  it("copies just the heading when every item is archived", async () => {
    mocks.api.mockImplementation(() =>
      Promise.resolve(response([item({ title: "Archived", removedAt: "2026-01-01" })])),
    );
    render(MyList);
    await flush();

    fireEvent.click(screen.getByRole("button", { name: /Copy list/ }));
    expect(mocks.copyToClipboard).toHaveBeenCalledWith("Dave Dragonfruit's birthday list:");
  });
});

describe("MyList first-run birthday prompt", () => {
  function birthDialog(container: HTMLElement): HTMLDialogElement {
    const dlg = Array.from(container.querySelectorAll("dialog")).find(
      (d) => (d as HTMLDialogElement).open,
    ) as HTMLDialogElement;
    if (!dlg) throw new Error("expected an open birthday dialog");
    return dlg;
  }

  it("does not show the modal for existing users", async () => {
    mocks.api.mockImplementation(() => Promise.resolve(response([item({ title: "Mug" })])));
    render(MyList);
    await flush();
    expect(screen.queryByText("What’s your birthday?")).toBeNull();
  });

  it("shows the modal when the account is new (isNew) and lands on my list", async () => {
    mocks.userStore.set({ ...ME, isNew: true, birthday: "" });
    mocks.api.mockImplementation(() => Promise.resolve(response([item({ title: "Mug" })])));
    const { container } = render(MyList);
    await flush();

    const dlg = birthDialog(container);
    expect(within(dlg).getByText("What’s your birthday?")).toBeTruthy();

    const input = within(dlg).getByLabelText("Birthday") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "05-15" } });
    fireEvent.submit(dlg.querySelector("form") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/profile", {
      method: "PATCH",
      body: { birthday: "05-15" },
    });
  });
});
