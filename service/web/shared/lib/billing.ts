// HQ billing (FR-A08, FR-A23, DATA_SOURCE=api): invoices, contracts, customers, inquiries and reminder recipients
// projected for the admin billing screen. Pure code shared by the Server Component and the client view. Texts in the
// display language (`t` / `i`, IR299): billing periods, due dates, billing months and pay dates are Kuala Lumpur business
// days written in the user's language; inquiries are instants in the display time zone.
import type { Currency } from "@ac/web/lib/contracts.gen";
import { EN, intlTag, showDate, showTime, translator, type I18n, type Locale, type T } from "@ac/web/lib/i18n";

const en = translator("en");
export type Range = { from: string; to: string };
export type ApiInvoice = { id: string; version: number; number: string; amountMinor: number; currency: Currency; contractId: string; contractVersion: number; period: Range; dueAt: string; status: "unpaid" | "processing" | "paid"; paymentMethod: string | null; paymentStatus: string | null; paidAt: string | null };
export type ApiPayment = { id: string; version: number; amountMinor: number; currency: Currency; invoiceId: string; method: string | null; status: "initiated" | "processing" | "confirmed" | "failed"; paymentReference: string | null; confirmedAt: string | null; confirmationReason: string | null };
export type ApiInvoiceDetail = ApiInvoice & { paymentRefs: ApiPayment[]; restrictionIds: string[] };
export type ApiContract = { id: string; version: number; customerId: string; customerOrgId: string; unitIds: string[]; planType: string; startAt: string; endAt: string; priceMinor: number; currency: Currency; restrictionEligible: boolean };
export type ApiCustomer = { id: string; name: string; organizationId: string; status: string };
export type ApiProperty = { id: string; customerOrgId: string; name: string; archived: boolean };
export type ApiInquiry = { id: string; version: number; customerId: string; invoiceId: string | null; restrictionId: string | null; subjectType: "payment" | "restriction"; message: string; state: "received" | "answered"; reply: string | null; createdAt: string };
export type ApiRecipient = { id: string; role: string; displayLabel: string; allowedChannels: ("inApp" | "email" | "whatsapp")[] };

export type InvoiceStatus = "Unpaid" | "Overdue" | "Processing" | "Paid";
export type InvoiceRow = {
  id: string; version: number; number: string; amt: string; amountMinor: number; currency: Currency; meta: string; method: string; methodName: string; paymentState: string | null;
  st: InvoiceStatus; customerId: string; contractId: string; period: string; due: string;
};
export type InvoiceDetail = { id: string; version: number; processingPaymentId: string | null; processingPaymentVersion: number | null; payments: string[]; restrictionIds: string[] };
export type InquiryRow = { id: string; version: number; cust: string; kind: string; at: string; text: string; state: "received" | "answered"; reply: string | null; invoiceId: string | null; restrictionId: string | null };
export type ContractOption = { id: string; version: number; label: string; priceMinor: number; currency: Currency };

export const KL = "Asia/Kuala_Lumpur";
/** A contract or invoice date — a Kuala Lumpur business day — in the user's language: “10 Sept 2026”. */
export const businessDay = (iso: string, locale: Locale = "en") => showDate(iso, { locale, timeZone: KL });
const plain = (s: string) => s.replace(/[\u00a0\u2009\u202f]/g, " ");
/** The billing month of an invoice period in the user's language: “September 2026”. */
export const billingMonth = (iso: string, locale: Locale = "en") => plain(new Date(iso).toLocaleDateString(intlTag(locale), { timeZone: KL, month: "long", year: "numeric" }));
/** A YYYY-MM payout period in the user's language: “Sept 2026”. */
const periodName = (p: string, locale: Locale) =>
  /^\d{4}-\d{2}$/.test(p) ? plain(new Date(`${p}-15T00:00:00Z`).toLocaleDateString(intlTag(locale), { month: "short", year: "numeric", timeZone: "UTC" })) : p;
export const money = (minor: number, currency: string) => `${(minor / 100).toFixed(2)} ${currency}`;
const methodName = (m: string | null, t: T) => (m === "demo_credit_card" ? t("Credit card") : m === "demo_debit_card" ? t("Debit card") : m);
/** A payment's status as the screen words it; another code stays as it is. */
export const paymentWord = (s: string, t: T = en) =>
  ({ initiated: t("initiated"), processing: t("processing"), confirmed: t("confirmed"), failed: t("failed") })[s] ?? s;

/** Invoice state for HQ: unpaid past its due date is overdue (IR04). */
export function invoiceStatus(i: Pick<ApiInvoice, "status" | "dueAt">, now: Date): InvoiceStatus {
  if (i.status === "paid") return "Paid";
  if (i.status === "processing") return "Processing";
  return Date.parse(i.dueAt) < now.getTime() ? "Overdue" : "Unpaid";
}

export function invoiceRows(invoices: ApiInvoice[], contracts: ApiContract[], customers: ApiCustomer[], now: Date, i18n: I18n = EN): InvoiceRow[] {
  const { t, display: { locale } } = i18n;
  const contract = new Map(contracts.map((c) => [c.id, c]));
  const customer = new Map(customers.map((c) => [c.id, c.name]));
  return invoices.map((i) => {
    const k = contract.get(i.contractId);
    const st = invoiceStatus(i, now);
    // the method is read-only and "Not selected" when null — a manual payment never invents one (DD-A08)
    const name = methodName(i.paymentMethod, t) ?? t("Method not selected");
    const paymentState = i.paymentStatus ? paymentWord(i.paymentStatus, t) : null;
    return {
      id: i.id, version: i.version, number: i.number, amt: money(i.amountMinor, i.currency), amountMinor: i.amountMinor, currency: i.currency,
      meta: `${(k && customer.get(k.customerId)) ?? t("customer")} · ${k ? t("{plan} contract v{version}", { plan: k.planType, version: i.contractVersion }) : i.contractId} · ${billingMonth(i.period.from, locale)}`,
      method: paymentState ? `${name} · ${paymentState}` : name, methodName: name, paymentState, st, customerId: k?.customerId ?? "", contractId: i.contractId,
      period: `${businessDay(i.period.from, locale)} → ${businessDay(i.period.to, locale)}`, due: businessDay(i.dueAt, locale),
    };
  });
}

/** Totals per currency of the given statuses (never converted, FR-A08). */
export function totals(rows: InvoiceRow[], statuses: InvoiceStatus[]): string {
  const by = new Map<string, number>();
  for (const r of rows) if (statuses.includes(r.st)) by.set(r.currency, (by.get(r.currency) ?? 0) + r.amountMinor);
  return by.size === 0 ? "0.00" : [...by].map(([c, m]) => money(m, c)).join(" · ");
}

export function invoiceDetail(d: ApiInvoiceDetail, t: T = en): InvoiceDetail {
  const processing = d.paymentRefs.find((p) => p.status === "processing");
  return {
    id: d.id, version: d.version, processingPaymentId: processing?.id ?? null, processingPaymentVersion: processing?.version ?? null,
    payments: d.paymentRefs.map((p) => [
      methodName(p.method, t) ?? t("No method (recorded by HQ)"), paymentWord(p.status, t), p.paymentReference && t("ref {ref}", { ref: p.paymentReference }),
      p.confirmationReason && t("reason “{reason}”", { reason: p.confirmationReason }),
    ].filter(Boolean).join(" · ")),
    restrictionIds: d.restrictionIds,
  };
}

/** Inquiries, received first: the subject in the display language and the time received in the display time zone. */
export function inquiryRows(inquiries: ApiInquiry[], customers: ApiCustomer[], i: I18n = EN): InquiryRow[] {
  const { t } = i;
  const customer = new Map(customers.map((c) => [c.id, c.name]));
  return [...inquiries].sort((a, b) => (a.state === b.state ? b.createdAt.localeCompare(a.createdAt) : a.state === "received" ? -1 : 1)).map((q) => ({
    id: q.id, version: q.version, cust: customer.get(q.customerId) ?? t("customer"), kind: q.subjectType === "payment" ? t("Payment") : t("Cooling restriction"), at: showTime(q.createdAt, i.display),
    text: `“${q.message}”`, state: q.state, reply: q.reply, invoiceId: q.invoiceId, restrictionId: q.restrictionId,
  }));
}

export function contractOptions(contracts: ApiContract[], customers: ApiCustomer[], t: T = en): ContractOption[] {
  const customer = new Map(customers.map((c) => [c.id, c.name]));
  return contracts.map((k) => ({ id: k.id, version: k.version, label: `${customer.get(k.customerId) ?? t("customer")} · ${k.planType} · ${money(k.priceMinor, k.currency)}`, priceMinor: k.priceMinor, currency: k.currency }));
}

/** PayoutStatement of service-contracts.ts. */
export type ApiStatement = {
  id: string; version: number; contractorOrgId: string; period: string; status: "draft" | "approved" | "paid"; currency: string; grossMinor: number; deductionsMinor: number; netMinor: number; payDate: string;
  lines: { id: string; jobId: string; workType: string; acceptedAt: string | null; amountMinor: number; kind: "charge" | "deduction" | "adjustment"; note: string | null }[];
  queries: { id: string; lineId: string; topic: string; message: string; state: "open" | "answered" | "adjusted"; reply: string | null; adjustmentMinor: number | null }[];
};
export type ApiOrganization = { id: string; name: string; kind: string };
export type StatementRow = { id: string; version: number; contractor: string; period: string; status: "Draft" | "Approved" | "Paid"; jobs: number; net: string };
/** A statement's status, a line's kind, a question's topic and state as the screen words them. */
export const statementWord = (s: StatementRow["status"], t: T = en) => ({ Draft: t("Draft"), Approved: t("Approved"), Paid: t("Paid") })[s] ?? s;
export type StatementDetail = StatementRow & {
  gross: string; deductions: string; payDate: string; payable: boolean;
  lines: { id: string; job: string; work: string; kind: string; amount: string; note: string }[];
  queries: { id: string; line: string; topic: string; message: string; state: string; open: boolean; reply: string | null; adjustment: string | null }[];
};
const statusLabel = { draft: "Draft", approved: "Approved", paid: "Paid" } as const;

export function statementRows(statements: ApiStatement[], orgs: ApiOrganization[], i: I18n = EN): StatementRow[] {
  const org = new Map(orgs.map((o) => [o.id, o.name]));
  return [...statements].sort((a, b) => b.period.localeCompare(a.period) || a.contractorOrgId.localeCompare(b.contractorOrgId)).map((s) => ({
    id: s.id, version: s.version, contractor: org.get(s.contractorOrgId) ?? i.t("contractor"), period: periodName(s.period, i.display.locale), status: statusLabel[s.status],
    jobs: new Set(s.lines.map((l) => l.jobId)).size, net: money(s.netMinor, s.currency),
  }));
}

/** The statement detail; `payable` is whether its pay date has come (approved → paid only on or after it). The pay
 * date is a Kuala Lumpur business day. */
export function statementDetail(s: ApiStatement, orgs: ApiOrganization[], now: Date, i: I18n = EN): StatementDetail {
  const { t } = i;
  const [row] = statementRows([s], orgs, i);
  const line = new Map(s.lines.map((l) => [l.id, l]));
  const work: Record<string, string> = { periodic_inspection: t("Periodic inspection"), repair_base: t("Repair"), emergency: t("Emergency call-out"), rework_deduction: t("Rework deduction") };
  const kind: Record<string, string> = { charge: t("charge"), deduction: t("deduction"), adjustment: t("adjustment") };
  const topic: Record<string, string> = { amount: t("Amount"), deduction: t("Deduction"), missing_job: t("Missing job"), other: t("Other") };
  const state: Record<string, string> = { open: t("open"), answered: t("answered"), adjusted: t("adjusted") };
  return {
    ...row, gross: money(s.grossMinor, s.currency), deductions: money(s.deductionsMinor, s.currency), payDate: businessDay(s.payDate, i.display.locale), payable: Date.parse(s.payDate) <= now.getTime(),
    lines: s.lines.map((l) => ({ id: l.id, job: l.jobId, work: work[l.workType] ?? l.workType, kind: kind[l.kind] ?? l.kind, amount: l.kind === "deduction" ? `−${money(l.amountMinor, s.currency)}` : money(l.amountMinor, s.currency), note: l.note ?? "" })), // a minus sign, as on the adjustments
    queries: s.queries.map((q) => ({
      id: q.id, line: line.get(q.lineId)?.jobId ?? q.lineId, topic: topic[q.topic] ?? q.topic, message: q.message, state: state[q.state] ?? q.state, open: q.state === "open", reply: q.reply,
      adjustment: q.adjustmentMinor === null ? null : `${q.adjustmentMinor > 0 ? "+" : "−"}${money(Math.abs(q.adjustmentMinor), s.currency)}`,
    })),
  };
}
