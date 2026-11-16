import { writable } from "svelte/store";
import { api } from "./api.js";
import type { PageMetadata } from "./types.js";

const cache = new Map<string, Promise<PageMetadata | null>>();

/**
 * Bumped whenever a cached preview is invalidated (e.g. an admin regenerated
 * one). Components read this inside their metadata effect so a URL whose string
 * is unchanged still re-runs its lookup. Other URLs keep their cache entry, so
 * only the invalidated target refetches.
 */
export const metadataVersion = writable(0);

/**
 * Drops the cached preview for a URL and bumps the version so live components
 * re-fetch it.
 */
export function invalidateMetadata(url: string): void {
  if (url) cache.delete(url);
  metadataVersion.update((n) => n + 1);
}

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
