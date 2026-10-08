// HQ billing (FR-A08, FR-A23, DATA_SOURCE=api): invoices, contracts, customers, inquiries and reminder recipients
// projected for the admin billing screen. Pure code shared by the Server Component and the client view.
export type Range = { from: string; to: string };
export type ApiInvoice = { id: string; version: number; number: string; amountMinor: number; currency: string; contractId: string; contractVersion: number; period: Range; dueAt: string; status: "unpaid" | "processing" | "paid"; paymentMethod: string | null; paymentStatus: string | null; paidAt: string | null };
export type ApiPayment = { id: string; version: number; amountMinor: number; currency: string; invoiceId: string; method: string | null; status: "initiated" | "processing" | "confirmed" | "failed"; paymentReference: string | null; confirmedAt: string | null; confirmationReason: string | null };
export type ApiInvoiceDetail = ApiInvoice & { paymentRefs: ApiPayment[]; restrictionIds: string[] };
export type ApiContract = { id: string; version: number; customerId: string; customerOrgId: string; unitIds: string[]; planType: string; startAt: string; endAt: string; priceMinor: number; currency: string; restrictionEligible: boolean };
export type ApiCustomer = { id: string; name: string; organizationId: string; status: string };
export type ApiProperty = { id: string; customerOrgId: string; name: string; archived: boolean };
export type ApiInquiry = { id: string; version: number; customerId: string; invoiceId: string | null; restrictionId: string | null; subjectType: "payment" | "restriction"; message: string; state: "received" | "answered"; reply: string | null; createdAt: string };
export type ApiRecipient = { id: string; role: string; displayLabel: string; allowedChannels: ("inApp" | "email" | "whatsapp")[] };

export type InvoiceStatus = "Unpaid" | "Overdue" | "Processing" | "Paid";
export type InvoiceRow = { id: string; version: number; number: string; amt: string; amountMinor: number; currency: string; meta: string; method: string; st: InvoiceStatus; customerId: string; contractId: string; period: string; due: string };
export type InvoiceDetail = { id: string; version: number; processingPaymentId: string | null; processingPaymentVersion: number | null; payments: string[]; restrictionIds: string[] };
export type InquiryRow = { id: string; version: number; cust: string; kind: string; at: string; text: string; state: "received" | "answered"; reply: string | null; invoiceId: string | null; restrictionId: string | null };
export type ContractOption = { id: string; version: number; label: string; priceMinor: number; currency: string };

const KL = "Asia/Kuala_Lumpur";
const day = (iso: string) => new Date(iso).toLocaleDateString("en-CA", { timeZone: KL });
const month = (iso: string) => new Date(iso).toLocaleDateString("en-US", { timeZone: KL, month: "short", year: "numeric" }); // "Sep 2026" (en-GB gives "Sept")
export const money = (minor: number, currency: string) => `${(minor / 100).toFixed(2)} ${currency}`;
const methodLabel: Record<string, string> = { demo_credit_card: "Credit card", demo_debit_card: "Debit card" };

/** Invoice state for HQ: unpaid past its due date is overdue (IR04). */
export function invoiceStatus(i: Pick<ApiInvoice, "status" | "dueAt">, now: Date): InvoiceStatus {
  if (i.status === "paid") return "Paid";
  if (i.status === "processing") return "Processing";
  return Date.parse(i.dueAt) < now.getTime() ? "Overdue" : "Unpaid";
}

export function invoiceRows(invoices: ApiInvoice[], contracts: ApiContract[], customers: ApiCustomer[], now: Date): InvoiceRow[] {
  const contract = new Map(contracts.map((c) => [c.id, c]));
  const customer = new Map(customers.map((c) => [c.id, c.name]));
  return invoices.map((i) => {
    const k = contract.get(i.contractId);
    const st = invoiceStatus(i, now);
    // the method is read-only and "Not selected" when null — a manual payment never invents one (DD-A08)
    const method = `${i.paymentMethod ? methodLabel[i.paymentMethod] ?? i.paymentMethod : "Method not selected"}${i.paymentStatus ? ` · ${i.paymentStatus}` : ""}`;
    return {
      id: i.id, version: i.version, number: i.number, amt: money(i.amountMinor, i.currency), amountMinor: i.amountMinor, currency: i.currency,
      meta: `${(k && customer.get(k.customerId)) ?? "customer"} · ${k ? `${k.planType} contract v${i.contractVersion}` : i.contractId} · ${month(i.period.from)}`,
      method, st, customerId: k?.customerId ?? "", contractId: i.contractId, period: `${day(i.period.from)} → ${day(i.period.to)}`, due: day(i.dueAt),
    };
  });
}

/** Totals per currency of the given statuses (never converted, FR-A08). */
export function totals(rows: InvoiceRow[], statuses: InvoiceStatus[]): string {
  const by = new Map<string, number>();
  for (const r of rows) if (statuses.includes(r.st)) by.set(r.currency, (by.get(r.currency) ?? 0) + r.amountMinor);
  return by.size === 0 ? "0.00" : [...by].map(([c, m]) => money(m, c)).join(" · ");
}

export function invoiceDetail(d: ApiInvoiceDetail): InvoiceDetail {
  const processing = d.paymentRefs.find((p) => p.status === "processing");
  return {
    id: d.id, version: d.version, processingPaymentId: processing?.id ?? null, processingPaymentVersion: processing?.version ?? null,
    payments: d.paymentRefs.map((p) => [p.method ? methodLabel[p.method] ?? p.method : "No method (recorded by HQ)", p.status, p.paymentReference && `ref ${p.paymentReference}`, p.confirmationReason && `reason “${p.confirmationReason}”`].filter(Boolean).join(" · ")),
    restrictionIds: d.restrictionIds,
  };
}

export function inquiryRows(inquiries: ApiInquiry[], customers: ApiCustomer[]): InquiryRow[] {
  const customer = new Map(customers.map((c) => [c.id, c.name]));
  return [...inquiries].sort((a, b) => (a.state === b.state ? b.createdAt.localeCompare(a.createdAt) : a.state === "received" ? -1 : 1)).map((q) => ({
    id: q.id, version: q.version, cust: customer.get(q.customerId) ?? "customer", kind: q.subjectType, at: day(q.createdAt), text: `“${q.message}”`, state: q.state, reply: q.reply, invoiceId: q.invoiceId, restrictionId: q.restrictionId,
  }));
}

export function contractOptions(contracts: ApiContract[], customers: ApiCustomer[]): ContractOption[] {
  const customer = new Map(customers.map((c) => [c.id, c.name]));
  return contracts.map((k) => ({ id: k.id, version: k.version, label: `${customer.get(k.customerId) ?? "customer"} · ${k.planType} · ${money(k.priceMinor, k.currency)}`, priceMinor: k.priceMinor, currency: k.currency }));
}

/** PayoutStatement of service-contracts.ts. */
export type ApiStatement = {
  id: string; version: number; contractorOrgId: string; period: string; status: "draft" | "approved" | "paid"; currency: string; grossMinor: number; deductionsMinor: number; netMinor: number; payDate: string;
  lines: { id: string; jobId: string; workType: string; acceptedAt: string | null; amountMinor: number; kind: "charge" | "deduction" | "adjustment"; note: string | null }[];
  queries: { id: string; lineId: string; topic: string; message: string; state: "open" | "answered" | "adjusted"; reply: string | null; adjustmentMinor: number | null }[];
};
export type ApiOrganization = { id: string; name: string; kind: string };
export type StatementRow = { id: string; version: number; contractor: string; period: string; status: "Draft" | "Approved" | "Paid"; jobs: number; net: string };
export type StatementDetail = StatementRow & {
  gross: string; deductions: string; payDate: string; payable: boolean;
  lines: { id: string; job: string; work: string; kind: string; amount: string; note: string }[];
  queries: { id: string; line: string; topic: string; message: string; state: string; reply: string | null; adjustment: string | null }[];
};
const statusLabel = { draft: "Draft", approved: "Approved", paid: "Paid" } as const;

export function statementRows(statements: ApiStatement[], orgs: ApiOrganization[]): StatementRow[] {
  const org = new Map(orgs.map((o) => [o.id, o.name]));
  return [...statements].sort((a, b) => b.period.localeCompare(a.period) || a.contractorOrgId.localeCompare(b.contractorOrgId)).map((s) => ({
    id: s.id, version: s.version, contractor: org.get(s.contractorOrgId) ?? "contractor", period: s.period, status: statusLabel[s.status],
    jobs: new Set(s.lines.map((l) => l.jobId)).size, net: money(s.netMinor, s.currency),
  }));
}

/** The statement detail; `payable` is whether its pay date has come (approved → paid only on or after it). */
export function statementDetail(s: ApiStatement, orgs: ApiOrganization[], now: Date): StatementDetail {
  const [row] = statementRows([s], orgs);
  const line = new Map(s.lines.map((l) => [l.id, l]));
  return {
    ...row, gross: money(s.grossMinor, s.currency), deductions: money(s.deductionsMinor, s.currency), payDate: day(s.payDate), payable: Date.parse(s.payDate) <= now.getTime(),
    lines: s.lines.map((l) => ({ id: l.id, job: l.jobId, work: l.workType, kind: l.kind, amount: money(l.kind === "deduction" ? -l.amountMinor : l.amountMinor, s.currency), note: l.note ?? "" })),
    queries: s.queries.map((q) => ({
      id: q.id, line: line.get(q.lineId)?.jobId ?? q.lineId, topic: q.topic, message: q.message, state: q.state, reply: q.reply,
      adjustment: q.adjustmentMinor === null ? null : `${q.adjustmentMinor > 0 ? "+" : "−"}${money(Math.abs(q.adjustmentMinor), s.currency)}`,
    })),
  };
}

/** The reminder text: the invoice name from notifications.preview (params.targetName) with the invoice's amount and
 * due date (the preview carries neither). */
export function reminderText(targetName: string, row: Pick<InvoiceRow, "amt" | "due">): string {
  return `Your invoice ${targetName} (${row.amt}) was due on ${row.due}. Pay by card or view payment instructions in the app →`;
}
