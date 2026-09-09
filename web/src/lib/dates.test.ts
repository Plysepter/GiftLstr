import { describe, expect, it } from "vitest";
import { Temporal } from "temporal-polyfill";
import {
  lastResetOccurrence,
  nthSundayOf,
  holidayDate,
  lastHolidayOccurrence,
  nextHolidayOccurrence,
} from "./dates.js";

describe("lastResetOccurrence", () => {
  it("returns this year when the date has passed", () => {
    const now = Temporal.PlainDate.from("2026-08-22");
    expect(lastResetOccurrence("12-25", now).toString()).toBe("2025-12-25");
  });

  it("handles birthdays earlier in the year", () => {
    const now = Temporal.PlainDate.from("2026-03-10");
    expect(lastResetOccurrence("03-05", now).toString()).toBe("2026-03-05");
  });
});

describe("nthSundayOf", () => {
  it("finds the 1st/2nd Sunday of May 2026 (May 1 2026 is a Friday)", () => {
    expect(nthSundayOf(2026, 5, 1).toString()).toBe("2026-05-03");
    expect(nthSundayOf(2026, 5, 2).toString()).toBe("2026-05-10");
  });

  it("finds the 2nd Sunday of May 2027 (Mother's Day)", () => {
    expect(nthSundayOf(2027, 5, 2).toString()).toBe("2027-05-09");
  });

  it("finds the 3rd Sunday of June for Father's Day", () => {
    expect(nthSundayOf(2026, 6, 3).toString()).toBe("2026-06-21");
    expect(nthSundayOf(2025, 6, 3).toString()).toBe("2025-06-15");
  });

  it("handles a month that starts on a Sunday", () => {
    expect(nthSundayOf(2023, 1, 1).toString()).toBe("2023-01-01");
    expect(nthSundayOf(2023, 1, 2).toString()).toBe("2023-01-08");
  });
});

describe("holidayDate", () => {
  it("maps mothers/fathers to their dates and rejects unknown kinds", () => {
    expect(holidayDate("mothers", 2026)!.toString()).toBe("2026-05-10");
    expect(holidayDate("fathers", 2026)!.toString()).toBe("2026-06-21");
    expect(holidayDate("", 2026)).toBeNull();
    expect(holidayDate("christmas", 2026)).toBeNull();
  });
});

describe("lastHolidayOccurrence", () => {
  it("returns this year once the holiday has passed", () => {
    const now = Temporal.PlainDate.from("2026-05-15");
    expect(lastHolidayOccurrence("mothers", now)!.toString()).toBe("2026-05-10");
  });

  it("falls back to the previous year when the holiday is upcoming", () => {
    const now = Temporal.PlainDate.from("2026-04-15");
    expect(lastHolidayOccurrence("mothers", now)!.toString()).toBe("2025-05-11");
  });

  it("handles the day of the holiday itself", () => {
    const now = Temporal.PlainDate.from("2026-05-10");
    expect(lastHolidayOccurrence("mothers", now)!.toString()).toBe("2026-05-10");
  });

  it("returns null for an unknown kind", () => {
    expect(lastHolidayOccurrence("", Temporal.PlainDate.from("2026-05-10"))).toBeNull();
  });
});

describe("nextHolidayOccurrence", () => {
  it("returns the current-year holiday when still ahead", () => {
    const now = Temporal.PlainDate.from("2026-06-20");
    expect(nextHolidayOccurrence("fathers", now)!.toString()).toBe("2026-06-21");
  });

  it("rolls into next year after the holiday", () => {
    const now = Temporal.PlainDate.from("2026-06-22");
    expect(nextHolidayOccurrence("fathers", now)!.toString()).toBe("2027-06-20");
  });
});
