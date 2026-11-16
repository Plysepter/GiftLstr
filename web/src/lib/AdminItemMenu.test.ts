import { describe, expect, it, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import AdminItemMenu from "./AdminItemMenu.svelte";

afterEach(() => cleanup());

function renderMenu(overrides: Partial<Record<string, unknown>> = {}) {
  const props = {
    x: 10,
    y: 10,
    onedit: vi.fn(),
    onregenerate: vi.fn(),
    onregenerateName: vi.fn(),
    onclose: vi.fn(),
    ...overrides,
  };
  render(AdminItemMenu, props as never);
  return props;
}

describe("AdminItemMenu", () => {
  it("renders the admin actions and fires each callback", async () => {
    const props = renderMenu();
    expect(screen.getByRole("menu", { name: "Item actions" })).toBeTruthy();

    fireEvent.click(screen.getByRole("menuitem", { name: "Edit item…" }));
    expect(props.onedit).toHaveBeenCalled();

    fireEvent.click(screen.getByRole("menuitem", { name: "Regenerate preview" }));
    expect(props.onregenerate).toHaveBeenCalled();

    fireEvent.click(screen.getByRole("menuitem", { name: /update name/ }));
    expect(props.onregenerateName).toHaveBeenCalled();
  });

  it("closes on Escape", async () => {
    const props = renderMenu();
    await fireEvent.keyDown(window, { key: "Escape" });
    expect(props.onclose).toHaveBeenCalled();
  });

  it("closes on an outside mousedown but not on an inside one", async () => {
    const props = renderMenu();
    fireEvent.mouseDown(screen.getByRole("menu"));
    expect(props.onclose).not.toHaveBeenCalled();
    fireEvent.mouseDown(document.body);
    expect(props.onclose).toHaveBeenCalled();
  });
});
