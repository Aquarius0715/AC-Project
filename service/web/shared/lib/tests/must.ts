/** A value a test needs: the value itself, or a failure that names what was missing (instead of a `!` assertion). */
export function must<T>(value: T | null | undefined, what = "a value the test needs"): T {
  if (value === null || value === undefined) throw new Error(`missing ${what}`);
  return value;
}
