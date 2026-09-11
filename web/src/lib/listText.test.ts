import { describe, expect, it } from "vitest";
import { buildListText, selectExportItems } from "./listText.js";
import type { Item } from "./types.js";

function item(partial: Partial<Item> & { title: string }): Item {
  return {
    id: 1,
    url: "",
    notes: "",
    removedAt: null,
    claimed: false,
    claimedByMe: false,
    ...partial,
  } as Item;
}

describe("buildListText", () => {
  it("starts with {owner}'s {event} list:", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "Christmas",
      items: [],
      isOwner: true,
    });
    expect(text).toBe("Alex's Christmas list:");
  });

  it("drops the event word when there is none", () => {
    const text = buildListText({ ownerName: "Alex", event: "", items: [], isOwner: true });
    expect(text).toBe("Alex's list:");
  });

  it("uses the URL as the item title when present, otherwise the name", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [
        item({ title: "With link", url: "https://example.com/thing" }),
        item({ title: "Just a name", url: "" }),
      ],
      isOwner: true,
    });
    expect(text).toContain("- https://example.com/thing");
    expect(text).toContain("- Just a name");
  });

  it("puts one blank line between an item and its notes, prefixed with Notes:", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [item({ title: "Thing", url: "", notes: "the note" })],
      isOwner: true,
    });
    expect(text).toContain("- Thing\n\nNotes: the note");
  });

  it("places two empty lines between items", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [item({ title: "One", url: "" }), item({ title: "Two", url: "" })],
      isOwner: true,
    });
    expect(text).toContain("- One\n\n\n- Two");
  });

  it("places a single empty line between heading and first item only", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [item({ title: "One", url: "" }), item({ title: "Two", url: "" })],
      isOwner: true,
    });
    expect(text).toMatch(/^Alex's birthday list:\n\n- One/);
    expect(text).not.toMatch(/^Alex's birthday list:\n\n\n/);
  });

  it("produces the full expected clipboard layout", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [
        item({ title: "With link", url: "https://example.com/thing", notes: "note text" }),
        item({ title: "Just a name", url: "" }),
      ],
      isOwner: true,
    });
    expect(text).toBe(
      "Alex's birthday list:\n\n" +
        "- https://example.com/thing\n\nNotes: note text\n\n\n" +
        "- Just a name",
    );
  });

  it("does not separate an item with no notes from the next by notes lines", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [item({ title: "One", url: "" }), item({ title: "Two", url: "" })],
      isOwner: true,
    });
    expect(text).toContain("- One\n\n\n- Two");
    expect(text).not.toMatch(/- One\n\n\n\n/);
  });

  it("moves ideas suggested by others under a trailing section for a visitor", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [
        item({ title: "From Alex", url: "" }),
        item({ title: "Suggested one", url: "", suggestedBy: "Bob" }),
        item({ title: "Suggested two", url: "", suggestedBy: "Carol" }),
      ],
      isOwner: false,
    });
    expect(text).toBe(
      "Alex's birthday list:\n\n" +
        "- From Alex\n\n\n" +
        "Suggested ideas from others:\n\n" +
        "- Suggested one\n\n\n" +
        "- Suggested two",
    );
  });

  it("puts extra spacing before the suggested-ideas header", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [
        item({ title: "From Alex", url: "" }),
        item({ title: "Suggested one", url: "", suggestedBy: "Bob" }),
      ],
      isOwner: false,
    });
    expect(text).toMatch(/- From Alex\n\n\nSuggested ideas from others:/);
  });

  it("keeps suggested items inline for the list owner", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [
        item({ title: "From Alex", url: "" }),
        item({ title: "Suggested one", url: "", suggestedBy: "Bob" }),
      ],
      isOwner: true,
    });
    expect(text).toContain("- From Alex\n\n\n- Suggested one");
    expect(text).not.toContain("Suggested ideas from others");
  });

  it("omits the suggested section when there are no suggestions", () => {
    const text = buildListText({
      ownerName: "Alex",
      event: "birthday",
      items: [item({ title: "From Alex", url: "" })],
      isOwner: false,
    });
    expect(text).toBe("Alex's birthday list:\n\n- From Alex");
    expect(text).not.toContain("Suggested ideas from others");
  });
});

describe("selectExportItems", () => {
  it("always excludes archived items", () => {
    const items = [item({ title: "kept" }), item({ title: "gone", removedAt: "2026-01-01" })];
    expect(selectExportItems(items, true).map((i) => i.title)).toEqual(["kept"]);
    expect(selectExportItems(items, false).map((i) => i.title)).toEqual(["kept"]);
  });

  it("excludes claimed items for a non-author", () => {
    const items = [
      item({ title: "free" }),
      item({ title: "taken", claimed: true, claimedByMe: false }),
    ];
    expect(selectExportItems(items, false).map((i) => i.title)).toEqual(["free"]);
  });

  it("keeps claimed items for the author", () => {
    const items = [item({ title: "kept", claimed: true, claimedByMe: true })];
    expect(selectExportItems(items, true).map((i) => i.title)).toEqual(["kept"]);
  });
});
