import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import PrivateList from "./PrivateList.svelte";
import type { Item, PrivateListResponse } from "../lib/types.js";
// The real network module is a tiny $state box — keeping it (rather than a
// plain-object mock) means toggling `offline` re-renders the view in tests.
import { network } from "../lib/network.svelte.js";

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
    isOfflineError: vi.fn(() => false),
    enqueueItem: vi.fn(),
    copyToClipboard: vi.fn<(text: string) => Promise<void>>(),
    apply: vi.fn(),
    ownerStyleOverride: { value: null },
    ALL_STYLES: ["aurora", "forest", "lagoon"],
  };
});

vi.mock("../lib/api.js", () => ({ api: mocks.api }));
vi.mock("../lib/auth.js", () => ({
  user: mocks.userStore,
  loaded: { subscribe: () => () => {} },
  openSignIn: vi.fn(),
  signOut: vi.fn(),
}));
vi.mock("../lib/theme.svelte.js", () => ({
  theme: { choice: "system" },
  season: { kind: "birthday", days: 3, label: "birthday" },
  seasonOverride: { value: "auto" },
  prefs: { style: "aurora", events: true, useOwnerTheme: true, contrast: false },
  ALL_STYLES: mocks.ALL_STYLES,
  ownerStyleOverride: mocks.ownerStyleOverride,
  resolveSeason: () => ({ kind: "birthday", days: 3, label: "birthday" }),
  setSeasonOverride: vi.fn(),
  setTheme: vi.fn(),
  apply: mocks.apply,
}));
vi.mock("../lib/confetti.js", () => ({ confetti: vi.fn() }));
vi.mock("../lib/offlineQueue.js", () => ({
  isOfflineError: mocks.isOfflineError,
  enqueueItem: mocks.enqueueItem,
}));
vi.mock("../lib/metadata.js", () => ({ getMetadata: () => Promise.resolve(null) }));
vi.mock("../lib/listText.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../lib/listText.js")>();
  return { ...actual, copyToClipboard: mocks.copyToClipboard };
});

const LIST_ID = "L1";

const ME = {
  id: "u9",
  username: "me",
  displayName: "Me Myself",
  isAdmin: false,
  birthday: "",
  spendingEnabled: true,
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

function response(items: Item[], extra: Partial<PrivateListResponse> = {}): PrivateListResponse {
  return {
    user: {
      id: LIST_ID,
      username: "",
      displayName: "Carol Cherry",
      birthday: "05-15",
      themeStyle: "aurora",
      holidayReset: "",
    },
    items,
    isOwner: true,
    ...extra,
  };
}

/** Mocks the list GET and lets everything else resolve as an empty success. */
function mockList(items: Item[], extra: Partial<PrivateListResponse> = {}): void {
  mocks.api.mockImplementation((path: string) => {
    if (path.startsWith(`/api/private-lists/${LIST_ID}`))
      return Promise.resolve(response(items, extra));
    return Promise.resolve({});
  });
}

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.isOfflineError.mockReset();
  mocks.isOfflineError.mockImplementation(() => false);
  mocks.enqueueItem.mockReset();
  mocks.copyToClipboard.mockReset();
  mocks.copyToClipboard.mockResolvedValue(undefined);
  mocks.apply.mockReset();
  mocks.ownerStyleOverride.value = null;
  network.offline = false;
  network.synced = 0;
  mocks.userStore.set(ME);
  window.location.hash = "";
});

afterEach(() => {
  cleanup();
  mocks.userStore.set(null);
});

describe("PrivateList loading", () => {
  it("loads the list and shows its ideas with a private pill", async () => {
    mockList([item({ title: "Mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    expect(mocks.api).toHaveBeenCalledWith(`/api/private-lists/${LIST_ID}`);
    expect(screen.getByText("Mug")).toBeTruthy();
    expect(screen.getByText("Private")).toBeTruthy();
  });

  it("shows the API error when the list cannot be loaded", async () => {
    mocks.api.mockRejectedValue(new Error("private list not found"));
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    expect(screen.getByText("private list not found")).toBeTruthy();
  });
});

describe("PrivateList items", () => {
  it("claims an idea through the claim endpoint", async () => {
    mockList([item({ title: "Mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Claim" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/items/i1/claim", { method: "PUT" });
  });

  it("unclaims an idea the viewer already holds", async () => {
    mockList([item({ title: "Mug", claimed: true, claimedByMe: true })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Unclaim" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/items/i1/claim", { method: "DELETE" });
  });

  it("sends the private price in dollars", async () => {
    mockList([item({ title: "Mug", claimed: true, claimedByMe: true, mySpendCents: 1250 })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    const input = screen.getByLabelText("Price for Mug") as HTMLInputElement;
    await fireEvent.change(input, { target: { value: "12.50" } });
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/items/i1/price", {
      method: "PATCH",
      body: { price: 12.5 },
    });
  });

  it("adds an idea carrying this list's id", async () => {
    mockList([item({ title: "Mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    const input = screen.getByLabelText("Add gift idea") as HTMLInputElement;
    await fireEvent.input(input, { target: { value: "Socks" } });
    await fireEvent.submit(input.closest("form") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/my/items", {
      method: "POST",
      body: { title: "Socks", url: "", notes: "", listId: LIST_ID },
    });
  });

  it("edits an idea through the my-items endpoint", async () => {
    mockList([item({ title: "Mug" })]);
    const { container } = render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Edit Mug" }));
    await flush();
    const editDlg = container.querySelector("dialog.edit-dlg") as HTMLDialogElement;
    expect(editDlg.open).toBe(true);

    await fireEvent.input(container.querySelector("#eName") as HTMLInputElement, {
      target: { value: "Mug 2" },
    });
    await fireEvent.submit(editDlg.querySelector("form") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/my/items/i1", {
      method: "PATCH",
      body: { title: "Mug 2", url: "", notes: "" },
    });
  });

  it("removes an idea and restores it from the removed section", async () => {
    mockList([item({ title: "Mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Remove Mug" }));
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/my/items/i1", { method: "DELETE" });
    expect(screen.getByText(/1 removed/)).toBeTruthy();

    await fireEvent.click(screen.getByRole("button", { name: "Restore" }));
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/my/items/i1/restore", { method: "POST" });
  });

  it("reorders with the grip's arrow keys, scoped to this list", async () => {
    mockList([item({ title: "A" }), item({ id: "i2", title: "B" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    const grips = screen.getAllByRole("button", { name: /Reorder/ });
    await fireEvent.keyDown(grips[0]!, { key: "ArrowDown" });
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/my/items/order", {
      method: "PUT",
      body: { ids: ["i2", "i1"], listId: LIST_ID },
    });
  });

  it("flags offline instead of queueing when an add fails offline", async () => {
    mocks.api.mockImplementation((path: string) => {
      if (path.startsWith(`/api/private-lists/${LIST_ID}`))
        return Promise.resolve(response([item({ title: "Mug" })]));
      return Promise.reject(new Error("Failed to fetch"));
    });
    mocks.isOfflineError.mockImplementationOnce(() => true);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    const input = screen.getByLabelText("Add gift idea") as HTMLInputElement;
    await fireEvent.input(input, { target: { value: "Socks" } });
    await fireEvent.submit(input.closest("form") as HTMLFormElement);
    await flush();

    expect(network.offline).toBe(true);
    expect(mocks.enqueueItem).not.toHaveBeenCalled();
    expect(screen.getByTestId("offline-state")).toBeTruthy();
    expect(input.value).toBe("Socks");
  });
});

describe("PrivateList theme", () => {
  it("applies the list's own theme palette", async () => {
    mockList([item({ title: "Mug" })], { user: { ...response([]).user, themeStyle: "forest" } });
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    expect(mocks.ownerStyleOverride.value).toBe("forest");
    expect(mocks.apply).toHaveBeenCalled();
  });

  it("resets the theme override when leaving the page", async () => {
    mockList([item({ title: "Mug" })], { user: { ...response([]).user, themeStyle: "lagoon" } });
    const { unmount } = render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    expect(mocks.ownerStyleOverride.value).toBe("lagoon");
    unmount();
    expect(mocks.ownerStyleOverride.value).toBeNull();
  });
});

describe("PrivateList copy list", () => {
  it("copies the active items as clipboard text for the owner", async () => {
    mockList([item({ title: "Mug", url: "https://example.com/mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Copy list to clipboard" }));
    await flush();

    expect(mocks.copyToClipboard).toHaveBeenCalledTimes(1);
    const text = mocks.copyToClipboard.mock.calls[0]![0] as string;
    expect(text).toContain("Carol Cherry's birthday list:");
    expect(text).toContain("https://example.com/mug");
  });

  it("surfaces the error when the clipboard write fails", async () => {
    mockList([item({ title: "Mug" })]);
    mocks.copyToClipboard.mockRejectedValue(new Error("Could not copy to clipboard"));
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Copy list to clipboard" }));
    await flush();

    expect(screen.getByText("Could not copy to clipboard")).toBeTruthy();
  });
});

describe("PrivateList edit list", () => {
  function listDlg(container: HTMLElement): HTMLDialogElement {
    return container.querySelector("dialog.pl-dlg") as HTMLDialogElement;
  }

  it("prefills and PATCHes names, birthday and theme", async () => {
    mockList([item({ title: "Mug" })]);
    const { container } = render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Edit list" }));
    await flush();

    const dlg = listDlg(container);
    expect(dlg.open).toBe(true);
    const first = screen.getByLabelText("First name") as HTMLInputElement;
    const last = screen.getByLabelText("Last name") as HTMLInputElement;
    expect(first.value).toBe("Carol");
    expect(last.value).toBe("Cherry");

    await fireEvent.input(first, { target: { value: "Carol Anne" } });
    await fireEvent.click(screen.getByRole("button", { name: "Forest" }));
    await fireEvent.submit(listDlg(container).querySelector("form") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith(`/api/private-lists/${LIST_ID}`, {
      method: "PATCH",
      body: {
        firstName: "Carol Anne",
        lastName: "Cherry",
        birthday: "05-15",
        theme: "forest",
      },
    });
  });

  it("surfaces the edit error and keeps the dialog open", async () => {
    mocks.api.mockImplementation((path: string, opts?: { method?: string }) => {
      if (path === `/api/private-lists/${LIST_ID}` && (!opts?.method || opts.method === "GET"))
        return Promise.resolve(response([item({ title: "Mug" })]));
      return Promise.reject(new Error("update failed"));
    });
    const { container } = render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Edit list" }));
    await flush();

    const dlg = listDlg(container);
    await fireEvent.submit(dlg.querySelector("form") as HTMLFormElement);
    await flush();

    expect(screen.getByText("update failed")).toBeTruthy();
    expect(dlg.open).toBe(true);
  });
});

describe("PrivateList delete", () => {
  it("hard-deletes the list after the confirmation dialog", async () => {
    mockList([item({ title: "Mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Delete Carol" }));
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Delete list" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith(`/api/private-lists/${LIST_ID}`, { method: "DELETE" });
    expect(window.location.hash).toBe("#/lists");
  });

  it("keeps the list when the confirmation is cancelled", async () => {
    mockList([item({ title: "Mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Delete Carol" }));
    await flush();
    await fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await flush();

    expect(mocks.api).not.toHaveBeenCalledWith(`/api/private-lists/${LIST_ID}`, {
      method: "DELETE",
    });
    expect(window.location.hash).not.toBe("#/lists");
  });
});

describe("PrivateList archive", () => {
  it("archives the list after the confirmation dialog and opens the fresh one", async () => {
    mocks.api.mockImplementation((path: string, opts?: { method?: string }) => {
      if (path === `/api/private-lists/${LIST_ID}` && (!opts?.method || opts.method === "GET"))
        return Promise.resolve(response([item({ title: "Mug" })]));
      if (path === `/api/private-lists/${LIST_ID}/archive`) return Promise.resolve({ id: "L2" });
      return Promise.resolve({});
    });
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Archive Carol" }));
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Archive list" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith(`/api/private-lists/${LIST_ID}/archive`, {
      method: "POST",
    });
    expect(window.location.hash).toBe("#/private-list/L2");
  });

  it("keeps the list when archiving is cancelled", async () => {
    mockList([item({ title: "Mug" })]);
    render(PrivateList, { props: { listId: LIST_ID } });
    await flush();

    await fireEvent.click(screen.getByRole("button", { name: "Archive Carol" }));
    await flush();
    await fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await flush();

    expect(mocks.api).not.toHaveBeenCalledWith(`/api/private-lists/${LIST_ID}/archive`, {
      method: "POST",
    });
  });
});
