import { describe, expect, it, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import AdminItemDialog from "./AdminItemDialog.svelte";
import type { Item } from "./types.js";

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
  invalidateMetadata: vi.fn(),
}));

vi.mock("./api.js", () => ({ api: mocks.api }));
vi.mock("./metadata.js", () => ({
  metadataVersion: { subscribe: () => () => {} },
  getMetadata: () => Promise.resolve(null),
  invalidateMetadata: mocks.invalidateMetadata,
}));

function item(partial: Partial<Item> = {}): Item {
  return {
    id: "i1",
    title: "Mug",
    url: "https://example.com/mug",
    notes: "old notes",
    removedAt: null,
    claimed: false,
    claimedByMe: false,
    ...partial,
  } as Item;
}

async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

function form(container: HTMLElement): HTMLFormElement {
  return container.querySelector("form") as HTMLFormElement;
}

beforeEach(() => {
  mocks.api.mockReset();
  mocks.invalidateMetadata.mockReset();
});

afterEach(() => cleanup());

describe("AdminItemDialog", () => {
  it("prefills the item and PATCHes the admin endpoint", async () => {
    mocks.api.mockResolvedValue({ ok: true });
    const onclose = vi.fn();
    const onsaved = vi.fn();
    const { container } = render(AdminItemDialog, { item: item(), onclose, onsaved });
    await flush();

    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Mug");
    expect((screen.getByLabelText("Link") as HTMLInputElement).value).toBe(
      "https://example.com/mug",
    );
    expect((screen.getByLabelText("Notes") as HTMLTextAreaElement).value).toBe("old notes");

    fireEvent.input(screen.getByLabelText("Name"), { target: { value: "New name" } });
    fireEvent.input(screen.getByLabelText("Notes"), { target: { value: "new notes" } });
    fireEvent.submit(form(container));
    await flush();

    expect(mocks.api).toHaveBeenCalledWith("/api/admin/items/i1", {
      method: "PATCH",
      body: {
        title: "New name",
        url: "https://example.com/mug",
        notes: "new notes",
        regenerate: false,
        updateName: false,
      },
    });
    expect(onsaved).toHaveBeenCalled();
    expect(onclose).toHaveBeenCalled();
  });

  it("sends regenerate and updateName when the admin opts in", async () => {
    mocks.api.mockResolvedValue({ ok: true });
    const { container } = render(AdminItemDialog, { item: item(), onclose: vi.fn() });
    await flush();

    fireEvent.click(screen.getByLabelText("Regenerate link preview"));
    fireEvent.click(screen.getByLabelText("Update name from page title"));
    fireEvent.submit(form(container));
    await flush();

    const body = mocks.api.mock.calls[0]?.[1]?.body as Record<string, unknown>;
    expect(body.regenerate).toBe(true);
    expect(body.updateName).toBe(true);
    expect(mocks.invalidateMetadata).toHaveBeenCalledWith("https://example.com/mug");
  });

  it("gates update-name on regenerate and clears it when regenerate is turned off", async () => {
    render(AdminItemDialog, { item: item(), onclose: vi.fn() });
    await flush();
    const regen = screen.getByLabelText("Regenerate link preview") as HTMLInputElement;
    const upd = screen.getByLabelText("Update name from page title") as HTMLInputElement;
    expect(upd.disabled).toBe(true);

    fireEvent.click(regen);
    await flush();
    expect(upd.disabled).toBe(false);

    fireEvent.click(upd);
    await flush();
    expect(upd.checked).toBe(true);

    fireEvent.click(regen);
    await flush();
    expect(upd.checked).toBe(false);
  });

  it("invalidates both the old and the new URL when the link changes", async () => {
    mocks.api.mockResolvedValue({ ok: true });
    const { container } = render(AdminItemDialog, { item: item(), onclose: vi.fn() });
    await flush();

    fireEvent.input(screen.getByLabelText("Link"), {
      target: { value: "https://example.com/new" },
    });
    fireEvent.submit(form(container));
    await flush();

    expect(mocks.invalidateMetadata).toHaveBeenCalledWith("https://example.com/new");
    expect(mocks.invalidateMetadata).toHaveBeenCalledWith("https://example.com/mug");
  });

  it("reports a metadataError without blocking the save", async () => {
    mocks.api.mockResolvedValue({ ok: true, metadataError: "boom" });
    const onerror = vi.fn();
    const onclose = vi.fn();
    const { container } = render(AdminItemDialog, { item: item(), onclose, onerror });
    await flush();

    fireEvent.click(screen.getByLabelText("Regenerate link preview"));
    fireEvent.submit(form(container));
    await flush();

    expect(onerror).toHaveBeenCalledWith(expect.stringContaining("boom"));
    expect(onclose).toHaveBeenCalled();
  });

  it("rejects an invalid link without calling the API", async () => {
    const { container } = render(AdminItemDialog, { item: item(), onclose: vi.fn() });
    await flush();

    fireEvent.input(screen.getByLabelText("Link"), { target: { value: "not a url" } });
    fireEvent.submit(form(container));
    await flush();

    expect(mocks.api).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toMatch(/valid http\(s\) link/);
  });

  it("keeps the dialog open and shows the error when the request fails", async () => {
    mocks.api.mockRejectedValue(new Error("nope"));
    const onclose = vi.fn();
    const { container } = render(AdminItemDialog, { item: item(), onclose });
    await flush();

    fireEvent.submit(form(container));
    await flush();

    expect(onclose).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toMatch(/nope/);
  });
});
