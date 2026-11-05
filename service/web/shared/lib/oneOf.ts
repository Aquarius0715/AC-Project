/** The member of `options` equal to `value` (a form control hands back plain text), or `fallback` when none is. */
export function oneOf<T extends string, F>(options: readonly T[], value: string, fallback: F): T | F {
  return options.find((option) => option === value) ?? fallback;
}

/** Like oneOf, for a value that must be a member: anything else is a programming or data error, not a default. */
export function oneOfOrThrow<T extends string>(options: readonly T[], value: string, what: string): T {
  const found = options.find((option) => option === value);
  if (found === undefined) throw new Error(`${what} "${value}" is not one of ${options.join(", ")}`);
  return found;
}
