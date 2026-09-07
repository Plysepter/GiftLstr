import { describe, expect, it, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/svelte";
import Login from "./Login.svelte";

vi.mock("../lib/auth.js", () => ({ openSignIn: vi.fn() }));

describe("Login centered card", () => {
  it("renders a centered card with max-width and the sign-in button", () => {
    const { container } = render(Login);
    const card = container.querySelector(".card") as HTMLElement;
    expect(card).toBeTruthy();
    expect(card.style.maxWidth).toBe("380px");
    expect(card.style.margin).toContain("auto");
    expect(card.style.textAlign).toBe("center");
    expect(screen.getByText("GiftLstr")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy();
    cleanup();
  });
});
