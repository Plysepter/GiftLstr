export interface User {
  id: string;
  username: string;
  displayName: string;
  isAdmin: boolean;
  birthday: string;
  themeStyle?: string;
  /** The user's share code, empty when sharing is disabled. */
  shareCode?: string;
  /** True when the account was just created (drives the first-run birthday prompt). */
  isNew?: boolean;
}

export interface Item {
  id: string;
  title: string;
  url: string;
  notes: string;
  removedAt: string | null;
  claimed: boolean;
  claimedByMe: boolean;
  /** Only present in contexts that expose the claimer (own list / archive). */
  claimedByName?: string;
}

export interface UserListResponse {
  user: Pick<User, "id" | "username" | "displayName" | "birthday" | "themeStyle">;
  items: Item[];
  isOwner: boolean;
}

export interface ListsPageUser {
  id: string;
  displayName: string;
  username: string;
  birthday: string;
  myClaims: number;
  connected: boolean;
  canView: boolean;
  claimedLabel: string;
}

export interface ListsPageResponse {
  users: ListsPageUser[];
  page: number;
  hasMore: boolean;
  total: number;
}

export interface ArchiveListSummary {
  id: string;
  label: string;
  archivedAt: string;
  itemCount: number;
  claimedCount: number;
}

export interface ArchiveIndexResponse {
  lists: ArchiveListSummary[];
  page: number;
  hasMore: boolean;
  total: number;
}

export interface ArchiveDetailResponse {
  id: string;
  label: string;
  items: Item[];
}

export interface AdminUser {
  id: string;
  username: string;
  displayName: string;
  isAdmin: boolean;
  birthday: string;
  /** Items on the user's active list (excluding removed). */
  itemCount: number;
  /** Claims held on the user's active list. */
  claimedCount: number;
  /** Outgoing connection user ids. */
  connections: string[];
  /** Incoming connection user ids. */
  connectedBy: string[];
}

export interface AdminUsersResponse {
  users: AdminUser[];
  page: number;
  hasMore: boolean;
  total: number;
  names?: Record<string, string>;
}

export interface PageMetadata {
  url: string;
  title: string;
  description: string;
  image: string;
}

/**
 * Normalizes a user-typed URL: prepends https:// when no scheme is present.
 * Returns "" for empty input, or null if not a valid absolute http(s) URL.
 */
export function normalizeItemURL(raw: string): string | null {
  const s = raw.trim();
  if (!s) return "";
  const candidate = /^[a-z][a-z0-9+.-]*:/i.test(s) ? s : `https://${s}`;
  try {
    const u = new URL(candidate);
    if ((u.protocol === "https:" || u.protocol === "http:") && u.host) return u.toString();
  } catch {
    // fall through
  }
  return null;
}
