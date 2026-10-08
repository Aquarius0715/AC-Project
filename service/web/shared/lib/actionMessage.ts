// Readable text for a failed Server Action (DATA_SOURCE=api): the ServiceError code, message key and field errors of
// the Core API turned into one toast line. Pure code shared by every client view that calls Server Actions.
export type ActionFailure = { code: string; messageKey: string; fieldErrors: Record<string, string> };

const fieldText: Record<string, string> = {
  "error.required": "is required",
  "error.length": "is too short or too long",
  "error.invalid": "is not valid",
  "error.range": "is out of range",
  "error.past": "must not be in the past",
  "error.future": "must not be in the future",
  "error.duplicate": "is already used",
  "error.count": "has the wrong number of entries",
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

export function actionMessage(r: ActionFailure): string {
  const [field, key] = Object.entries(r.fieldErrors)[0] ?? [];
  if (field && key) return `${humanize(field.split(".").pop() ?? field)} ${fieldText[key] ?? `— ${humanize(key).toLowerCase()}`}`;
  if (r.messageKey === "error.versionConflict") return "Someone changed this in the meantime — the screen now shows the latest version";
  if (r.messageKey.startsWith("errors.")) return humanize(r.messageKey);
  return codeText[r.code] ?? `${r.code}: ${humanize(r.messageKey)}`;
}
