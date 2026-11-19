// The setpoint range every chosen unit accepts.

/** commonRange is the range inside each unit's own temperature range (the highest minimum to the lowest maximum), or
null when no unit states one. A unit without a range does not narrow it. When the ranges do not overlap the result is
empty (min above max): see sharesNoSetpoint. */
export function commonRange(units: readonly { temperature?: { min: number; max: number } | null }[]): { min: number; max: number } | null {
  const ranges = units.flatMap((u) => (u.temperature ? [u.temperature] : []));
  return ranges.length === 0 ? null : { min: Math.max(...ranges.map((r) => r.min)), max: Math.min(...ranges.map((r) => r.max)) };
}

/** sharesNoSetpoint tells whether a common range is empty: no setpoint suits every chosen unit. */
export const sharesNoSetpoint = (range: { min: number; max: number }): boolean => range.min > range.max;
