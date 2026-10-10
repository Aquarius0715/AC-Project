import { describe, expect, it } from "vitest";
import { clientUserRows, inviteError, type ApiClientUser } from "@ac/web/lib/assets";
import type { ApiInvoice } from "@ac/web/lib/billing";
import {
  clientInvoiceRows, contractCards, inquiryLines, noticeTimeline, noticeUnits, paymentLines, previewText, restrictionNotice, type ClientContract, type ClientPayment,
} from "@ac/web/lib/clientBilling";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import { policyText, type ApiCommand, type ApiRestriction } from "@ac/web/lib/restrictions";

const MS_TOKYO = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-14T01:00:00Z"); // Mon 09:00 KL
const unit = () => ({ name: "Bedroom AC", place: "Home A › 1F" });
const contract: ClientContract = {
  id: "k1aaaaaaaa", version: 1, customerId: "c", customerOrgId: "o", unitIds: ["u1"], planType: "rto", startAt: "2026-08-31T16:00:00Z", endAt: "2027-08-31T16:00:00Z", priceMinor: 1, currency: "MYR", restrictionEligible: true,
};
const invoice: ApiInvoice = {
  id: "i1", version: 1, number: "INV-2026-09", amountMinor: 12000, currency: "MYR", contractId: "k1", contractVersion: 1, period: { from: "2026-08-31T16:00:00Z", to: "2026-09-30T16:00:00Z" },
  dueAt: "2026-09-10T15:59:59Z", status: "unpaid", paymentMethod: null, paymentStatus: null, paidAt: null, // due 23:59:59 KL on 10 Sept — already 11 Sept in Tokyo
};
const restriction = {
  id: "r1", version: 1, createdAt: "", unitIds: ["u1", "u2"], rulesVersion: "1", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 27 }, state: "release_requested", recoveryCases: [],
  perUnit: [
    { unitId: "u1", applyState: "applied", releaseState: "requested", applyCommandIds: ["c1"], releaseCommandIds: ["c2"], observedRestriction: null, observedAt: null, pendingReason: null },
    { unitId: "u2", applyState: "sent_unknown", releaseState: "failed", applyCommandIds: ["c3"], releaseCommandIds: [], observedRestriction: null, observedAt: null, pendingReason: "offline" },
  ],
  noticeAt: "2026-09-10T01:00:00Z", executeAfter: "2026-09-11T01:00:00Z", releaseIntent: { source: "payment", at: "2026-09-13T02:05:00Z" },
  events: [
    { action: "restrictions.schedule", occurredAt: "2026-09-10T01:00:00Z", actorId: "", actorRoleAtTime: "", reason: null, result: "" },
    { action: "restriction.applied", occurredAt: "2026-09-11T01:00:00Z", actorId: "", actorRoleAtTime: "", reason: null, result: "" },
    { action: "payments.release", occurredAt: "2026-09-13T02:05:00Z", actorId: "", actorRoleAtTime: "", reason: null, result: "" },
  ],
} as ApiRestriction;
const commands = new Map<string, ApiCommand>([["c2", { id: "c2", status: "sent", delivery: "", requestedAt: "", failureCode: null }], ["c3", { id: "c3", status: "expired", delivery: "", requestedAt: "", failureCode: null }]]);

describe("customer contracts & payments in the display language (FR-C10–C12, IR267)", () => {
  it("names contracts and keeps their periods and the due dates on Kuala Lumpur business days", () => {
    expect(contractCards([contract], unit, NOW).map((c) => [c.name, c.sub, c.period, c.plan, c.status])).toEqual([["RTO Plan", "k1aaaaaa · 1 Sept → 1 Sept 2027", "1 Sept 2026 → 1 Sept 2027", "Rent-to-own (RTO)", "Active"]]);
    const old = { ...contract, id: "k2bbbbbbbb", planType: "general", startAt: "2025-08-31T16:00:00Z", endAt: "2026-08-31T16:00:00Z" };
    expect(contractCards([old, contract], unit, NOW, MS_TOKYO).map((c) => [c.name, c.period, c.status])).toEqual([["Pelan RTO", "1 Sep 2026 → 1 Sep 2027", "Active"], ["Penyelenggaraan am", "1 Sep 2025 → 1 Sep 2026", "Expired"]]);
    expect(clientInvoiceRows([invoice], NOW).map((r) => [r.due, r.status, r.payable])).toEqual([["Due 10 Sept 2026", "Overdue", true]]);
    const paid = { ...invoice, id: "i2", number: "INV-2026-08", dueAt: "2026-08-10T15:59:59Z", status: "paid" as const, paidAt: "2026-08-09T23:30:00Z" }; // paid 08:30 on 10 Aug in Tokyo
    expect(clientInvoiceRows([invoice, paid], NOW, MS_TOKYO).map((r) => r.due)).toEqual(["Tarikh akhir 10 Sep 2026", "Tarikh akhir 10 Ogo 2026 · dibayar 10 Ogo 2026"]);
  });

  it("times payments, restriction events and inquiries in the display time zone", () => {
    const p: ClientPayment = {
      id: "p1aaaaaaaa", version: 2, amountMinor: 12000, currency: "MYR", invoiceId: "i1", method: "demo_credit_card", status: "confirmed", paymentReference: "DEMO-PAY-1",
      confirmedAt: "2026-09-13T02:05:00Z", confirmationReason: null, createdAt: "2026-09-13T02:00:00Z", updatedAt: "2026-09-13T02:05:00Z",
    };
    expect(paymentLines([p]).map((l) => [l.label, l.at])).toEqual([["p1aaaaaa · Credit card (demo)", "13 Sept 2026, 10:05 am MYT"]]);
    expect(paymentLines([{ ...p, method: null }], MS_TOKYO).map((l) => [l.label, l.at])).toEqual([["p1aaaaaa · Pindahan bank (direkod oleh HQ)", "13 Sep 2026, 11:05 PG GMT+9"]]);
    expect(noticeTimeline(restriction, MS_TOKYO)).toEqual([
      { time: "10 Sep 2026, 10:00 PG GMT+9", title: "Dijadualkan" }, { time: "11 Sep 2026, 10:00 PG GMT+9", title: "Dikenakan", tone: "warn" },
      { time: "13 Sep 2026, 11:05 PG GMT+9", title: "Pelepasan diminta selepas pembayaran" }, { time: "—", title: "Dilepaskan (hanya apabila setiap unit mengesahkan)", tone: "ok" },
    ]);
    expect(noticeTimeline(restriction)[2]).toEqual({ time: "13 Sept 2026, 10:05 am MYT", title: "Release requested after payment" });
    expect(inquiryLines([{ id: "q1", version: 1, customerId: "c", invoiceId: "i1", restrictionId: null, subjectType: "payment", message: "Hi", state: "received", reply: null, createdAt: "2026-09-13T03:00:00Z" }], MS_TOKYO)
      .map((q) => [q.subject, q.at])).toEqual([["Pembayaran", "13 Sep 2026, 12:00 PTG GMT+9"]]);
  });

  it("words the restriction notice per unit in Malay, never released before the unit confirms", () => {
    expect(restrictionNotice(restriction, unit)).toMatchObject({ label: "Release requested", policy: "Temperature limit — cooling setpoint ≥ 27 °C", units: "2 units in scope · Bedroom AC (Home A › 1F), Bedroom AC (Home A › 1F)" });
    expect(restrictionNotice(restriction, unit, MS_TOKYO.t)).toMatchObject({ label: "Pelepasan diminta", policy: "Had suhu — suhu tetapan penyejukan ≥ 27 °C", units: "2 unit dalam skop · Bedroom AC (Home A › 1F), Bedroom AC (Home A › 1F)" });
    expect(noticeUnits(restriction, unit, commands, MS_TOKYO.t).map((u) => [u.apply, u.release, u.releaseTone, u.note, u.tone])).toEqual([
      ["Dikenakan", "Pelepasan dihantar", "primary", "menunggu unit mengesahkan", "warn"], ["Keputusan tidak diketahui", "Pelepasan gagal", "crit", "unit di luar talian · arahan terakhir tamat tempoh", "unknown"],
    ]);
    expect([policyText({ kind: "power_off" }), policyText({ kind: "power_off" }, MS_TOKYO.t)]).toEqual(["Power off", "Matikan kuasa"]);
  });

  it("writes the payment message preview in the recipient's language", () => {
    const inv = { number: "INV-2026-09", amount: "120.00 MYR", dueDate: "10 Sep 2026", status: "Overdue" as const };
    expect(previewText("email", inv)).toEqual({ subject: "Payment reminder — INV-2026-09", body: "Hello, INV-2026-09 (120.00 MYR) was due on 10 Sep 2026. Pay with a demo card or view payment instructions in the app." });
    expect(previewText("whatsapp", { ...inv, status: "Unpaid" }, MS_TOKYO.t)).toEqual({ subject: null, body: "Helo, INV-2026-09 (120.00 MYR) perlu dibayar pada 10 Sep 2026. Bayar dengan kad demo atau lihat arahan pembayaran dalam aplikasi." });
    expect(previewText("email", { ...inv, status: "Paid" }, MS_TOKYO.t)).toEqual({ subject: "Pembayaran diterima — INV-2026-09", body: "Helo, INV-2026-09 (120.00 MYR) telah dibayar — terima kasih." });
  });
});

describe("the customer's users in the display language (FR-C19, IR267)", () => {
  const users: ApiClientUser[] = [
    { id: "a", version: 1, customerId: "c", membershipId: "m1", email: "owner@x.com", displayName: "Aisyah", clientRole: "owner", status: "active", lastSignInAt: "2026-09-13T23:30:00Z", allowedChannels: ["inApp", "email"], invitedAt: "2026-09-01T00:00:00Z", invitedByMembershipId: "hq" },
    { id: "b", version: 1, customerId: "c", membershipId: null, email: "lim@x.com", displayName: null, clientRole: "member", status: "invited", lastSignInAt: null, allowedChannels: [], invitedAt: "2026-09-13T16:30:00Z", invitedByMembershipId: "m1" },
  ];

  it("keeps HQ's register in English and Kuala Lumpur and gives the owner their language and time zone", () => {
    expect(clientUserRows(users, (m) => (m === "m1" ? "you" : "HQ"), "m1").map((u) => [u.sub, u.lastSignIn, u.channels])).toEqual([["owner@x.com", "2026-09-14 07:30", "In-app · Email"], ["invited 2026-09-14 by you", "—", "—"]]);
    expect(clientUserRows(users, (m) => (m === "m1" ? MS_TOKYO.t("you") : "HQ"), "m1", MS_TOKYO).map((u) => [u.sub, u.lastSignIn, u.channels, u.lastOwner, u.you])).toEqual([
      ["owner@x.com", "14 Sep 2026, 8:30 PG GMT+9", "Dalam aplikasi · E-mel", true, true], ["dijemput 14 Sep 2026 oleh anda", "—", "—", false, false],
    ]);
  });

  it("checks an invite address in either language", () => {
    expect([inviteError("bad", []), inviteError("lim@x.com", [{ email: "LIM@x.com" }], translator("ms")), inviteError("new@x.com", [])]).toEqual(["A valid email address (up to 254 characters)", "Sudah menjadi pengguna pelanggan ini", undefined]);
  });
});
