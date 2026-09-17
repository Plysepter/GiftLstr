import { describe, expect, it } from "vitest";
import { formatMoney, centsToDollars, dollarsToCents } from "./money.js";

describe("formatMoney", () => {
  it("formats whole dollars", () => {
    expect(formatMoney(1200)).toBe("$12.00");
  });
  it("formats cents", () => {
    expect(formatMoney(1234)).toBe("$12.34");
  });
  it("groups thousands", () => {
    expect(formatMoney(155000)).toBe("$1,550.00");
  });
  it("formats zero", () => {
    expect(formatMoney(0)).toBe("$0.00");
  });
});

describe("centsToDollars", () => {
  it("renders two decimals always", () => {
    expect(centsToDollars(1234)).toBe("12.34");
    expect(centsToDollars(1200)).toBe("12.00");
    expect(centsToDollars(0)).toBe("0.00");
  });
});

describe("dollarsToCents", () => {
  it("converts a dollar amount to cents", () => {
    expect(dollarsToCents("12.34")).toBe(1234);
    expect(dollarsToCents("10")).toBe(1000);
  });
  it("strips $, commas and whitespace", () => {
    expect(dollarsToCents("  $1,234.50  ")).toBe(123450);
  });
  it("treats empty input as zero (clears price)", () => {
    expect(dollarsToCents("")).toBe(0);
    expect(dollarsToCents("   ")).toBe(0);
  });
  it("trims to whole cents (never rounds)", () => {
    expect(dollarsToCents("12.345")).toBe(1234);
    expect(dollarsToCents("12.349")).toBe(1234);
    expect(dollarsToCents("12")).toBe(1200);
  });
  it("rejects invalid values", () => {
    expect(dollarsToCents("abc")).toBeNull();
    expect(dollarsToCents("-5")).toBeNull();
  });
});
