// An alert's or observation's evidence text, and the resolution reasons the system writes, in the display language (IR480). The API keeps the evidence as English text,
// written by the seed (the fixture's demo observations), the filter-care reminders, the policy rules and the tamper and
// reconciliation alerts. The texts those sources write are shown in the user's language; any other text (a future source,
// or one a device sends) stays as stored. Names, serials, metrics and numbers inside a text are kept as they are.
import type { Params, T } from "@ac/web/lib/i18n";

/** Texts written as they are. */
const FIXED = new Set([
  "Cleaning due. Not a fault.",
  "Device still reports an ended restriction; reconcile it from HQ",
  "Demo insulation",
  "Demo observation",
  "Demo: cooling load rose while the outdoor temperature was stable.",
  "Demo: cooling load rose; no cause evidence is available (fictional).",
  "Demo: high temperature observed (fictional)",
  "Demo: inspection recorded worn insulation around the indoor unit piping (fictional).",
  "Demo: synthetic allergen observation without unit (fictional).",
  "Demo: synthetic allergen observation; not a health assessment.",
  "Demo: temperature at or above 30°C for 60 seconds (fictional)",
]);

/** Texts written with values in them: the stored form, the dictionary key and its values. */
const TEMPLATES: [RegExp, string, (m: RegExpMatchArray) => Params][] = [
  [/^Cleaning due \((\d+) h of run time since the last cleaning\)\. Not a fault\.$/, "Cleaning due ({hours} h of run time since the last cleaning). Not a fault.", (m) => ({ hours: m[1] })],
  [/^Cleaning due \((\d+) h of run time, no cleaning recorded\)\. Not a fault\.$/, "Cleaning due ({hours} h of run time, no cleaning recorded). Not a fault.", (m) => ({ hours: m[1] })],
  [/^Cleaning due \((\d+) days since the last cleaning, run time unknown\)\. Not a fault\.$/, "Cleaning due ({days} days since the last cleaning, run time unknown). Not a fault.", (m) => ({ days: m[1] })],
  [/^Device (\S+) reported its cover opened while powered \(tamper_signal\)$/, "Device {serial} reported its cover opened while powered (tamper_signal)", (m) => ({ serial: m[1] })],
  [/^Demo event: (\S+) (\S+) threshold$/, "Demo event: {metric} {operator} threshold", (m) => ({ metric: m[1], operator: m[2] })],
];

/** Resolution reasons the system writes: a cleaned filter, a recovery case resolved, a policy's sustained recovery
 * (D08). A reason a person typed when resolving stays as written. */
const REASONS = new Set(["filter cleaned", "recovery resolved"]);
const REASON_TEMPLATES: typeof TEMPLATES = [
  [/^Recovered: (\S+) (\S+) ([<>]) (\S+) for (\d+) s \(D08\)$/, "Recovered: {metric} {value} {dir} {threshold} for {seconds} s (D08)", (m) => ({ metric: m[1], value: m[2], dir: m[3], threshold: m[4], seconds: m[5] })],
];

function known(stored: string, t: T, fixed: Set<string>, templates: typeof TEMPLATES): string {
  if (fixed.has(stored)) return t(stored);
  for (const [form, key, values] of templates) {
    const m = stored.match(form);
    if (m) return t(key, values(m));
  }
  return stored;
}

/** The evidence text in the display language; a text no source above writes stays as stored. */
export const evidenceText = (stored: string, t: T): string => known(stored, t, FIXED, TEMPLATES);

/** An alert's resolution reason in the display language; a person's own reason stays as written. */
export const resolutionText = (stored: string, t: T): string => known(stored, t, REASONS, REASON_TEMPLATES);

/** The dictionary keys this module uses (the key check of the tests). */
export const EVIDENCE_KEYS = [...FIXED, ...TEMPLATES.map(([, key]) => key), ...REASONS, ...REASON_TEMPLATES.map(([, key]) => key)];
