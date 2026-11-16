/** Whether this caller can open the Restrictions screen: the full manager (read or write) or the override-only view.
A link to a restriction is shown only then; otherwise the screen would only say which access is missing (UI/UX loop R57). */
export function canOpenRestrictions(perms: ReadonlySet<string>): boolean {
  return ["restriction.read", "restriction.write", "restriction.override"].some((p) => perms.has(p));
}
