import { describe, expect, it } from "vitest";
import {
  billingMonth, billingMonths, businessDay, contractOptions, lastClosedMonth, monthsText, inquiryRows, invoiceDetail, invoiceRows, invoiceStatus, paymentWord, statementDetail, statementRows, statementWord, totals,
  type ApiContract, type ApiCustomer, type ApiInquiry, type ApiInvoice, type ApiStatement,
} from "@ac/web/lib/billing";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-15T01:00:00Z");
const customers: ApiCustomer[] = [{ id: "c1", name: "Demo Customer A", organizationId: "org-a", status: "active" }];
const contracts: ApiContract[] = [{ id: "k1", version: 1, customerId: "c1", customerOrgId: "org-a", unitIds: ["u1"], planType: "rto", startAt: "2026-01-01T00:00:00+08:00", endAt: "2027-01-01T00:00:00+08:00", priceMinor: 12000, currency: "MYR", restrictionEligible: true }];
const invoice = (over: Partial<ApiInvoice>): ApiInvoice => ({
  id: "i1", version: 1, number: "INV-202608-0001", amountMinor: 12000, currency: "MYR", contractId: "k1", contractVersion: 1, period: { from: "2026-07-31T16:00:00Z", to: "2026-08-31T16:00:00Z" },
  dueAt: "2026-09-09T16:00:00Z", status: "unpaid", paymentMethod: null, paymentStatus: null, paidAt: null, ...over,
});
const statement: ApiStatement = {
  id: "s1", version: 2, contractorOrgId: "org-p", period: "2026-08", status: "approved", currency: "MYR", grossMinor: 30000, deductionsMinor: 5000, netMinor: 25000, payDate: "2026-09-14T16:00:00Z",
  lines: [{ id: "l1", jobId: "job-1", workType: "repair_base", acceptedAt: null, amountMinor: 30000, kind: "charge", note: null }, { id: "l2", jobId: "job-2", workType: "rework_deduction", acceptedAt: null, amountMinor: 5000, kind: "deduction", note: "rework" }],
  queries: [{ id: "q1", lineId: "l2", topic: "deduction", message: "Why?", state: "open", reply: null, adjustmentMinor: null }, { id: "q2", lineId: "l1", topic: "amount", message: "Short", state: "adjusted", reply: "Fixed", adjustmentMinor: -500 }],
  approvedByMembershipId: "m-hq", paidAt: null,
};

describe("HQ billing (FR-A08, FR-A23, DD-A08)", () => {
  it("rows invoices with Kuala Lumpur business days and the payment method as recorded", () => {
    expect([invoiceStatus(invoice({}), NOW), invoiceStatus(invoice({ dueAt: "2026-09-20T16:00:00Z" }), NOW), invoiceStatus(invoice({ status: "processing" }), NOW), invoiceStatus(invoice({ status: "paid" }), NOW)])
      .toEqual(["Overdue", "Unpaid", "Processing", "Paid"]);
    const [a, b] = invoiceRows([invoice({}), invoice({ id: "i2", contractId: "kx", paymentMethod: "demo_credit_card", paymentStatus: "processing", status: "processing" })], contracts, customers, NOW);
    expect(a).toMatchObject({ amt: "120.00 MYR", meta: "Demo Customer A · rto contract v1 · August 2026", method: "Method not selected", methodName: "Method not selected", paymentState: null, st: "Overdue", period: "1 Aug 2026 → 1 Sept 2026", due: "10 Sept 2026" });
    expect(b).toMatchObject({ meta: "customer · kx · August 2026", method: "Credit card · processing", methodName: "Credit card", paymentState: "processing", customerId: "" });
    expect([totals([a, b], ["Overdue", "Processing"]), totals([a], ["Paid"])]).toEqual(["240.00 MYR", "0.00"]);
  });

  it("words the payments of an invoice, inquiries, contracts and payout statements", () => {
    const d = invoiceDetail({ ...invoice({}), restrictionIds: ["r1"], paymentRefs: [
      { id: "p1", version: 1, amountMinor: 12000, currency: "MYR", invoiceId: "i1", method: null, status: "confirmed", paymentReference: "BANK-1", confirmationReason: "Bank transfer", confirmedAt: "2026-09-12T02:00:00Z" },
      { id: "p2", version: 3, amountMinor: 12000, currency: "MYR", invoiceId: "i1", method: "demo_debit_card", status: "processing", paymentReference: null, confirmationReason: null, confirmedAt: null },
    ] });
    expect(d).toEqual({ id: "i1", version: 1, processingPaymentId: "p2", processingPaymentVersion: 3, restrictionIds: ["r1"], payments: ["No method (recorded by HQ) · confirmed · ref BANK-1 · reason “Bank transfer”", "Debit card · processing"] });
    const iq: ApiInquiry = { id: "q1", version: 1, customerId: "c1", invoiceId: "i1", restrictionId: null, subjectType: "payment", message: "Paid already", state: "received", reply: null, createdAt: "2026-09-14T02:00:00Z" };
    expect(inquiryRows([{ ...iq, id: "q0", state: "answered", createdAt: "2026-09-15T00:00:00Z", subjectType: "restriction" }, iq], customers).map((r) => [r.id, r.kind, r.at, r.text])).toEqual([
      ["q1", "Payment", "14 Sept 2026, 10:00 am MYT", "“Paid already”"], ["q0", "Cooling restriction", "15 Sept 2026, 8:00 am MYT", "“Paid already”"],
    ]); // received first
    expect(contractOptions(contracts, [])[0].label).toBe("customer · rto · 120.00 MYR");
    expect(statementRows([statement], [{ id: "org-p", name: "CoolFix", kind: "contractor" }])).toEqual([{
      id: "s1", version: 2, contractor: "CoolFix", period: "Aug 2026", status: "Approved", jobs: 2, net: "250.00 MYR", statusText: "Approved · pays 15 Sept 2026", queryCount: 2, openQueries: 1,
    }]);
    // the status with its date (IR322): paid on its paid day, a draft as it is
    expect(statementRows([{ ...statement, status: "paid", paidAt: "2026-09-15T04:00:00Z" }, { ...statement, id: "s2", status: "draft" }], []).map((r) => r.statusText)).toEqual(["Paid 15 Sept 2026", "Draft"]);
    expect([statementDetail(statement, [], NOW, undefined, new Map([["m-hq", "hq-operator"]])).approvedBy, statementDetail({ ...statement, approvedByMembershipId: null }, [], NOW).approvedBy]).toEqual(["hq-operator", null]);
    expect([lastClosedMonth(NOW), lastClosedMonth(new Date("2026-01-01T00:00:00+08:00")), lastClosedMonth(new Date("2026-09-30T16:30:00Z"))]).toEqual(["2026-08", "2025-12", "2026-09"]); // 1 Oct in Kuala Lumpur
    const s = statementDetail(statement, [], NOW);
    expect([s.contractor, s.payDate, s.payable, s.lines.map((l) => [l.work, l.kind, l.amount])]).toEqual(["contractor", "15 Sept 2026", true, [["Repair", "charge", "300.00 MYR"], ["Rework deduction", "deduction", "−50.00 MYR"]]]);
    expect(s.queries.map((q) => [q.topic, q.line, q.state, q.open, q.adjustment])).toEqual([["Deduction", "job-2", "open", true, null], ["Amount", "job-1", "adjusted", false, "−5.00 MYR"]]);
    expect(statementDetail({ ...statement, payDate: "2026-09-20T16:00:00Z" }, [], NOW).payable).toBe(false);
  });
});

describe("HQ billing months (DD-A08 step 5, IR321)", () => {
  it("turns the URL months into invoices.list filters on the period start, Kuala Lumpur months", () => {
    expect(billingMonths("2026-08", "2026-09")).toEqual({ from: "2026-08", to: "2026-09", filters: { from: "2026-07-31T16:00:00.000Z", to: "2026-09-30T16:00:00.000Z" } });
    expect(billingMonths("2026-12", "2026-12").filters.to).toBe("2026-12-31T16:00:00.000Z"); // into the next year
    expect(billingMonths("2026-08")).toEqual({ from: "2026-08", filters: { from: "2026-07-31T16:00:00.000Z" } });
    expect([billingMonths("2026-09", "2026-08"), billingMonths("2026-13", "x"), billingMonths()]).toEqual([{ filters: {} }, { filters: {} }, { filters: {} }]); // reversed or malformed: no period
  });

  it("names the months in the user's language", () => {
    expect([monthsText("2026-08", "2026-09"), monthsText("2025-12", "2026-01"), monthsText("2026-08", "2026-08"), monthsText("2026-08", undefined), monthsText(undefined, "2026-09"), monthsText(undefined, undefined)])
      .toEqual(["Aug – Sept 2026", "Dec 2025 – Jan 2026", "Aug 2026", "from Aug 2026", "until Sept 2026", null]);
    expect([monthsText("2026-08", "2026-09", MS), monthsText("2026-08", undefined, MS)]).toEqual(["Ogo – Sep 2026", "dari Ogo 2026"]);
  });
});

describe("HQ billing in Malay with the display time zone (IR299)", () => {
  it("keeps business days in Kuala Lumpur, words everything else and shows inquiry times in the display zone", () => {
    const t = translator("ms");
    expect([businessDay("2026-09-09T16:00:00Z", "ms"), billingMonth("2026-07-31T16:00:00Z", "ms")]).toEqual(["10 Sep 2026", "Ogos 2026"]); // Kuala Lumpur days, not Tokyo's
    const [r] = invoiceRows([invoice({ paymentMethod: "demo_debit_card", paymentStatus: "failed" })], contracts, [], NOW, MS);
    expect(r).toMatchObject({ meta: "pelanggan · kontrak rto v1 · Ogos 2026", method: "Kad debit · gagal", period: "1 Ogo 2026 → 1 Sep 2026", due: "10 Sep 2026" });
    expect([paymentWord("initiated", t), paymentWord("refunded", t), statementWord("Draft", t), statementWord("Paid", t)]).toEqual(["dimulakan", "refunded", "Draf", "Dibayar"]);
    const iq: ApiInquiry = { id: "q1", version: 1, customerId: "cx", invoiceId: null, restrictionId: "r1", subjectType: "restriction", message: "Why?", state: "received", reply: null, createdAt: "2026-09-14T02:00:00Z" };
    expect(inquiryRows([iq], customers, MS)[0]).toMatchObject({ cust: "pelanggan", kind: "Sekatan penyejukan", at: "14 Sep 2026, 11:00 PG GMT+9" });
    const s = statementDetail(statement, [], NOW, MS);
    expect([s.period, s.payDate, s.lines[0].work, s.lines[1].kind, s.queries[0].state, s.queries[1].state]).toEqual(["Ogo 2026", "15 Sep 2026", "Pembaikan", "potongan", "terbuka", "dilaraskan"]);
  });
});
