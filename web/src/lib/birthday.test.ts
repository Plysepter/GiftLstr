import { describe, expect, it } from "vitest";
import { parseBday, formatBday, BDAY_DAYS } from "./birthday.js";

describe("parseBday", () => {
  it("splits a canonical MM-DD value and strips day padding", () => {
    expect(parseBday("05-15")).toEqual({ m: "05", d: "15" });
    expect(parseBday("01-01")).toEqual({ m: "01", d: "1" });
  });

  it("returns empty parts for missing or malformed values", () => {
    expect(parseBday("")).toEqual({ m: "", d: "" });
    expect(parseBday("junk")).toEqual({ m: "", d: "" });
    expect(parseBday("05")).toEqual({ m: "", d: "" });
  });
});

describe("formatBday", () => {
  it("pads the day to two digits", () => {
    expect(formatBday("05", "5")).toBe("05-05");
    expect(formatBday("05", "15")).toBe("05-15");
  });

  it("round-trips through parseBday", () => {
    expect(parseBday(formatBday("07", "4")).m).toBe("07");
    expect(parseBday(formatBday("07", "4")).d).toBe("4");
  });
});

describe("BDAY_DAYS", () => {
  it("offers every day of the month", () => {
    expect(BDAY_DAYS.length).toBe(31);
    expect(BDAY_DAYS[0]).toBe("1");
    expect(BDAY_DAYS[30]).toBe("31");
  });
});
