import { describe, expect, it } from "vitest";
import { Temporal } from "temporal-polyfill";
import { lastResetOccurrence } from "./dates.js";

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
