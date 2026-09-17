/**
 * Formats an amount in integer cents as a currency string, e.g. 1234 -> "$12.34".
 *
 * The currency symbol is intentionally hard-coded to USD because the app is a
 * personal gift tracker — but keeping formatting here means switching to a
 * locale-aware formatter later is a one-line change.
 */
export function formatMoney(cents: number): string {
  return (cents / 100).toLocaleString("en-US", {
    style: "currency",
    currency: "USD",
  });
}

/** Renders cents for a price input, e.g. 1234 -> "12.34". */
export function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2);
}

/**
 * Parses a user-typed dollar amount into integer cents. Strips "$", commas and
 * whitespace; empty input means zero (clears a price). Values with more than
 * two decimals are trimmed (never rounded) so no fractions of a cent can be
 * stored. Returns null when the value is not a finite non-negative number.
 */
export function dollarsToCents(input: string): number | null {
  const s = input.trim();
  if (s === "") return 0;
  const clean = s.replace(/[$,\s]/g, "");
  const n = Number(clean);
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.trunc(n * 100);
}
