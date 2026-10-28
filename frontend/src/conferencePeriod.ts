export type ConferencePeriod = "all" | "7" | "30" | "90";

/**
 * Returns inclusive local calendar dates for a quick meeting filter.
 * @param period Number of days including today, or all to clear both bounds.
 * @param now Local reference date, injectable for deterministic tests.
 */
export function conferencePeriodRange(
  period: ConferencePeriod,
  now = new Date(),
): { from: string; to: string } {
  if (period === "all") return { from: "", to: "" };
  const start = new Date(now);
  start.setDate(start.getDate() - Number(period) + 1);
  return { from: localDate(start), to: localDate(now) };
}

/** Formats a local date for date inputs without shifting it through UTC. */
function localDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}
