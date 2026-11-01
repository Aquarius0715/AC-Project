/** After a failed save, takes focus to the field to fix (IR573, IR586): the element with `id` when it is a control,
 * else the first control inside it (a set of checkboxes or choice buttons). Focusing scrolls the field into view, and
 * the root's scroll padding keeps it below the sticky header (globals.css). Runs after the error texts render. */
export function focusField(id: string): void {
  requestAnimationFrame(() => {
    const el = document.getElementById(id);
    const control = "input, select, textarea, button, [role=checkbox]";
    (el?.matches(control) ? el : el?.querySelector<HTMLElement>(control))?.focus();
  });
}
