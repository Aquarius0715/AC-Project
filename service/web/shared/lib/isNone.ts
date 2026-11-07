/** isNone is `value == null` spelled out: the value is null or undefined. Where a type allows only one of the two, the
comparison names it (`=== null`, `=== undefined`); this is for a value that can be either (an optional chain's result, a
field the Core API leaves out or sends as null). */
export function isNone(value: unknown): value is null | undefined {
  return value === null || value === undefined;
}
