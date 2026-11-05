/** The member of `options` equal to `value` (a form control hands back plain text), or `fallback` when none is. */
export function oneOf<T extends string, F>(options: readonly T[], value: string, fallback: F): T | F {
  return options.find((option) => option === value) ?? fallback;
}
