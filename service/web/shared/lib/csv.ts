// CSV export cells (IR368). A cell that begins with "=", "+", "-", "@", a tab or a carriage return is read as a formula
// when a spreadsheet opens the file (CSV injection, CWE-1236). Names come from users and from uploaded import files,
// so such a cell is kept as text with a leading apostrophe; a plain number such as -12.5 stays a number.
const FORMULA_START = /^[=+\-@\t\r]/;
const PLAIN_NUMBER = /^[+-]?\d+(\.\d+)?$/;

/** The text of a cell, neutralized when a spreadsheet would run it as a formula. */
export function csvText(v: string | number): string {
  const s = String(v);
  return FORMULA_START.test(s) && !PLAIN_NUMBER.test(s) ? `'${s}` : s;
}

/** A cell quoted only when it needs it (a quote, comma or line break inside). */
export function csvCell(v: string | number): string {
  const s = csvText(v);
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

/** A cell always quoted. */
export const csvQuoted = (v: string | number): string => `"${csvText(v).replace(/"/g, '""')}"`;
