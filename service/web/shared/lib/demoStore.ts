// Where the Phase 1A demo stores keep their state (FR-X05, IR257): one tab, so the four apps of a tab see the same demo
// data when they share an origin and switching roles keeps it (sessionStorage is per tab), while a reload — or the reset
// on /demo — brings back the seed.

import type { z } from "zod";

/** True when this document was opened by a reload (the reload restores the seed). */
function reloaded(): boolean {
  try {
    return (performance.getEntriesByType("navigation")[0] as PerformanceNavigationTiming | undefined)?.type === "reload";
  } catch {
    return false;
  }
}

/** The saved demo value, or null for the seed: nothing saved, blocked storage, or a reload. */
export function readDemo<T>(key: string): T | null {
  try {
    if (reloaded()) {
      sessionStorage.removeItem(key);
      return null;
    }
    const raw = sessionStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

/** The saved demo value when it still has the shape `schema` describes; anything else (an older release's data, a
tampered value) brings back the seed, as nothing saved does. */
export function readDemoAs<T>(key: string, schema: z.ZodType<T>): T | null {
  const saved = readDemo<unknown>(key);
  const parsed = schema.safeParse(saved);
  return parsed.success ? parsed.data : null;
}

export function writeDemo(key: string, value: unknown): void {
  try {
    sessionStorage.setItem(key, JSON.stringify(value));
  } catch {
    // blocked storage: the demo keeps working in memory
  }
}
