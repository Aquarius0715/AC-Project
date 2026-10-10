import { describe, expect, it } from "vitest";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import { attemptText, conceptText, eventItem, kg3, quoteErrors, quoteInput, recordRows, stateWord, type ApiOffsetRecord, type QuoteDraft } from "@ac/web/lib/offsets";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const attempt = { id: "a1", stage: "purchase" as const, status: "pending" as const, startedAt: "2026-09-14T01:30:00Z", completedAt: null };
const record = (over: Partial<ApiOffsetRecord>): ApiOffsetRecord => ({
  id: "rec-0001-aaaa", version: 2, createdAt: "2026-09-14T01:00:00Z", quoteId: "quo-0001-bbbb", customerId: "c1", amountKg: 1.5, attempts: [attempt], currentAttemptId: "a1",
  state: "demo_requested", previousState: null, purchaseRef: null, retirementRef: null, demoCertificateRef: null,
  eventHistory: [{ action: "offsets.request", occurredAt: "2026-09-14T01:00:00Z", actorId: "user-hq-0001", result: "success", reason: null }], ...over,
});
const draft = (over: Partial<QuoteDraft>): QuoteDraft => ({ customerId: "c1", unitIds: ["u1"], from: "2026-08-01T00:00", to: "2026-09-01T00:00", purpose: " Offset August ", amountKg: "1.250", ...over });

describe("HQ offset demo (FR-A15, DD-A15)", () => {
  it("words each record's state, next step and current attempt", () => {
    const rows = recordRows([
      record({}), record({ id: "r2", state: "demo_purchased", customerId: "cx" }), record({ id: "r3", state: "demo_retired", demoCertificateRef: "DEMO-0007", currentAttemptId: null }),
      record({ id: "r4", state: "failed", previousState: "demo_purchased" }), record({ id: "r5", state: "failed", previousState: "demo_requested" }),
    ], [{ id: "c1", name: "Demo Customer A" }]);
    expect(rows.map((r) => [r.customer, r.amount, r.stateText, r.stage, r.next])).toEqual([
      ["Demo Customer A", "1.500 kgCO₂e", "Demo requested", 1, "Next: confirm the demo purchase"], ["customer", "1.500 kgCO₂e", "Demo purchased", 2, "Next: demo retirement"],
      ["Demo Customer A", "1.500 kgCO₂e", "Demo retired", 3, "DEMO-0007 · retired"], ["Demo Customer A", "1.500 kgCO₂e", "Failed", -1, "Retirement failed · previous: Demo purchased"],
      ["Demo Customer A", "1.500 kgCO₂e", "Failed", -1, "Purchase failed · previous: Demo requested"],
    ]);
    expect([rows[0].currentText, rows[2].currentText]).toEqual(["purchase · pending · started 14 Sept 2026, 9:30 am MYT", null]);
    expect(rows[0].events).toEqual([{ time: "14 Sept 2026, 9:00 am MYT", title: "Demo request · done", detail: "user-hq-" }]);
    expect(eventItem({ action: "offsets.retire", occurredAt: "2026-09-14T01:00:00Z", actorId: "user-hq-0001", actorName: "hq-operator", result: "success", reason: null }).detail).toBe("hq-operator"); // who acted (IR305)
    expect(eventItem({ action: "offsets.something_new", occurredAt: "2026-09-14T01:00:00Z", actorId: "system", result: "denied", reason: "not allowed" }).title).toBe("offsets.something_new · denied");
    expect([kg3(0.1), kg3(1234.5)]).toEqual(["0.100", "1,234.500"]);
  });

  it("checks a demo quote and sends its Kuala Lumpur period as instants", () => {
    expect(quoteErrors(draft({}))).toEqual({});
    expect(quoteErrors(draft({ customerId: "", unitIds: [], to: "2026-08-01T00:00", purpose: " ", amountKg: "1.2345" }))).toEqual({
      customerId: "Choose a customer", unitIds: "Choose 1–100 units", period: "The end must be after the start", purpose: "1–1000 characters", amountKg: "Above 0, at most 100000, up to 3 decimals",
    });
    expect(quoteInput(draft({}))).toEqual({ customerId: "c1", purpose: "Offset August", amountKg: 1.25, period: { from: "2026-07-31T16:00:00.000Z", to: "2026-08-31T16:00:00.000Z" }, unitIds: ["u1"] });
    expect(conceptText({ stage: "future_concept", providerLabel: "unselected", verificationStatus: "unverified", ledgerStatus: "not_connected" })).toBe("future concept · verification unverified · ledger not connected");
  });
});

describe("HQ offset demo in Malay with the display time zone (IR298)", () => {
  it("words the records, attempts, events, market concept and checks", () => {
    const t = translator("ms");
    const [row] = recordRows([record({ state: "failed", previousState: "demo_purchased", attempts: [{ ...attempt, stage: "retirement", status: "failed" }] })], [], MS);
    expect([row.customer, row.stateText, row.next, row.currentText]).toEqual(["pelanggan", "Gagal", "Pembatalan gagal · sebelumnya: Demo dibeli", "pembatalan · gagal · dimulakan 14 Sep 2026, 10:30 PG GMT+9"]);
    expect(row.events).toEqual([{ time: "14 Sep 2026, 10:00 PG GMT+9", title: "Permintaan demo · selesai", detail: "user-hq-" }]);
    expect([stateWord("demo_retired", t), stateWord(null, t), attemptText({ ...attempt, status: "succeeded" }, MS)]).toEqual(["Demo dibatalkan", "—", "pembelian · berjaya · dimulakan 14 Sep 2026, 10:30 PG GMT+9"]);
    expect(conceptText({ stage: "future_concept", providerLabel: "unselected", verificationStatus: "unverified", ledgerStatus: "not_connected" }, t)).toBe("konsep masa depan · pengesahan belum disahkan · lejar tidak disambungkan");
    expect(quoteErrors(draft({ customerId: "", amountKg: "0" }), t)).toEqual({ customerId: "Pilih pelanggan", amountKg: "Melebihi 0, paling banyak 100000, sehingga 3 perpuluhan" });
  });
});
