// Text for a value that may be missing, so a template never prints "null" or "undefined".

/** orDash is the text, or "—" when there is none. */
export const orDash = (text: string | null | undefined): string => text ?? "—";

/** characters splits `text` into Unicode code points (an emoji is one, not two UTF-16 halves). */
export const characters = (text: string): string[] => Array.from(text.matchAll(/./gsu), (match) => match[0]);

/** codePoints is the length of `text` in Unicode code points, as the Core API counts a text's length. */
export const codePoints = (text: string): number => characters(text).length;

/** wrapped is `value` between `before` and `after`, or nothing when there is no value (an optional part of a sentence). */
export const wrapped = (value: string | null | undefined, before: string, after = ""): string => (value ? `${before}${value}${after}` : "");
