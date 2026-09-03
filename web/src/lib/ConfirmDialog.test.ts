import { describe, expect, it, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import type { ComponentProps } from "svelte";
import ConfirmDialog from "./ConfirmDialog.svelte";

type Props = ComponentProps<typeof ConfirmDialog>;

/** Lets Svelte 5 flush its microtask-scheduled DOM updates. */
async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function mount(props: Props) {
  return render(ConfirmDialog, props);
}

describe("ConfirmDialog", () => {
  it("stays closed until {open} becomes true", () => {
    const { container } = mount({ open: false, title: "Do it?" });
    const dialog = container.querySelector("dialog");
    expect(dialog).not.toBeNull();
    expect((dialog as HTMLDialogElement).open).toBe(false);
  });

  it("opens when open=true and closes again when open flips back to false", async () => {
    const { container, rerender } = mount({
      open: true,
      title: "Do it?",
      message: "Are you sure?",
    });
    expect((container.querySelector("dialog") as HTMLDialogElement).open).toBe(true);
    await rerender({ open: false });
    expect((container.querySelector("dialog") as HTMLDialogElement).open).toBe(false);
  });

  it("confirms via the primary button", () => {
    const onconfirm = vi.fn();
    mount({ open: true, title: "Do it?", confirmLabel: "Go", onconfirm });
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    expect(onconfirm).toHaveBeenCalledTimes(1);
  });

  it("awaits an async onconfirm and shows busy state meanwhile", async () => {
    let release!: () => void;
    let resolved = false;
    const onconfirm = vi.fn(() =>
      new Promise<void>((resolve) => {
        release = () => resolve();
      }).then(() => {
        resolved = true;
      }),
    );
    mount({ open: true, title: "Do it?", confirmLabel: "Go", onconfirm });
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    await flush();
    expect((screen.getByRole("button", { name: "Working…" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect((screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(resolved).toBe(false);
    release();
    await flush();
    expect(resolved).toBe(true);
  });

  it("cancels via cancel button and never calls onconfirm", () => {
    const onconfirm = vi.fn();
    const oncancel = vi.fn();
    mount({ open: true, title: "Do it?", confirmLabel: "Go", onconfirm, oncancel });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(oncancel).toHaveBeenCalledTimes(1);
    expect(onconfirm).not.toHaveBeenCalled();
  });

  it("requires typing the exact confirmText to enable Confirm", async () => {
    const onconfirm = vi.fn();
    mount({
      open: true,
      title: "Delete?",
      message: "Type to confirm",
      confirmLabel: "Delete",
      confirmText: "bob",
      onconfirm,
    });
    const confirm = screen.getByRole("button", { name: "Delete" }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);

    const input = screen.getByLabelText(/to confirm/) as HTMLInputElement;
    fireEvent.input(input, { target: { value: "bo" } });
    await flush();
    expect(confirm.disabled).toBe(true);
    fireEvent.input(input, { target: { value: "bob" } });
    await flush();
    expect(confirm.disabled).toBe(false);

    fireEvent.click(confirm);
    expect(onconfirm).toHaveBeenCalledTimes(1);
  });

  it("does not confirm an empty confirmText field", () => {
    const onconfirm = vi.fn();
    mount({ open: true, title: "Delete?", confirmText: "x", onconfirm });
    const input = screen.getByLabelText(/to confirm/) as HTMLInputElement;
    fireEvent.input(input, { target: { value: "" } });
    expect((screen.getByRole("button", { name: "Confirm" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    expect(onconfirm).not.toHaveBeenCalled();
  });

  it("marks the danger button with the danger class", () => {
    mount({ open: true, title: "Delete?", confirmLabel: "Delete", danger: true });
    expect(screen.getByRole("button", { name: "Delete" }).className).toContain("danger");
  });

  it("fires oncancel when the dialog closes natively (close event)", () => {
    const oncancel = vi.fn();
    const { container } = mount({ open: true, title: "Do it?", oncancel });
    (container.querySelector("dialog") as HTMLDialogElement).close();
    expect(oncancel).toHaveBeenCalledTimes(1);
  });

  it("resets the typed confirmText between sessions", async () => {
    const props: Props = { open: true, title: "Delete?", confirmText: "bob" };
    const { rerender } = mount(props);
    const input = screen.getByLabelText(/to confirm/) as HTMLInputElement;
    fireEvent.input(input, { target: { value: "bob" } });
    await flush();
    expect((screen.getByRole("button", { name: "Confirm" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
    await rerender({ open: false });
    await flush();
    await rerender(props);
    await flush();
    expect((screen.getByLabelText(/to confirm/) as HTMLInputElement).value).toBe("");
  });
});
