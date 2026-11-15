/** Whether any filter of a list is set: an empty list without one means there is nothing yet, not a filter to change
(UI/UX loop R62). */
export function anySet(...values: (string | boolean | null | undefined)[]): boolean {
  return values.some((v) => v === true || (typeof v === "string" && v !== ""));
}
