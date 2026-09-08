export type ThemeChoice = "system" | "light" | "dark";
export type ThemeStyle =
  | "aurora"
  | "aurora-warm"
  | "kinetic"
  | "ocean"
  | "forest"
  | "berry"
  | "gold"
  | "ruby"
  | "teal"
  | "dino"
  | "dogs";
export type Season = "none" | "christmas" | "birthday";

export const ALL_STYLES: ThemeStyle[] = [
  "aurora",
  "aurora-warm",
  "kinetic",
  "ocean",
  "forest",
  "berry",
  "gold",
  "ruby",
  "teal",
  "dino",
  "dogs",
];

const KEY = "theme";

/** The user's persisted preference; 'system' follows prefers-color-scheme. */
export const theme = $state<{ choice: ThemeChoice }>({ choice: loadChoice() });

export const prefs = $state<{ style: ThemeStyle; events: boolean; useOwnerTheme: boolean }>(
  loadPrefs(),
);

/**
 * Active style override set while viewing another user's list that opts into
 * "show their theme". apply() honours it ahead of prefs.style so that light/dark
 * toggles (which call apply()) re-theme the brightness without clobbering the
 * gift recipient's chosen palette.
 */
export const ownerStyleOverride = $state<{ value: ThemeStyle | null }>({ value: null });

/** Set once App knows the user's birthday; drives event theming. */
export const season = $state<{ kind: Season; days: number; label: string }>({
  kind: "none",
  days: 0,
  label: "",
});

const SEASON_OVERRIDE_KEY = "season-override";

/** Admin-only local override: 'auto' follows birthdays/Christmas, otherwise forces the palette. */
export const seasonOverride = $state<{ value: Season | "auto" }>({ value: loadSeasonOverride() });

function loadSeasonOverride(): Season | "auto" {
  try {
    const v = localStorage.getItem(SEASON_OVERRIDE_KEY);
    if (v === "christmas" || v === "birthday" || v === "none") return v;
  } catch {
    // localStorage unavailable
  }
  return "auto";
}

export function setSeasonOverride(v: Season | "auto"): void {
  seasonOverride.value = v;
  try {
    if (v === "auto") localStorage.removeItem(SEASON_OVERRIDE_KEY);
    else localStorage.setItem(SEASON_OVERRIDE_KEY, v);
  } catch {
    // Non-fatal
  }
  computeSeason(lastBirthdayMMDD);
}

let lastBirthdayMMDD = "01-01";

function loadChoice(): ThemeChoice {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    // localStorage unavailable; fall back to system.
  }
  return "system";
}

function loadPrefs(): { style: ThemeStyle; events: boolean; useOwnerTheme: boolean } {
  try {
    let style = localStorage.getItem("theme-style");
    const events = localStorage.getItem("theme-events");
    const useOwnerTheme = localStorage.getItem("theme-useOwner");
    return {
      style: ALL_STYLES.includes(style as ThemeStyle) ? (style as ThemeStyle) : "aurora",
      events: events !== "0",
      useOwnerTheme: useOwnerTheme !== "0",
    };
  } catch {
    return { style: "aurora", events: true, useOwnerTheme: true };
  }
}

function systemDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function apply(): void {
  const resolved = theme.choice === "system" ? (systemDark() ? "dark" : "light") : theme.choice;
  document.documentElement.dataset.theme = resolved;
  const style = ownerStyleOverride.value !== null ? ownerStyleOverride.value : prefs.style;
  document.documentElement.dataset.style = style;
  document.documentElement.dataset.season = season.kind;
}

/** Pure helper: returns the season that would be active for a given birthday. */
export function resolveSeason(
  birthdayMMDD: string,
  eventsEnabled: boolean,
  today = new Date(),
): { kind: Season; days: number; label: string } {
  const next = (month: number, day: number): number => {
    let t = new Date(today.getFullYear(), month - 1, day, 23, 59, 59);
    if (t.getTime() < today.getTime())
      t = new Date(today.getFullYear() + 1, month - 1, day, 23, 59, 59);
    return Math.round((t.getTime() - today.getTime()) / 864e5);
  };
  const bday = next(
    Number(birthdayMMDD.split("-")[0]) || 1,
    Number(birthdayMMDD.split("-")[1]) || 1,
  );
  const xmas = next(12, 25);
  let kind: Season = "none";
  let days = 0;
  let label = "";
  if (seasonOverride.value !== "auto") {
    kind = seasonOverride.value;
    if (kind === "christmas") {
      days = xmas;
      label = "Christmas";
    } else if (kind === "birthday") {
      days = bday;
      label = "birthday";
    } else {
      days = 0;
      label = "";
    }
  } else {
    if (bday <= xmas) {
      days = bday;
      label = "birthday";
      kind = "birthday";
    } else {
      days = xmas;
      label = "Christmas";
      kind = "christmas";
    }
    if (!eventsEnabled || days >= 30) kind = "none";
  }
  return { kind, days, label };
}

/** Sets the theme preference, persists it, and applies it immediately. */
export function setTheme(choice: ThemeChoice): void {
  theme.choice = choice;
  try {
    localStorage.setItem(KEY, choice);
  } catch {
    // Non-fatal; the in-memory choice still applies for this session.
  }
  apply();
}

export function setStyle(style: ThemeStyle): void {
  prefs.style = style;
  try {
    localStorage.setItem("theme-style", style);
  } catch {
    // Non-fatal.
  }
  apply();
}

export function setEvents(events: boolean): void {
  prefs.events = events;
  try {
    localStorage.setItem("theme-events", events ? "1" : "0");
  } catch {
    // Non-fatal.
  }
  apply();
}

export function setUseOwnerTheme(v: boolean): void {
  prefs.useOwnerTheme = v;
  try {
    localStorage.setItem("theme-useOwner", v ? "1" : "0");
  } catch {
    // Non-fatal.
  }
}

/**
 * Recomputes the active season from the user's birthday (+ fixed Christmas)
 * and the "event theming" preference. Events apply when less than a month away.
 * An admin local override (seasonOverride) forces the palette regardless of date.
 */
export function computeSeason(birthdayMMDD: string, today = new Date()): void {
  lastBirthdayMMDD = birthdayMMDD;
  const { kind, days, label } = resolveSeason(birthdayMMDD, prefs.events, today);
  season.kind = kind;
  season.days = days;
  season.label = label;
  apply();
}

// honour persisted admin override before first paint
if (seasonOverride.value !== "auto") {
  season.kind = seasonOverride.value;
}

const media = window.matchMedia("(prefers-color-scheme: dark)");
media.addEventListener("change", apply);
apply();
