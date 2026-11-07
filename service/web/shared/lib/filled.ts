/** A text with something in it, or undefined: an empty text (a cleared URL parameter or environment variable) counts as absent. */
export const filled = (s: string | null | undefined): string | undefined => (s === null || s === undefined || s === "" ? undefined : s);
