import { Temporal } from "temporal-polyfill";

export function parseMMDD(mmdd: string, year: number): Temporal.PlainDate {
  const [m, d] = mmdd.split("-").map(Number);
  return Temporal.PlainDate.from({ year, month: m ?? 1, day: d ?? 1 });
}

export function lastResetOccurrence(mmdd: string, now: Temporal.PlainDate): Temporal.PlainDate {
  let t = parseMMDD(mmdd, now.year);
  if (Temporal.PlainDate.compare(t, now) > 0) {
    t = parseMMDD(mmdd, now.year - 1);
  }
  return t;
}

export type HolidayKind = "mothers" | "fathers";

/**
 * The Nth weekday-Sunday of a month (Temporal's dayOfWeek is Mon=1..Sun=7).
 * Mother's Day = 2nd Sunday of May, Father's Day = 3rd Sunday of June.
 */
export function nthSundayOf(year: number, month: number, nth: number): Temporal.PlainDate {
  const first = Temporal.PlainDate.from({ year, month, day: 1 });
  const offset = (7 - first.dayOfWeek) % 7;
  return first.add({ days: offset + (nth - 1) * 7 });
}

export function holidayDate(
  kind: string | null | undefined,
  year: number,
): Temporal.PlainDate | null {
  if (kind === "mothers") return nthSundayOf(year, 5, 2);
  if (kind === "fathers") return nthSundayOf(year, 6, 3);
  return null;
}

/** Latest holiday occurrence at or before `now` (this year if passed, otherwise last year). */
export function lastHolidayOccurrence(
  kind: string | null | undefined,
  now: Temporal.PlainDate,
): Temporal.PlainDate | null {
  const thisYear = holidayDate(kind, now.year);
  if (!thisYear) return null;
  if (Temporal.PlainDate.compare(thisYear, now) > 0) {
    return holidayDate(kind, now.year - 1);
  }
  return thisYear;
}

/** Next holiday occurrence at or after `now` (may roll into next year). */
export function nextHolidayOccurrence(
  kind: string | null | undefined,
  now: Temporal.PlainDate,
): Temporal.PlainDate | null {
  const thisYear = holidayDate(kind, now.year);
  if (!thisYear) return null;
  if (Temporal.PlainDate.compare(thisYear, now) >= 0) return thisYear;
  return holidayDate(kind, now.year + 1);
}
