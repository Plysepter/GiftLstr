import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup, within } from "@testing-library/svelte";
import Admin from "./Admin.svelte";
import type { AdminUser, AdminUsersResponse } from "../lib/types.js";

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
    setSeasonOverride: vi.fn(),
    userStore: store(null),
    loadedStore: store(false),
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
  setSeasonOverride: mocks.setSeasonOverride,
  setTheme: vi.fn(),
}));

const ALICE: AdminUser = {
  id: "u1",
  username: "alice",
  displayName: "Alice Apple",
  isAdmin: true,
  birthday: "01-01",
  itemCount: 3,
  claimedCount: 1,
  connections: ["u2"],
  connectedBy: [],
};
const BOB: AdminUser = {
  id: "u2",
  username: "bob",
  displayName: "Bob Banana",
  isAdmin: false,
  birthday: "",
  itemCount: 0,
  claimedCount: 0,
  connections: ["u1"],
  connectedBy: [],
};
const CAROL: AdminUser = {
  id: "u3",
  username: "carol",
  displayName: "Carol Cherry",
  isAdmin: false,
  birthday: "",
  itemCount: 1,
  claimedCount: 0,
  connections: [],
  connectedBy: [],
};

const ADA = {
  id: "u100",
  username: "ada",
  displayName: "Ada Root",
  isAdmin: true,
  birthday: "12-25",
};

function pageResponse(users: AdminUser[], total = users.length): AdminUsersResponse {
  return { users, page: 1, hasMore: false, total, names: {} };
}

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

/** The currently open <dialog> element (admin UI mounts both dialogs). */
function openDialog(container: HTMLElement): HTMLElement {
  const dlg = Array.from(container.querySelectorAll("dialog")).find(
    (d) => (d as HTMLDialogElement).open,
  );
  if (!dlg) throw new Error("expected an open dialog");
  return dlg;
}

function card(container: HTMLElement, displayName: string): HTMLElement {
  return (
    (within(container).getByText(displayName).closest(".card") as HTMLElement) ??
    (() => {
      throw new Error("card not found");
    })()
  );
}

function openMenu(container: HTMLElement, displayName: string): void {
  fireEvent.click(
    within(card(container, displayName)).getByRole("button", {
      name: `Options for ${displayName}`,
    }),
  );
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.api.mockImplementation((path: string) => {
    if (path.startsWith("/api/admin/users?q=")) {
      return Promise.resolve(pageResponse([CAROL]));
    }
    return Promise.resolve(pageResponse([ALICE, BOB]));
  });
  mocks.userStore.set(ADA);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  mocks.userStore.set(null);
});

describe("Admin access guard", () => {
  it("shows the guard instead of the page for non-admins", async () => {
    mocks.userStore.set({ ...ADA, isAdmin: false });
    const { container } = render(Admin);
    await flush();
    expect(within(container).getByText("Admin access required.")).toBeTruthy();
    expect(within(container).queryByText("Event theme")).toBeNull();
  });
});

describe("Admin user cards", () => {
  it("lists users with management controls (manage/archive/kebab)", async () => {
    const { container } = render(Admin);
    await flush();
    const alice = card(container, "Alice Apple");
    expect(within(alice).getByText("@alice")).toBeTruthy();
    expect(
      (within(alice).getByRole("button", { name: "Manage connections" }) as HTMLButtonElement)
        .className,
    ).toContain("secondary");
    expect(within(alice).getByRole("button", { name: "Archive current list" })).toBeTruthy();
    expect(within(alice).getByRole("button", { name: "Options for Alice Apple" })).toBeTruthy();
  });

  it("shows an admin badge for admins and revoke vs make labels", async () => {
    const { container } = render(Admin);
    await flush();
    const alice = card(container, "Alice Apple");
    expect(within(alice).getByText("admin")).toBeTruthy();

    openMenu(container, "Alice Apple");
    await flush();
    expect(within(alice).getByRole("menuitem", { name: "Revoke admin" })).toBeTruthy();

    fireEvent.click(document.body);
    await flush();
    const bob = card(container, "Bob Banana");
    openMenu(container, "Bob Banana");
    await flush();
    expect(within(bob).getByRole("menuitem", { name: "Make admin" })).toBeTruthy();
  });

  it("closes the kebab menu on an outside click", async () => {
    const { container } = render(Admin);
    await flush();
    openMenu(container, "Alice Apple");
    await flush();
    expect(screen.getByRole("menu", { name: "Options for Alice Apple" })).toBeTruthy();
    fireEvent.click(document.body);
    await flush();
    expect(screen.queryByRole("menu", { name: "Options for Alice Apple" })).toBeNull();
  });
});

describe("Admin confirmable actions", () => {
  it("revoke admin confirms then PATCHes isAdmin=false", async () => {
    const { container } = render(Admin);
    await flush();
    openMenu(container, "Alice Apple");
    await flush();
    const alice = card(container, "Alice Apple");
    fireEvent.click(within(alice).getByRole("menuitem", { name: "Revoke admin" }));
    await flush();

    const dlg = openDialog(container);
    expect((dlg as HTMLDialogElement).open).toBe(true);
    expect(within(dlg).getByText(/Revoke administrator access from Alice Apple/)).toBeTruthy();
    fireEvent.click(within(dlg).getByRole("button", { name: "Revoke admin" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/admin/users/u1", {
      method: "PATCH",
      body: { isAdmin: false },
    });
    // load() refreshes the user list after the change.
    expect(mocks.api).toHaveBeenCalledWith("/api/admin/users?page=1");
  });

  it("make admin confirms then PATCHes isAdmin=true", async () => {
    const { container } = render(Admin);
    await flush();
    openMenu(container, "Bob Banana");
    await flush();
    const bob = card(container, "Bob Banana");
    fireEvent.click(within(bob).getByRole("menuitem", { name: "Make admin" }));
    await flush();

    const dlg = openDialog(container);
    expect(within(dlg).getByText(/Grant administrator access to Bob Banana/)).toBeTruthy();
    fireEvent.click(within(dlg).getByRole("button", { name: "Make admin" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/admin/users/u2", {
      method: "PATCH",
      body: { isAdmin: true },
    });
  });

  it("archive current list confirms then POSTs the archive endpoint", async () => {
    const { container } = render(Admin);
    await flush();
    const alice = card(container, "Alice Apple");
    fireEvent.click(within(alice).getByRole("button", { name: "Archive current list" }));
    await flush();

    const dlg = openDialog(container);
    expect((dlg as HTMLDialogElement).querySelector("h2")?.textContent).toBe(
      "Archive current list?",
    );
    expect(within(dlg).getByText(/Unclaimed items will be copied/)).toBeTruthy();

    // Cancel does nothing.
    fireEvent.click(within(dlg).getByRole("button", { name: "Cancel" }));
    await flush();
    expect(mocks.api.mock.calls.some((c) => c[0] === "/api/admin/users/u1/archive")).toBe(false);

    // Confirm archives.
    fireEvent.click(within(alice).getByRole("button", { name: "Archive current list" }));
    await flush();
    const dlg2 = openDialog(container);
    fireEvent.click(within(dlg2).getByRole("button", { name: "Archive list" }));
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/admin/users/u1/archive", { method: "POST" });
    expect(within(container).getByText("Archived Alice Apple's list.")).toBeTruthy();
  });

  it("delete list confirms then DELETEs the items endpoint", async () => {
    const { container } = render(Admin);
    await flush();
    openMenu(container, "Bob Banana");
    await flush();
    const bob = card(container, "Bob Banana");
    fireEvent.click(within(bob).getByRole("menuitem", { name: "Delete list" }));
    await flush();

    const dlg = openDialog(container);
    expect(within(dlg).getByText(/Permanently delete all items and claims/)).toBeTruthy();
    fireEvent.click(within(dlg).getByRole("button", { name: "Delete list" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/admin/users/u2/list", { method: "DELETE" });
  });

  it("delete user requires typing the username, then DELETEs the user", async () => {
    const { container } = render(Admin);
    await flush();
    openMenu(container, "Bob Banana");
    await flush();
    const bob = card(container, "Bob Banana");
    fireEvent.click(within(bob).getByRole("menuitem", { name: "Delete user" }));
    await flush();

    const dlg = openDialog(container) as HTMLElement;
    const confirmBtn = within(dlg).getByRole("button", {
      name: "Delete user",
    }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
    expect(within(dlg).getByText(/Type the username to confirm/)).toBeTruthy();

    const input = within(dlg).getByLabelText(/to confirm/) as HTMLInputElement;
    fireEvent.input(input, { target: { value: "wrong" } });
    await flush();
    expect(confirmBtn.disabled).toBe(true);
    fireEvent.input(input, { target: { value: "bob" } });
    await flush();
    expect(confirmBtn.disabled).toBe(false);
    fireEvent.click(confirmBtn);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/admin/users/u2", { method: "DELETE" });
    expect(within(container).getByText("Deleted user Bob Banana.")).toBeTruthy();
  });

  it("disables demoting the last admin and deleting yourself", async () => {
    mocks.api.mockImplementation(() => Promise.resolve(pageResponse([ALICE], 1)));
    const { container } = render(Admin);
    await flush();
    openMenu(container, "Alice Apple");
    await flush();
    const alice = card(container, "Alice Apple");
    const revoke = within(alice).getByRole("menuitem", {
      name: "Revoke admin",
    }) as HTMLButtonElement;
    expect(revoke.disabled).toBe(true);
    expect(revoke.title).toBe("Cannot demote the last admin");

    mocks.userStore.set(ALICE);
    await flush();
    const delSelf = within(alice).getByRole("menuitem", {
      name: "Delete user",
    }) as HTMLButtonElement;
    expect(delSelf.disabled).toBe(true);
    expect(delSelf.title).toBe("Cannot delete yourself");
  });
});

describe("Admin connections modal", () => {
  it("opens as a modal containing the add-connection search and suggestions", async () => {
    vi.useFakeTimers();
    const { container } = render(Admin);
    await flush();
    const alice = card(container, "Alice Apple");
    fireEvent.click(within(alice).getByRole("button", { name: "Manage connections" }));
    await flush();

    const dlg = openDialog(container) as HTMLDialogElement;
    expect(dlg.open).toBe(true);
    expect(within(dlg).getByText("Alice Apple · 1 connections")).toBeTruthy();
    expect(within(dlg).getByText("Bob Banana")).toBeTruthy();

    // Search lives inside the modal; typing queries the admin users API.
    const search = within(dlg).getByLabelText("Search to connect Alice Apple") as HTMLInputElement;
    fireEvent.input(search, { target: { value: "carol" } });
    await vi.advanceTimersByTimeAsync(300);
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/admin/users?q=carol&page=1");

    const suggest = within(dlg).getByRole("button", { name: /Carol Cherry/ });
    fireEvent.click(suggest);
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/connections", {
      method: "PUT",
      body: { a: "u1", b: "u3", add: true },
    });
  });

  it("removes a connection through the confirm dialog", async () => {
    const { container } = render(Admin);
    await flush();
    const alice = card(container, "Alice Apple");
    fireEvent.click(within(alice).getByRole("button", { name: "Manage connections" }));
    await flush();
    const dlg = openDialog(container);

    fireEvent.click(within(dlg).getByRole("button", { name: "Remove" }));
    await flush();

    const confirm = Array.from(container.querySelectorAll("dialog")).find(
      (d) => (d as HTMLDialogElement).ariaLabel === "Remove connection?",
    ) as HTMLElement;
    expect(confirm).toBeTruthy();
    expect(within(confirm).getByText(/Remove the connection between/)).toBeTruthy();
    fireEvent.click(within(confirm).getByRole("button", { name: "Remove" }));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/connections", {
      method: "PUT",
      body: { a: "u1", b: "u2", add: false },
    });
  });
});
