import { api } from "./api.js";
import type { PageMetadata } from "./types.js";

const cache = new Map<string, Promise<PageMetadata | null>>();

/**
 * Fetches Open Graph metadata for a URL, de-duplicating concurrent and
 * repeated requests for the same target across the whole app.
 * Resolves to null when the lookup fails.
 */
export function getMetadata(url: string): Promise<PageMetadata | null> {
  let p = cache.get(url);
  if (!p) {
    p = api<PageMetadata>(`/api/metadata?url=${encodeURIComponent(url)}`).catch(() => null);
    cache.set(url, p);
  }
  return p;
}
