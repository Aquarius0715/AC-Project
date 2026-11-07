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
    expect([actionMessage(failure({ code: "CONFLICT", messageKey: "errors.revoked_by_hq" })), actionMessage(failure({ fieldErrors: { email: "errors.revoked_by_hq" } }), translator("ms"))]).toEqual([
      "HQ ended this account's access — only HQ can give it back", "HQ telah menamatkan akses orang ini — hanya HQ boleh memulihkannya"]);
    // IR510: an HQ membership on another app's token
    expect(actionMessage(failure({ code: "FORBIDDEN", messageKey: "errors.hq_app_required" }))).toBe("An HQ account works in the HQ app — sign in there");
    // IR486: a QR check-in with another unit's label says what to do, in the display language
    const qr = failure({ fieldErrors: { qrUnitId: "errors.qr_unit_mismatch" } });
    expect([actionMessage(qr), actionMessage(qr, translator("ms"))]).toEqual([
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
    ]).toEqual([
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
    expect(fieldErrorTexts(f, { email: "email", accessUntil: "until" })).toEqual({
      email: "Email belongs to someone who still has access — use another address or ask HQ",
      until: "Access until is out of range",
    });
    expect(fieldErrorTexts(failure({ code: "CONFLICT", messageKey: "errors.own_account" }), { email: "email" })).toEqual({});
  });
  it("words them in the display language", () => {
    const ms = fieldErrorTexts(failure({ fieldErrors: { reason: ERROR_LENGTH } }), { reason: "reason" }, translator("ms"));
    expect(ms.reason).not.toBe("Reason is too short or too long");
  });
});
