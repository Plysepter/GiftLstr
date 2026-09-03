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
