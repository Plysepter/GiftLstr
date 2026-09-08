import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup, within } from "@testing-library/svelte";
import Profile from "./Profile.svelte";
import type { User } from "../lib/types.js";

const mocks = vi.hoisted(() => {
  type Cb = (v: unknown) => void;
  function store(init: unknown) {
    let value: unknown = init;
    const subs = new Set<Cb>();
    return {
      value,
      subscribe(cb: Cb): () => void {
        cb(value);
        subs.add(cb);
        return () => {
          subs.delete(cb);
        };
      },
      set(next: unknown): void {
        value = next;
        this.value = value;
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
    setTheme: vi.fn(),
    setStyle: vi.fn(),
    setEvents: vi.fn(),
    setUseOwnerTheme: vi.fn(),
    signOut: vi.fn(),
    copyToClipboard: vi.fn(),
  };
});

vi.mock("../lib/api.js", () => ({ api: mocks.api }));
vi.mock("../lib/auth.js", () => ({
  user: mocks.userStore,
  loaded: mocks.loadedStore,
  openSignIn: vi.fn(),
  signOut: mocks.signOut,
}));
vi.mock("../lib/theme.svelte.js", () => ({
  theme: { choice: "system" },
  prefs: { style: "kinetic", events: true, useOwnerTheme: false },
  season: { kind: "none", days: 0, label: "" },
  seasonOverride: { value: "auto" },
  setTheme: mocks.setTheme,
  setStyle: mocks.setStyle,
  setEvents: mocks.setEvents,
  setUseOwnerTheme: mocks.setUseOwnerTheme,
}));
vi.mock("../lib/listText.js", () => ({ copyToClipboard: mocks.copyToClipboard }));

const ALICE: User = {
  id: "u1",
  username: "alice",
  displayName: "Alice Apple",
  isAdmin: false,
  birthday: "",
  themeStyle: "kinetic",
  shareCode: "",
};

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.copyToClipboard.mockReset();
  mocks.copyToClipboard.mockResolvedValue(undefined);
  mocks.userStore.set(ALICE);
  mocks.loadedStore.set(true);
});

afterEach(() => {
  cleanup();
  mocks.userStore.set(null);
  mocks.loadedStore.set(false);
});

describe("Connect with others", () => {
  it("shows the toggle and no code when sharing is disabled", async () => {
    const { container } = render(Profile);
    await flush();
    expect(screen.getByText("Connect with others")).toBeTruthy();
    const toggle = container.querySelector("#shareToggle") as HTMLInputElement;
    expect(toggle.checked).toBe(false);
    expect(container.querySelector(".share-code")).toBeNull();
  });

  it("enabling the toggle PUTs /api/connect/share and reveals the code", async () => {
    mocks.api.mockResolvedValue({ shareCode: "ABCDEF12" });
    const { container } = render(Profile);
    await flush();
    const toggle = container.querySelector("#shareToggle") as HTMLInputElement;
    fireEvent.click(toggle);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/connect/share", {
      method: "PUT",
      body: { enabled: true },
    });
    const code = container.querySelector(".share-code") as HTMLElement;
    expect(code.textContent).toBe("ABCDEF12");
    expect(((mocks.userStore as { value: unknown }).value as User | null)?.shareCode).toBe(
      "ABCDEF12",
    );
  });

  it("disabling PUTs enabled:false and hides the code", async () => {
    mocks.userStore.set({ ...ALICE, shareCode: "ABCDEF12" });
    mocks.api.mockResolvedValue({ shareCode: "" });
    const { container } = render(Profile);
    await flush();

    const toggle = container.querySelector("#shareToggle") as HTMLInputElement;
    expect(toggle.checked).toBe(true);
    expect(container.querySelector(".share-code")).toBeTruthy();

    fireEvent.click(toggle);
    await flush();
    expect(mocks.api).toHaveBeenCalledWith("/api/connect/share", {
      method: "PUT",
      body: { enabled: false },
    });
    expect(container.querySelector(".share-code")).toBeNull();
  });

  it("copy button copies the code via clipboard helper", async () => {
    mocks.userStore.set({ ...ALICE, shareCode: "ZYXWVU98" });
    render(Profile);
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "Copy share code" }));
    await flush();
    expect(mocks.copyToClipboard).toHaveBeenCalledWith("ZYXWVU98");
  });
});

describe("Birthday", () => {
  it("saves birthday via PATCH /api/profile and shows green check", async () => {
    const { container } = render(Profile);
    await flush();
    fireEvent.change(screen.getByLabelText("Birth month"), { target: { value: "05" } });
    fireEvent.change(screen.getByLabelText("Birth day"), { target: { value: "15" } });
    fireEvent.submit(container.querySelector("form.card") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/profile", {
      method: "PATCH",
      body: { birthday: "05-15" },
    });
    const btn = container.querySelector(".bday-btn") as HTMLElement;
    expect(btn.classList.contains("done")).toBe(true);
  });

  it("shows red error state when saving fails", async () => {
    mocks.api.mockRejectedValue(new Error("boom"));
    const { container } = render(Profile);
    await flush();
    fireEvent.change(screen.getByLabelText("Birth month"), { target: { value: "05" } });
    fireEvent.change(screen.getByLabelText("Birth day"), { target: { value: "15" } });
    fireEvent.submit(container.querySelector("form.card") as HTMLFormElement);
    await flush();

    const btn = container.querySelector(".bday-btn") as HTMLElement;
    expect(btn.classList.contains("err")).toBe(true);
    expect(btn.classList.contains("done")).toBe(false);
  });
});

describe("Appearance", () => {
  it("lists Aurora first as the default style", async () => {
    render(Profile);
    await flush();
    const styles = within(screen.getByRole("radiogroup", { name: "Theme style" })).getAllByRole(
      "button",
    );
    const labels = styles.map((b) => b.textContent ?? "");
    expect(labels[0]).toMatch(/Aurora/);
    expect(labels.join(" ")).toMatch(/Aurora.*Kinetic/);
  });
});
