import { roundAway } from "@ac/web/lib/energy";

/** The y range of the chart: the values and the guide line with some room around them. */
export function axisRange(points: (number | null)[], guide: number | null): { min: number; max: number } {
  const vals = [...points.filter((p): p is number => p !== null), ...(guide === null ? [] : [guide])];
  if (vals.length === 0) return { min: 0, max: 1 };
  const lo = Math.min(...vals);
  const hi = Math.max(...vals);
  const pad = Math.max((hi - lo) * 0.15, Math.abs(hi) * 0.05, 1);
  return { min: Math.floor(lo - pad), max: Math.ceil(hi + pad) };
}
/** About four round y-axis grid values inside [min, max] (steps of 1, 2 or 5 × 10ⁿ). */
export function axisTicks(min: number, max: number): number[] {
  const raw = (max - min) / 4;
  const mag = 10 ** Math.floor(Math.log10(raw || 1));
  const stepSize = [1, 2, 5, 10].map((f) => f * mag).find((s) => s >= raw) ?? 10 * mag;
  const out: number[] = [];
  for (let t = Math.ceil(min / stepSize) * stepSize; t <= max; t += stepSize) out.push(roundAway(t, 6));
  return out;
}

/** Which side of a chart each unit is read on: the first unit on the left axis, a second one on the right, every other
unit not at all (a chart has two axes, IR689): a unit missing from the map has no axis. */
export function unitAxes(units: readonly string[]): Map<string, "left" | "right"> {
  const out = new Map<string, "left" | "right">();
  for (const unit of units) {
    if (out.has(unit)) continue;
    if (out.size === 0) out.set(unit, "left");
    else if (out.size === 1) out.set(unit, "right");
  }
  return out;
}

/** The selection after one series is pressed: taken out (never the last one) or put in; null when its unit would be a
third on a chart with two axes. `unitOf` names the unit a series is read in. */
export function toggleSeries<T extends string>(selected: readonly T[], key: T, unitOf: (key: T) => string): T[] | null {
  if (selected.includes(key)) return selected.length > 1 ? selected.filter((k) => k !== key) : [...selected];
  const next = [...selected, key];
  const units = next.map((k) => unitOf(k));
  return unitAxes(units).size >= new Set(units).size ? next : null;
}
