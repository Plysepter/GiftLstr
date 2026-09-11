import type { Item } from "./types.js";

export interface ListTextOptions {
  /** Display name of the list's owner. */
  ownerName: string;
  /** Event label, e.g. "Christmas" or "birthday". May be empty. */
  event: string;
  /** All items on the list (including removed/claimed). */
  items: Item[];
  /** Whether the person exporting the list is its author. */
  isOwner: boolean;
}

/**
 * Items that belong in a clipboard export: always excludes archived (removed)
 * items; when the exporter is not the list's author, also excludes items that
 * are already claimed by someone else.
 */
export function selectExportItems(items: Item[], isOwner: boolean): Item[] {
  return items.filter((i) => {
    if (i.removedAt) return false;
    if (!isOwner && i.claimed) return false;
    return true;
  });
}

/**
 * Builds the clipboard text for a list:
 *
 *   {owner}'s {event} list:
 *
 *   - {url or name}
 *
 *   Notes: {notes}
 *
 *
 *   - {url or name}
 *
 * Heading on the first line, one blank line before the first item, two blank
 * lines between items, and a single blank line between an item and its notes.
 *
 * When exporting someone else's list, ideas suggested by others are pulled out
 * into their own trailing section, preceded by extra (two blank line) spacing:
 *
 *
 *   Suggested ideas from others:
 *
 *   - {url or name}
 */
export function buildListText({ ownerName, event, items, isOwner }: ListTextOptions): string {
  const heading = event ? `${ownerName}'s ${event} list:` : `${ownerName}'s list:`;
  const format = ({ title, url, notes }: Item) => {
    const line = `- ${url || title}`;
    return notes ? `${line}\n\nNotes: ${notes}` : line;
  };
  const selected = selectExportItems(items, isOwner);
  const isVisitor = !isOwner;
  const suggested = isVisitor ? selected.filter((i) => i.suggestedBy).map(format) : [];
  const own = (isVisitor ? selected.filter((i) => !i.suggestedBy) : selected).map(format);
  const body = own.join("\n\n\n");
  let text = body ? `${heading}\n\n${body}` : heading;
  if (isVisitor && suggested.length > 0) {
    text += `\n\n\nSuggested ideas from others:\n\n${suggested.join("\n\n\n")}`;
  }
  return text;
}

/** Copies text to the clipboard, falling back to a hidden textarea. */
export async function copyToClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  const ok = document.execCommand("copy");
  ta.remove();
  if (!ok) throw new Error("Could not copy to clipboard");
}
