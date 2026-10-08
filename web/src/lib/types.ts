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
  /** 'mothers' | 'fathers' — extra reset holiday besides birthday & Christmas. */
  holidayReset?: string;
  /** Whether connected users can add suggested items to your list. Default true. */
  allowSuggestions?: boolean;
  /** Whether the user can attach private prices to claimed gifts. Default false. */
  spendingEnabled?: boolean;
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
  /** "Suggested by Jane D." tag on shared private items shown to connected users. */
  suggestedBy?: string;
  /** True when the current viewer authored this shared idea (can retract it). */
  suggestedByMe?: boolean;
  /**
   * The viewer's private price on this item in cents. Only present for items
   * the viewer holds a claim on and when spending tracking is enabled.
   */
  mySpendCents?: number;
}

export interface UserListResponse {
  user: Pick<User, "id" | "username" | "displayName" | "birthday" | "themeStyle" | "holidayReset">;
  items: Item[];
  /**
   * Private ideas the current viewer saved for this list. Only returned when
   * viewing someone else's list (never the owner's own list).
   */
  privateItems?: Item[];
  isOwner: boolean;
  /**
   * The viewer's private spend in cents on the active items of exactly this
   * list. Only present when spending tracking is enabled.
   */
  totalSpendCents?: number;
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
  /**
   * True for the viewer's own private lists, which are inlined into the page
   * sorted with regular users and opened at #/private-list/{id}.
   */
  isPrivate?: boolean;
  /**
   * The viewer's private spend in cents on this user's active list. Only
   * present when the viewer has claimed something and spending tracking is
   * enabled.
   */
  spendCents?: number;
}

export interface ListsPageResponse {
  users: ListsPageUser[];
  page: number;
  hasMore: boolean;
  total: number;
}

/** One of the viewer's own private lists, served by /api/private-lists/{id}. */
export interface PrivateListResponse {
  user: Pick<User, "id" | "username" | "displayName" | "birthday" | "themeStyle" | "holidayReset">;
  items: Item[];
  isOwner: boolean;
  /**
   * The viewer's private spend in cents on the active items of exactly this
   * list. Only present when spending tracking is enabled.
   */
  totalSpendCents?: number;
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
