import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup, within } from "@testing-library/svelte";
import ListsPage from "./ListsPage.svelte";
import type { ListsPageResponse, ListsPageUser } from "../lib/types.js";

const mocks = vi.hoisted(() => {
  return {
    api: vi.fn(),
    network: { offline: false },
  };
});

vi.mock("../lib/api.js", () => ({ api: mocks.api }));
vi.mock("../lib/offlineQueue.js", () => ({ isOfflineError: () => false }));
vi.mock("../lib/network.svelte.js", () => ({ network: mocks.network }));

const ALICE: ListsPageUser = {
  id: "u1",
  displayName: "Alice Apple",
  username: "alice",
  birthday: "01-01",
  myClaims: 2,
  connected: true,
  canView: true,
  claimedLabel: "",
};
const BOB: ListsPageUser = {
  id: "u2",
  displayName: "Bob Banana",
  username: "bob",
  birthday: "",
  myClaims: 0,
  connected: true,
  canView: true,
  claimedLabel: "",
};

function response(users: ListsPageUser[]): ListsPageResponse {
  return { users, page: 1, hasMore: false, total: users.length };
}

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.network.offline = false;
});

afterEach(() => {
  cleanup();
});

describe("ListsPage share-code connect", () => {
  function renderPage(users: ListsPageUser[] = [ALICE, BOB]) {
    mocks.api.mockImplementation((path: string) => {
      if (path.startsWith("/api/lists")) return Promise.resolve(response(users));
      return Promise.resolve({ displayName: "Carol Cherry" });
    });
    return render(ListsPage);
  }

  it("lists shared lists from /api/lists", async () => {
    renderPage();
    await flush();
    expect(screen.getByText("Alice Apple")).toBeTruthy();
    expect(screen.getByText("Bob Banana")).toBeTruthy();
  });

  it("POSTs the share code to /api/connect/accept and shows a success message", async () => {
    const { container } = renderPage();
    await flush();

    const input = container.querySelector(".share-input") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "CAROLCD" } });
    await flush();

    const btn = screen.getByRole("button", { name: "Connect with share code" });
    fireEvent.submit(container.querySelector("form") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/connect/accept", {
      method: "POST",
      body: { code: "CAROLCD" },
    });
    expect(screen.getByText("Connected to Carol Cherry!")).toBeTruthy();
    expect(btn.classList.contains("done")).toBe(true);
  });

  it("accepts codes with surrounding whitespace", async () => {
    const { container } = renderPage();
    await flush();
    const input = container.querySelector(".share-input") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "  carolcd  " } });
    await flush();
    fireEvent.submit(container.querySelector("form") as HTMLFormElement);
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/connect/accept", {
      method: "POST",
      body: { code: "carolcd" },
    });
  });

  it("shows an error message when connecting fails", async () => {
    mocks.api.mockImplementation((path: string) => {
      if (path.startsWith("/api/lists")) return Promise.resolve(response([ALICE]));
      return Promise.reject(new Error("No user found with that share code"));
    });
    const { container } = render(ListsPage);
    await flush();
    const input = container.querySelector(".share-input") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "NOPE0000" } });
    await flush();
    const btn = screen.getByRole("button", { name: "Connect with share code" });
    fireEvent.submit(container.querySelector("form") as HTMLFormElement);
    await flush();
    expect(screen.getByText("No user found with that share code")).toBeTruthy();
    expect(btn.classList.contains("err")).toBe(true);
  });

  it("disables the connect button while the input is empty", async () => {
    const { container } = renderPage();
    await flush();
    const btn = screen.getByRole("button", { name: "Connect with share code" });
    expect((btn as HTMLButtonElement).disabled).toBe(true);
    const input = container.querySelector(".share-input") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "ABCDEF12" } });
    await flush();
    expect((btn as HTMLButtonElement).disabled).toBe(false);
  });

  it("shows the EmptyState when there are no lists yet", async () => {
    const { container } = renderPage([]);
    await flush();
    expect(screen.getByText("No shared lists yet")).toBeTruthy();
    expect(within(container).queryByText("Alice Apple")).toBeNull();
  });
});
