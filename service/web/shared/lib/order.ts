// Comparators for toSorted: a sort without one compares numbers as text (10 before 9), and text by UTF-16 code units
// rather than the reader's order.

/** byText orders strings as the locale does. */
export const byText = (a: string, b: string): number => a.localeCompare(b);

/** byNumber orders numbers by value. */
export const byNumber = (a: number, b: number): number => a - b;

// byCodeUnit orders strings as a sort without a comparator does, by character codes (the same order for text without
// characters beyond U+FFFF, such as field names): for generated files, which must not change with the locale.
export const byCodeUnit = (a: string, b: string): number => {
  const shorter = Math.min(a.length, b.length);
  for (let index = 0; index < shorter; index++) {
    const difference = (a.codePointAt(index) ?? 0) - (b.codePointAt(index) ?? 0);
    if (difference !== 0) return difference;
  }
  return a.length - b.length;
};
