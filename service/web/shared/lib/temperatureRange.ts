// The setpoint range every chosen unit accepts.

/** commonRange is the range inside each unit's own temperature range (the highest minimum to the lowest maximum), or
null when no unit states one. A unit without a range does not narrow it. */
export function commonRange(units: readonly { temperature?: { min: number; max: number } | null }[]): { min: number; max: number } | null {
  const ranges = units.flatMap((u) => (u.temperature ? [u.temperature] : []));
  if (ranges.length === 0) return null;
  return { min: Math.max(...ranges.map((r) => r.min)), max: Math.min(...ranges.map((r) => r.max)) };
}
