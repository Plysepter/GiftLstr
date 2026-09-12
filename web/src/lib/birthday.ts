export interface MonthOption {
  v: string;
  l: string;
}

/** Full month names, first option empty (used by the profile form and first-run modal). */
export const BDAY_MONTHS: ReadonlyArray<MonthOption> = [
  { v: "", l: "" },
  { v: "01", l: "January" },
  { v: "02", l: "February" },
  { v: "03", l: "March" },
  { v: "04", l: "April" },
  { v: "05", l: "May" },
  { v: "06", l: "June" },
  { v: "07", l: "July" },
  { v: "08", l: "August" },
  { v: "09", l: "September" },
  { v: "10", l: "October" },
  { v: "11", l: "November" },
  { v: "12", l: "December" },
];

/** Abbreviated month names, first option empty (keeps the admin table compact). */
export const BDAY_MONTHS_SHORT: ReadonlyArray<MonthOption> = [
  { v: "", l: "" },
  { v: "01", l: "Jan" },
  { v: "02", l: "Feb" },
  { v: "03", l: "Mar" },
  { v: "04", l: "Apr" },
  { v: "05", l: "May" },
  { v: "06", l: "Jun" },
  { v: "07", l: "Jul" },
  { v: "08", l: "Aug" },
  { v: "09", l: "Sep" },
  { v: "10", l: "Oct" },
  { v: "11", l: "Nov" },
  { v: "12", l: "Dec" },
];

export const BDAY_DAYS: readonly string[] = Array.from({ length: 31 }, (_, i) => String(i + 1));

/** Extra archive/reset holidays besides the birthday and Christmas. */
export const HOLIDAY_OPTIONS: ReadonlyArray<{ v: string; l: string }> = [
  { v: "", l: "Off" },
  { v: "mothers", l: "Mother's Day (2nd Sunday of May)" },
  { v: "fathers", l: "Father's Day (3rd Sunday of June)" },
];

/** Splits a canonical "MM-DD" birthday into its parts (day returned without padding). */
export function parseBday(b: string): { m: string; d: string } {
  const parts = b.split("-");
  if (parts.length === 2 && parts[0] && parts[1]) {
    return { m: parts[0], d: String(parseInt(parts[1], 10)) };
  }
  return { m: "", d: "" };
}

/** Builds a canonical "MM-DD" value from a month and day select pair. */
export function formatBday(month: string, day: string): string {
  return `${month}-${day.padStart(2, "0")}`;
}
