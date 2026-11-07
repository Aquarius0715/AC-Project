// Text for a value that may be missing, so a template never prints "null" or "undefined".

/** orDash is the text, or "—" when there is none. */
export const orDash = (text: string | null | undefined): string => text ?? "—";

/** characters splits `text` into Unicode code points (an emoji is one, not two UTF-16 halves). */
export const characters = (text: string): string[] => Array.from(text.matchAll(/./gsu), (match) => match[0]);

/** codePoints is the length of `text` in Unicode code points, as the Core API counts a text's length. */
export const codePoints = (text: string): number => characters(text).length;

/** wrapped is `value` between `before` and `after`, or nothing when there is no value (an optional part of a sentence). */
export const wrapped = (value: string | null | undefined, before: string, after = ""): string => (value ? `${before}${value}${after}` : "");

/** isEmail tells whether `text` has the shape of an address: one @, a local part, and a dot inside the domain (the same test as /^[^\s@]+@[^\s@]+\.[^\s@]+$/, without a pattern that can backtrack). */
export const isEmail = (text: string): boolean => {
  const parts = text.split("@");
  if (parts.length !== 2 || /\s/.test(text)) return false;
  const [local = "", domain = ""] = parts;
  return local !== "" && domain.slice(1, -1).includes(".");
};

/** withoutNote drops a trailing "(…)" note and the space before it (the same as replacing /\s*\([^)]*\)$/, without a pattern that can backtrack). */
export const withoutNote = (text: string): string => {
  if (!text.endsWith(")")) return text;
  const open = text.indexOf("(", text.slice(0, -1).lastIndexOf(")") + 1);
  return open === -1 ? text : text.slice(0, open).trimEnd();
};

/** withoutYear drops a trailing four-digit year and the space before it (the same as replacing /\s*\d{4}$/). */
export const withoutYear = (text: string): string => (/^\d{4}$/.test(text.slice(-4)) ? text.slice(0, -4).trimEnd() : text);
