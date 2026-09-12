import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup, within } from "@testing-library/svelte";
import BirthdayModal from "./BirthdayModal.svelte";
import type { User } from "./types.js";

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
  };
});

vi.mock("./api.js", () => ({ api: mocks.api }));
vi.mock("./auth.js", () => ({
  user: mocks.userStore,
  openSignIn: vi.fn(),
  signOut: vi.fn(),
}));

const NEW: User = {
  id: "u7",
  username: "wren",
  displayName: "Wren Newguy",
  isAdmin: false,
  birthday: "",
  isNew: true,
};

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.userStore.set(NEW);
});

afterEach(() => {
  cleanup();
  mocks.userStore.set(null);
});

describe("BirthdayModal first-run prompt", () => {
  function birthDialog(container: HTMLElement): HTMLDialogElement {
    const dlg = Array.from(container.querySelectorAll("dialog")).find(
      (d) => (d as HTMLDialogElement).open,
    ) as HTMLDialogElement;
    if (!dlg) throw new Error("expected an open birthday dialog");
    return dlg;
  }

  it("shows the modal for a brand-new user", async () => {
    const { container } = render(BirthdayModal);
    await flush();
    const dlg = birthDialog(container);
    expect(within(dlg).getByText("What’s your birthday?")).toBeTruthy();
  });

  it("does not show the modal for existing users", async () => {
    mocks.userStore.set({ ...NEW, isNew: false });
    render(BirthdayModal);
    await flush();
    expect(screen.queryByText("What’s your birthday?")).toBeNull();
  });

  it("saves the birthday via PATCH /api/profile and clears isNew", async () => {
    const { container } = render(BirthdayModal);
    await flush();
    const dlg = birthDialog(container);

    const month = within(dlg).getByLabelText("Birth month") as HTMLSelectElement;
    const day = within(dlg).getByLabelText("Birth day") as HTMLSelectElement;
    fireEvent.change(month, { target: { value: "05" } });
    fireEvent.change(day, { target: { value: "15" } });
    fireEvent.submit(dlg.querySelector("form") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/profile", {
      method: "PATCH",
      body: { birthday: "05-15", holidayReset: "" },
    });
    expect(((mocks.userStore as { value: unknown }).value as User | null)?.birthday).toBe("05-15");
    expect(((mocks.userStore as { value: unknown }).value as User | null)?.isNew).toBe(false);
  });

  it("saves an additional reset holiday along with the birthday", async () => {
    const { container } = render(BirthdayModal);
    await flush();
    const dlg = birthDialog(container);

    fireEvent.change(within(dlg).getByLabelText("Birth month"), { target: { value: "06" } });
    fireEvent.change(within(dlg).getByLabelText("Birth day"), { target: { value: "10" } });
    fireEvent.change(within(dlg).getByLabelText("Additional reset holiday"), {
      target: { value: "fathers" },
    });
    fireEvent.submit(dlg.querySelector("form") as HTMLFormElement);
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/profile", {
      method: "PATCH",
      body: { birthday: "06-10", holidayReset: "fathers" },
    });
  });

  it("dismisses the prompt without saving when skipped", async () => {
    const { container } = render(BirthdayModal);
    await flush();
    const dlg = birthDialog(container);
    fireEvent.click(within(dlg).getByRole("button", { name: "Skip" }));
    await flush();

    expect(mocks.api).not.toHaveBeenCalled();
    expect(((mocks.userStore as { value: unknown }).value as User | null)?.isNew).toBe(false);
  });
});
