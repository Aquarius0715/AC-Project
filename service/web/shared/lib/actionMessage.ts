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
  // the address belongs to someone who still has access, here or elsewhere (IR461, IR462)
  "errors.email_taken": "{field} belongs to someone who still has access — use another address or ask HQ",
  // HQ ended this person's access: only HQ gives it back, not a new account (IR508)
  "errors.revoked_by_hq": "HQ ended this person's access — only HQ can give it back",
  // a QR check-in with another unit's label, or one the technician's units do not have (jobs.checkIn, IR486)
  "errors.qr_unit_mismatch": "The scanned QR label is not this job's unit — scan the label on this AC, or check in manually with a reason",
};
// the refusals of the user management and assignment (IR416, IR417, IR420, IR421, IR433) say what to do
const keyText: Record<string, string> = {
  "errors.own_account": "You can’t change or remove your own account",
  "errors.last_owner": "The last active owner can’t be demoted, disabled or removed",
  "errors.last_contractor": "Your company keeps one active contractor — this one can’t be disabled or removed",
  // losing the permission, ending or starting later: the tenant keeps someone who can manage access (IR472)
  "errors.last_identity_admin": "At least one active membership must keep identity.write — give it to another HQ membership first, then change this one",
  "errors.access_expired": "This account’s access period has ended — set a new access end first",
  // HQ ended the access in Access & roles: no access end, enabling or role change brings it back (IR508)
  "errors.revoked_by_hq": "HQ ended this account's access — only HQ can give it back",
  // one card attempt at a time, and a paid invoice is not paid again (IR447, IR474)
  "errors.invoice_has_payment": "This invoice already has a payment in progress or recorded — finish or fail that one first",
  // an invoice of the contract already covers part of the period (IR469)
  "errors.invoice_period_exists": "This contract already has an invoice for part of this period — choose a period after it",
  "errors.technician_booked": "This technician still has an assigned job — reassign it first",
  "errors.technician_access_ends": "This technician's access ends before the visit — choose another technician or ask HQ to extend it",
  "errors.invite_pending": "This user hasn’t signed in yet — remove them, or wait until they have",
  "errors.invite_not_pending": "Only a user who hasn’t signed in yet gets the set-up e-mail again",
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

/** A field of a refusal in words: its last part, humanized ("newUser.email" → "Email"). */
export const fieldName = (field: string) => humanize(field.split(".").pop() ?? field);

export function actionMessage(r: ActionFailure, t: T = translator("en")): string {
  const [field, key] = Object.entries(r.fieldErrors)[0] ?? [];
  if (field && key) {
    const name = fieldName(field);
    return fieldText[key] ? t(fieldText[key], { field: name }) : t("{field} — {problem}", { field: name, problem: humanize(key).toLowerCase() });
  }
  if (r.messageKey === "error.versionConflict") return t("Someone changed this in the meantime — the screen now shows the latest version");
  // the write limit per member (IR375): how long to wait, as the implementation contracts ask for RATE_LIMITED
  if (r.code === "RATE_LIMITED") {
    // the set-up e-mail goes at most every 5 minutes per user (IR423, IR461)
    // a contractor or a customer owner adds at most 30 accounts a day (IR464)
    if (r.messageKey === "errors.too_many_accounts") {
      return t("Many users were added in the last 24 hours — you can add more in {h} h", { h: Math.max(1, Math.ceil((r.retryAfterSeconds ?? 86400) / 3600)) });
    }
    if (r.messageKey === "errors.invite_recently_sent") {
      return t("The set-up e-mail was sent a moment ago — send it again in {m} min", { m: Math.max(1, Math.ceil((r.retryAfterSeconds ?? 300) / 60)) });
    }
    return r.retryAfterSeconds ? t("Too many changes in a short time — try again in {s} s", { s: r.retryAfterSeconds }) : t("Too many changes in a short time — try again shortly");
  }
  if (keyText[r.messageKey]) return t(keyText[r.messageKey]);
  if (r.messageKey.startsWith("errors.")) return humanize(r.messageKey);
  return codeText[r.code] ? t(codeText[r.code]) : `${r.code}: ${humanize(r.messageKey)}`;
}
