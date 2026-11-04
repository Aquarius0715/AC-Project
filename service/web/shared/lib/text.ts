// Text for a value that may be missing, so a template never prints "null" or "undefined".

/** orDash is the text, or "—" when there is none. */
export const orDash = (text: string | null | undefined): string => text ?? "—";
