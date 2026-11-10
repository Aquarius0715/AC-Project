/** A text field of a submitted form: its value, or "" when the field is missing or holds a file. */
export function formText(form: FormData, key: string): string {
  const value = form.get(key);
  return typeof value === "string" ? value : "";
}
