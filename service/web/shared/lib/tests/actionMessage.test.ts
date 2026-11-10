import { describe, expect, it } from "vitest";
import { actionMessage, fieldErrorTexts } from "@ac/web/lib/actionMessage";
import { translator } from "@ac/web/lib/i18n";

const ERROR_LENGTH = "error.length";
const ERROR_RANGE = "error.range";
const RATE_LIMITED_2 = "RATE_LIMITED";
const ERROR_RATELIMITED = "error.rateLimited";

const failure = (over: Partial<Parameters<typeof actionMessage>[0]>) => ({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: {}, ...over });

describe("actionMessage (one toast line per failed Server Action)", () => {
  it("reads the first field error with a known or humanized key", () => {
    expect(actionMessage(failure({ fieldErrors: { reason: ERROR_LENGTH } }))).toBe("Reason is too short or too long");
    expect(actionMessage(failure({ fieldErrors: { "query.filters.to": ERROR_RANGE } }))).toBe("To is out of range");
    expect(actionMessage(failure({ fieldErrors: { calibratedAt: "error.future" } }))).toBe("Calibrated at must not be in the future");
    expect(actionMessage(failure({ fieldErrors: { unit: "errors.unit_mismatch" } }))).toBe("Unit — unit mismatch");
    expect(actionMessage(failure({ fieldErrors: { sitePhoto: "error.invalidFile" } }))).toBe("Site photo is not an accepted file — check its type and size"); // IR308
  });
  it("says a malformed or taken MQTT node ID in words (IR635)", () => {
    expect(actionMessage(failure({ fieldErrors: { mqttNode: "error.mqttNodeFormat" } }))).toBe("The MQTT node ID needs 1–64 letters, digits, hyphens or underscores");
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "error.duplicateMqttNode" }))).toBe("Another device already has this MQTT node ID");
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "error.duplicateMqttNode" }), translator("ms"))).toBe("Peranti lain sudah mempunyai ID nod MQTT ini");
  });
  it("says why an import commit is refused, in the display language (IR751, IR754)", () => {
    const inactive = failure({ code: "CONFLICT", messageKey: "error.customerInactive" });
    const expired = failure({ code: "CONFLICT", messageKey: "error.previewExpired" });
    expect([actionMessage(inactive), actionMessage(expired)]).toStrictEqual([
      "This customer is inactive — activate it first",
      "The checked file has expired or changed — check it again, then import",
    ]);
    expect(actionMessage(inactive, translator("ms"))).not.toContain("CONFLICT");
    expect(actionMessage(expired, translator("ja"))).not.toContain("CONFLICT");
  });
  it("says why a refusal without a field happened, in every display language (IR756)", () => {
    const keys = ["error.customerActive", "error.notEmpty", "error.unitActive", "error.unitInUse", "error.alreadyUndone", "error.undoWindowClosed", "error.importPlaceInUse", "error.importUnitInUse", "error.alreadyBound", "error.unitHasDevice", "error.unitBusy", "error.duplicateModel", "error.retryOnlyFailedOrSkipped", "error.idempotencyKeyReused", "error.requestInProgress"];
    for (const messageKey of keys) {
      for (const language of ["en", "ms", "ja"] as const) {
        const line = actionMessage(failure({ code: "CONFLICT", messageKey }), translator(language));
        expect(line, `${messageKey} (${language})`).not.toContain("CONFLICT");
      }
    }
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "error.unitInUse" }))).toBe("This unit has history (jobs, contracts, invoices, telemetry or devices), so it cannot be deleted — archive it instead");
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "error.undoWindowClosed" }), translator("ja"))).toBe("この取り込みを取り消せる 24 時間の期限が過ぎました");
  });
  it("words the shared refusal keys, whatever their code, in every display language (IR758)", () => {
    const cases = [["CONFLICT", "error.invalidState"], ["CONFLICT", "error.conflict"], ["TIMEOUT", "error.timeout"], ["OFFLINE", "error.offline"], ["CONFLICT", "error.deviceNotBound"], ["CONFLICT", "error.duplicateSerial"], ["CONFLICT", "error.tamperUnresolved"], ["CONFLICT", "error.duplicateName"], ["CONFLICT", "error.unitArchived"]] as const;
    for (const [code, messageKey] of cases) {
      for (const language of ["en", "ms", "ja"] as const) {
        const line = actionMessage(failure({ code, messageKey }), translator(language));
        expect(line, `${messageKey} (${language})`).not.toContain(code);
      }
    }
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "error.invalidState" }))).toBe("This is no longer possible in its current state — reload the page to see the latest state");
    expect(actionMessage(failure({ code: "TIMEOUT", messageKey: "error.timeout" }), translator("ms"))).toBe("Permintaan mengambil masa terlalu lama — semak sama ada perubahan telah disimpan, kemudian cuba lagi");
  });
  it("explains version conflicts, domain errors and bare codes", () => {
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "error.versionConflict" }))).toMatch(/changed this in the meantime/);
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "errors.operation_not_running" }))).toBe("Operation not running");
    expect(actionMessage(failure({ code: "NOT_FOUND", messageKey: "error.notFound" }))).toBe("This item no longer exists or is outside your scope");
    expect(actionMessage(failure({ code: "FORBIDDEN", messageKey: "error.forbidden" }))).toMatch(/permission/);
    // IR375: the write limit says how long to wait
    expect(actionMessage(failure({ code: RATE_LIMITED_2, messageKey: ERROR_RATELIMITED, retryAfterSeconds: 40 }))).toBe("Too many changes in a short time — try again in 40 s");
    expect(actionMessage(failure({ code: RATE_LIMITED_2, messageKey: ERROR_RATELIMITED }))).toBe("Too many changes in a short time — try again shortly");
    expect(actionMessage(failure({ code: RATE_LIMITED_2, messageKey: "errors.invite_recently_sent", retryAfterSeconds: 241 }))).toBe("The set-up e-mail was sent a moment ago — send it again in 5 min");
    // IR469: an invoice period overlapping an invoiced one says what to do
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "errors.invoice_period_exists" }))).toBe("This contract already has an invoice for part of this period — choose a period after it");
    // IR464: the accounts added per day say when more can be added (rounded up to hours)
    expect(actionMessage(failure({ code: RATE_LIMITED_2, messageKey: "errors.too_many_accounts", retryAfterSeconds: 81_060 }))).toBe("Many users were added in the last 24 hours — you can add more in 23 h");
    // IR462: an address someone with access holds says what to do; a nested field is named by its last part
    expect(actionMessage(failure({ fieldErrors: { "newUser.email": "errors.email_taken" } }))).toBe("Email belongs to someone who still has access — use another address or ask HQ");
    // a notice the person does not get on that channel reads in words, not "Recipient membership id — recipient ineligible" (IR578)
    expect(actionMessage(failure({ fieldErrors: { recipientMembershipId: "errors.recipient_ineligible" } }))).toBe("This person does not get this notice on that channel");
    // IR508: HQ ended the access — only HQ gives it back, in the display language
    expect([actionMessage(failure({ code: "CONFLICT", messageKey: "errors.revoked_by_hq" })), actionMessage(failure({ fieldErrors: { email: "errors.revoked_by_hq" } }), translator("ms"))]).toStrictEqual([
      "HQ ended this account's access — only HQ can give it back", "HQ telah menamatkan akses orang ini — hanya HQ boleh memulihkannya"]);
    // IR510: an HQ membership on another app's token
    expect(actionMessage(failure({ code: "FORBIDDEN", messageKey: "errors.hq_app_required" }))).toBe("An HQ account works in the HQ app — sign in there");
    // IR486: a QR check-in with another unit's label says what to do, in the display language
    const qr = failure({ fieldErrors: { qrUnitId: "errors.qr_unit_mismatch" } });
    expect([actionMessage(qr), actionMessage(qr, translator("ms"))]).toStrictEqual([
      "The scanned QR label is not this job's unit — scan the label on this AC, or check in manually with a reason",
      "Label QR yang diimbas bukan unit kerja ini — imbas label pada AC ini, atau daftar masuk secara manual dengan sebab",
    ]);
  });
  it("words the line in the display language; field names and domain keys stay humanized English (IR269)", () => {
    const t = translator("ms");
    expect([
      actionMessage(failure({ fieldErrors: { reason: ERROR_LENGTH } }), t), actionMessage(failure({ fieldErrors: { durationMinutes: ERROR_RANGE } }), t),
      actionMessage(failure({ fieldErrors: { unit: "errors.unit_mismatch" } }), t), actionMessage(failure({ code: "CONFLICT", messageKey: "error.versionConflict" }), t),
      actionMessage(failure({ code: "NOT_FOUND", messageKey: "error.notFound" }), t), actionMessage(failure({ code: "UNAUTHENTICATED", messageKey: "error.unauthenticated" }), t),
      actionMessage(failure({ code: "CONFLICT", messageKey: "errors.operation_not_running" }), t), actionMessage(failure({ code: RATE_LIMITED_2, messageKey: ERROR_RATELIMITED }), t),
    ]).toStrictEqual([
      "Reason terlalu pendek atau terlalu panjang", "Duration minutes di luar julat", "Unit — unit mismatch", "Seseorang telah mengubahnya sebentar tadi — skrin kini menunjukkan versi terkini",
      "Item ini tidak lagi wujud atau di luar skop anda", "Sesi anda telah tamat — log masuk semula", "Operation not running", "Terlalu banyak perubahan dalam masa yang singkat — cuba lagi sebentar lagi",
    ]);
  });
});

const r = (messageKey: string) => ({ code: "CONFLICT", fieldErrors: {}, messageKey });

describe("actionMessage for the user management refusals (IR416–IR421)", () => {
  it("says what to do, in the display language", () => {
    expect(actionMessage(r("errors.technician_booked"))).toBe("This technician still has an assigned job — reassign it first");
    expect(actionMessage(r("errors.own_account"))).toBe("You can’t change or remove your own account");
    expect(actionMessage(r("errors.last_owner"), translator("ms"))).toBe("Pemilik aktif terakhir tidak boleh diturunkan, dilumpuhkan atau dialih keluar");
    expect(actionMessage(r("errors.invoice_has_payment"), translator("ms"))).toBe("Invois ini sudah mempunyai pembayaran yang sedang diproses atau telah direkodkan — selesaikan atau gagalkan pembayaran itu dahulu");
    expect(actionMessage(r("errors.last_identity_admin"))).toBe("At least one active membership must keep identity.write — give it to another HQ membership first, then change this one");
    expect(actionMessage(r("errors.some_other_rule"))).toBe("Some other rule"); // other keys are still humanized
  });
});

// IR620: the Users dialogs put a field refusal under its field as well as in the toast
describe("fieldErrorTexts", () => {
  it("words each known field as the toast does, under the form's own names, and leaves the rest out", () => {
    const f = failure({ fieldErrors: { email: "errors.email_taken", accessUntil: ERROR_RANGE, tenantId: "error.invalid" } });
    expect(fieldErrorTexts(f, { email: "email", accessUntil: "until" })).toStrictEqual({
      email: "Email belongs to someone who still has access — use another address or ask HQ",
      until: "Access until is out of range",
    });
    expect(fieldErrorTexts(failure({ code: "CONFLICT", messageKey: "errors.own_account" }), { email: "email" })).toStrictEqual({});
  });
  it("words them in the display language", () => {
    const ms = fieldErrorTexts(failure({ fieldErrors: { reason: ERROR_LENGTH } }), { reason: "reason" }, translator("ms"));
    expect(ms.reason).not.toBe("Reason is too short or too long");
  });
});
