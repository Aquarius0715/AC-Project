import { describe, expect, it } from "vitest";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { translator } from "@ac/web/lib/i18n";

const failure = (over: Partial<Parameters<typeof actionMessage>[0]>) => ({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: {}, ...over });

describe("actionMessage (one toast line per failed Server Action)", () => {
  it("reads the first field error with a known or humanized key", () => {
    expect(actionMessage(failure({ fieldErrors: { reason: "error.length" } }))).toBe("Reason is too short or too long");
    expect(actionMessage(failure({ fieldErrors: { "query.filters.to": "error.range" } }))).toBe("To is out of range");
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
    expect(actionMessage(failure({ code: "RATE_LIMITED", messageKey: "error.rateLimited", retryAfterSeconds: 40 }))).toBe("Too many changes in a short time — try again in 40 s");
    expect(actionMessage(failure({ code: "RATE_LIMITED", messageKey: "error.rateLimited" }))).toBe("Too many changes in a short time — try again shortly");
    expect(actionMessage(failure({ code: "RATE_LIMITED", messageKey: "errors.invite_recently_sent", retryAfterSeconds: 241 }))).toBe("The set-up e-mail was sent a moment ago — send it again in 5 min");
    // IR469: an invoice period overlapping an invoiced one says what to do
    expect(actionMessage(failure({ code: "CONFLICT", messageKey: "errors.invoice_period_exists" }))).toBe("This contract already has an invoice for part of this period — choose a period after it");
    // IR464: the accounts added per day say when more can be added (rounded up to hours)
    expect(actionMessage(failure({ code: "RATE_LIMITED", messageKey: "errors.too_many_accounts", retryAfterSeconds: 81060 }))).toBe("Many users were added in the last 24 hours — you can add more in 23 h");
    // IR462: an address someone with access holds says what to do; a nested field is named by its last part
    expect(actionMessage(failure({ fieldErrors: { "newUser.email": "errors.email_taken" } }))).toBe("Email belongs to someone who still has access — use another address or ask HQ");
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
      actionMessage(failure({ fieldErrors: { reason: "error.length" } }), t), actionMessage(failure({ fieldErrors: { durationMinutes: "error.range" } }), t),
      actionMessage(failure({ fieldErrors: { unit: "errors.unit_mismatch" } }), t), actionMessage(failure({ code: "CONFLICT", messageKey: "error.versionConflict" }), t),
      actionMessage(failure({ code: "NOT_FOUND", messageKey: "error.notFound" }), t), actionMessage(failure({ code: "UNAUTHENTICATED", messageKey: "error.unauthenticated" }), t),
      actionMessage(failure({ code: "CONFLICT", messageKey: "errors.operation_not_running" }), t), actionMessage(failure({ code: "RATE_LIMITED", messageKey: "error.rateLimited" }), t),
    ]).toEqual([
      "Reason terlalu pendek atau terlalu panjang", "Duration minutes di luar julat", "Unit — unit mismatch", "Seseorang telah mengubahnya sebentar tadi — skrin kini menunjukkan versi terkini",
      "Item ini tidak lagi wujud atau di luar skop anda", "Sesi anda telah tamat — log masuk semula", "Operation not running", "Terlalu banyak perubahan dalam masa yang singkat — cuba lagi sebentar lagi",
    ]);
  });
});

describe("actionMessage for the user management refusals (IR416–IR421)", () => {
  it("says what to do, in the display language", () => {
    const r = (messageKey: string) => ({ code: "CONFLICT", messageKey, fieldErrors: {} });
    expect(actionMessage(r("errors.technician_booked"))).toBe("This technician still has an assigned job — reassign it first");
    expect(actionMessage(r("errors.own_account"))).toBe("You can’t change or remove your own account");
    expect(actionMessage(r("errors.last_owner"), translator("ms"))).toBe("Pemilik aktif terakhir tidak boleh diturunkan, dilumpuhkan atau dialih keluar");
    expect(actionMessage(r("errors.invoice_has_payment"), translator("ms"))).toBe("Invois ini sudah mempunyai pembayaran yang sedang diproses atau telah direkodkan — selesaikan atau gagalkan pembayaran itu dahulu");
    expect(actionMessage(r("errors.last_identity_admin"))).toBe("At least one active membership must keep identity.write — give it to another HQ membership first, then change this one");
    expect(actionMessage(r("errors.some_other_rule"))).toBe("Some other rule"); // other keys are still humanized
  });
});
