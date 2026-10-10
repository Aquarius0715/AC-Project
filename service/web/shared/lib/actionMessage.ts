// Readable text for a failed Server Action (DATA_SOURCE=api): the ServiceError code, message key and field errors of
// the Core API turned into one toast line. Pure code shared by every client view that calls Server Actions. The line
// is in the display language (`t`, IR269); a field name and a domain message key are humanized English, because the
// API sends keys, not texts.
import { translator, type T } from "@ac/web/lib/i18n";

export type ActionFailure = { code: string; messageKey: string; fieldErrors: Record<string, string> };

const fieldText: Record<string, string> = {
  "error.required": "{field} is required",
  "error.length": "{field} is too short or too long",
  "error.invalid": "{field} is not valid",
  "error.range": "{field} is out of range",
  "error.past": "{field} must not be in the past",
  "error.future": "{field} must not be in the future",
  "error.duplicate": "{field} is already used",
  "error.count": "{field} has the wrong number of entries",
  "error.invalidFile": "{field} is not an accepted file — check its type and size", // the bytes, the type or the size (IR308)
};
const codeText: Record<string, string> = {
  FORBIDDEN: "You don't have permission for this action",
  NOT_FOUND: "This item no longer exists or is outside your scope",
  UNAVAILABLE: "The service is unavailable — try again shortly",
  UNAUTHENTICATED: "Your session ended — sign in again",
  VALIDATION: "Check the entered values",
};

/** "invoice_has_payment" → "Invoice has payment"; "paymentReference" → "Payment reference". */
const humanize = (s: string) => {
  const t = s.replace(/^errors?\./, "").replace(/[._]/g, " ").replace(/([a-z])([A-Z])/g, "$1 $2").toLowerCase().trim();
  return t.charAt(0).toUpperCase() + t.slice(1);
};

export function actionMessage(r: ActionFailure, t: T = translator("en")): string {
  const [field, key] = Object.entries(r.fieldErrors)[0] ?? [];
  if (field && key) {
    const name = humanize(field.split(".").pop() ?? field);
    return fieldText[key] ? t(fieldText[key], { field: name }) : t("{field} — {problem}", { field: name, problem: humanize(key).toLowerCase() });
  }
  if (r.messageKey === "error.versionConflict") return t("Someone changed this in the meantime — the screen now shows the latest version");
  if (r.messageKey.startsWith("errors.")) return humanize(r.messageKey);
  return codeText[r.code] ? t(codeText[r.code]) : `${r.code}: ${humanize(r.messageKey)}`;
}
