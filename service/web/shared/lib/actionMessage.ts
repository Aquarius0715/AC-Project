// Readable text for a failed Server Action (DATA_SOURCE=api): the ServiceError code, message key and field errors of
// the Core API turned into one toast line. Pure code shared by every client view that calls Server Actions. The line
// is in the display language (`t`, IR269); a field name and a domain message key are humanized English, because the
// API sends keys, not texts.
import { translator, type T } from "@ac/web/lib/i18n";

export type ActionFailure = { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

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
// the refusals of the user management (IR416, IR417, IR420, IR421) say what to do
const keyText: Record<string, string> = {
  "errors.own_account": "You can’t change or remove your own account",
  "errors.last_owner": "The last active owner can’t be demoted, disabled or removed",
  "errors.last_contractor": "Your company keeps one active contractor — this one can’t be disabled or removed",
  "errors.access_expired": "This account’s access period has ended — ask HQ to extend it first",
  "errors.technician_booked": "This technician still has an assigned job — reassign it first",
  "errors.invite_pending": "The invite is still pending — it becomes active when the account is opened",
  "errors.invite_not_pending": "Only a pending invite can be sent again",
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
  // the write limit per member (IR375): how long to wait, as the implementation contracts ask for RATE_LIMITED
  if (r.code === "RATE_LIMITED") {
    return r.retryAfterSeconds ? t("Too many changes in a short time — try again in {s} s", { s: r.retryAfterSeconds }) : t("Too many changes in a short time — try again shortly");
  }
  if (keyText[r.messageKey]) return t(keyText[r.messageKey]);
  if (r.messageKey.startsWith("errors.")) return humanize(r.messageKey);
  return codeText[r.code] ? t(codeText[r.code]) : `${r.code}: ${humanize(r.messageKey)}`;
}
