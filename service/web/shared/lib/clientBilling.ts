// Customer contracts & payments (FR-C10–C12, DATA_SOURCE=api): the customer's own contracts, invoices, demo payments,
// cooling-restriction notice and inquiries projected for /customer/payments. Pure code shared by the Server Components
// and the client views.
import { amount, klStamp } from "@ac/web/lib/energy";
import { invoiceStatus, type ApiContract, type ApiInquiry, type ApiInvoice, type ApiPayment, type InvoiceStatus } from "@ac/web/lib/billing";
import { policyText, stateLabel, type ApiCommand, type ApiRestriction, type RestrictionState } from "@ac/web/lib/restrictions";

const KL = "Asia/Kuala_Lumpur";
/** "Sep 10, 2026" in Kuala Lumpur. */
export const longDay = (iso: string) => new Date(iso).toLocaleDateString("en-US", { timeZone: KL, month: "short", day: "numeric", year: "numeric" });
const shortDay = (iso: string) => new Date(iso).toLocaleDateString("en-US", { timeZone: KL, month: "short", day: "numeric" });
const hm = (iso: string) => klStamp(iso).slice(11);

export const planName: Record<string, string> = { rto: "RTO Plan", general: "General maintenance", energy: "Energy service", environment: "Environment service" };
const planLong: Record<string, string> = { rto: "Rent-to-own (RTO)", general: "General maintenance", energy: "Energy service", environment: "Environment service" };

export type UnitPlace = { name: string; place: string };
/** Unit names with their location path ("Home A › 1F › Bedroom") from units.list, properties.list and spaces.list. */
export function unitPlaces(us: { id: string; displayName: string; propertyId: string; spaceId: string | null }[], ps: { id: string; name: string }[],
  ss: { id: string; name: string; parentSpaceId: string | null }[]): Map<string, UnitPlace> {
  const prop = new Map(ps.map((p) => [p.id, p.name]));
  const space = new Map(ss.map((x) => [x.id, x]));
  const path = (id: string | null) => {
    const out: string[] = [];
    let x = id ? space.get(id) : undefined;
    while (x) {
      out.unshift(x.name);
      x = x.parentSpaceId ? space.get(x.parentSpaceId) : undefined;
    }
    return out;
  };
  return new Map(us.map((u) => [u.id, { name: u.displayName, place: [prop.get(u.propertyId) ?? "", ...path(u.spaceId)].filter(Boolean).join(" › ") }]));
}
export type ClientContract = ApiContract & { activeRestrictionIds?: string[] };
export type ContractCard = {
  id: string; version: number; name: string; sub: string; period: string; plan: string; planType: string; status: "Active" | "Expired" | "Upcoming";
  units: (UnitPlace & { id: string })[]; restrictionIds: string[];
};
/** The customer's contracts, current ones first (a contract's end does not stop monitoring or control). */
export function contractCards(cs: ClientContract[], unit: (id: string) => UnitPlace | undefined, now: Date): ContractCard[] {
  const t = now.getTime();
  const status = (k: ApiContract): ContractCard["status"] => (t < Date.parse(k.startAt) ? "Upcoming" : t >= Date.parse(k.endAt) ? "Expired" : "Active");
  const rank = { Active: 0, Upcoming: 1, Expired: 2 };
  return [...cs].sort((a, b) => rank[status(a)] - rank[status(b)] || b.startAt.localeCompare(a.startAt)).map((k) => ({
    id: k.id, version: k.version, name: planName[k.planType] ?? k.planType, sub: `${k.id.slice(0, 8)} · ${shortDay(k.startAt)} → ${longDay(k.endAt)}`,
    period: `${longDay(k.startAt)} → ${longDay(k.endAt)}`, plan: planLong[k.planType] ?? k.planType, planType: k.planType, status: status(k),
    units: k.unitIds.map((id) => ({ id, ...(unit(id) ?? { name: id.slice(0, 8), place: "" }) })), restrictionIds: k.activeRestrictionIds ?? [],
  }));
}

export type ClientInvoiceRow = { id: string; version: number; number: string; amount: string; due: string; status: InvoiceStatus; payable: boolean; contractId: string };
/** Newest due date first; unpaid past the due date is Overdue, Processing is not paid (DD-C10). */
export function clientInvoiceRows(is: ApiInvoice[], now: Date): ClientInvoiceRow[] {
  return [...is].sort((a, b) => b.dueAt.localeCompare(a.dueAt)).map((i) => {
    const st = invoiceStatus(i, now);
    return {
      id: i.id, version: i.version, number: i.number, amount: amount(i.amountMinor, i.currency), due: `Due ${longDay(i.dueAt)}${i.paidAt ? ` · paid ${shortDay(i.paidAt)}` : ""}`,
      status: st, payable: st === "Unpaid" || st === "Overdue", contractId: i.contractId,
    };
  });
}
/** The status filter of the URL as invoices.list filters (overdue = overdueOnly, all = none). */
export type StatusFilter = "all" | "unpaid" | "overdue" | "processing" | "paid";
export const statusFilters: StatusFilter[] = ["all", "unpaid", "overdue", "processing", "paid"];
export const statusQuery = (f: StatusFilter) => (f === "all" ? {} : f === "overdue" ? { overdueOnly: true } : { status: f });

// ---- one invoice (FR-C11) ----

export type ClientPayment = ApiPayment & { createdAt: string; updatedAt: string };
export const methodLabel: Record<string, string> = { demo_credit_card: "Credit card (demo)", demo_debit_card: "Debit card (demo)" };
export type PaymentLine = { id: string; version: number; label: string; at: string; status: ClientPayment["status"]; reference: string | null };
export function paymentLines(ps: ClientPayment[]): PaymentLine[] {
  return [...ps].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).map((p) => ({
    id: p.id, version: p.version, label: `${p.id.slice(0, 8)} · ${p.method ? methodLabel[p.method] ?? p.method : "Bank transfer (recorded by HQ)"}`,
    at: `${shortDay(p.confirmedAt ?? p.updatedAt)} ${hm(p.confirmedAt ?? p.updatedAt)}`, status: p.status, reference: p.paymentReference,
  }));
}
/** The payment state the invoice page shows: the open payment (initiated or processing), else the last one. */
export function paymentPhase(ps: ClientPayment[]): { open: ClientPayment | null; last: ClientPayment | null } {
  const sorted = [...ps].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  return { open: sorted.find((p) => p.status === "initiated" || p.status === "processing") ?? null, last: sorted[0] ?? null };
}
/** The demo PSP reference of a payment (unique per payment, as payments.simulate requires for confirm). */
export const demoReference = (paymentId: string) => `DEMO-PAY-${paymentId.replace(/-/g, "").slice(-8).toUpperCase()}`;

// ---- cooling restriction notice (FR-C12) ----

const live: RestrictionState[] = ["scheduled", "requested", "applied", "release_requested"];
export type RestrictionNotice = { id: string; state: RestrictionState; label: string; policy: string; units: string; live: boolean };
export function restrictionNotice(r: ApiRestriction, unit: (id: string) => UnitPlace | undefined): RestrictionNotice {
  const names = r.unitIds.map((id) => { const u = unit(id); return u ? `${u.name}${u.place ? ` (${u.place})` : ""}` : id.slice(0, 8); });
  return {
    id: r.id, state: r.state, label: stateLabel[r.state], policy: policyText(r.policy), live: live.includes(r.state),
    units: `${r.unitIds.length} unit${r.unitIds.length === 1 ? "" : "s"} in scope · ${names.join(", ")}`,
  };
}
export type NoticeUnit = { unitId: string; name: string; apply: string; release: string; note: string; tone: "warn" | "ok" | "unknown" | "primary" | "muted" };
const applyText: Record<string, string> = { not_sent: "Not sent yet", sent_unknown: "Result unknown", applied: "Applied", not_applied: "Not applied" };
const releaseText: Record<string, string> = { none: "", waiting_reconcile: "Waiting for reconciliation", requested: "Release sent", released: "Released", not_required: "Release not needed", failed: "Release failed" };
/** Per-unit state as the customer sees it: never “released” before the unit confirms (FR-C12). */
export function noticeUnits(r: ApiRestriction, unit: (id: string) => UnitPlace | undefined, commands: Map<string, ApiCommand>): NoticeUnit[] {
  return r.perUnit.map((u) => {
    const last = u.releaseCommandIds.at(-1) ?? u.applyCommandIds.at(-1);
    const cmd = last ? commands.get(last) : undefined;
    const waiting = u.releaseState === "requested" && cmd && (cmd.status === "requested" || cmd.status === "sent");
    const note = [
      u.pendingReason === "offline" ? "the unit is offline" : null,
      waiting ? "waiting for the unit to confirm" : null,
      cmd?.status === "failed" || cmd?.status === "expired" ? `last command ${cmd.status}` : null,
    ].filter(Boolean).join(" · ");
    const n = unit(u.unitId);
    return {
      unitId: u.unitId, name: n ? `${n.name}${n.place ? ` · ${n.place}` : ""}` : u.unitId.slice(0, 8), apply: applyText[u.applyState] ?? u.applyState, release: releaseText[u.releaseState] ?? u.releaseState,
      note, tone: u.releaseState === "released" ? "ok" : u.applyState === "sent_unknown" ? "unknown" : u.applyState === "applied" ? "warn" : "primary",
    };
  });
}
const eventTitle: Record<string, string> = {
  "restrictions.schedule": "Scheduled", "restrictions.execute": "Requested", "restriction.applied": "Applied", "restrictions.release": "Release requested",
  "payments.release": "Release requested after payment", "restrictions.cancel": "Cancelled — release requested", "restrictions.override": "Release requested by HQ",
  "restriction.released": "Released", "restrictions.defer": "Grace period granted", "restrictions.exempt": "Exception granted",
};
/** The state events the customer may see (IR42 projection), then the pending “Released” step. */
export function noticeTimeline(r: ApiRestriction): { time: string; title: string; tone?: "warn" | "ok" }[] {
  const items: { time: string; title: string; tone?: "warn" | "ok" }[] = (r.events ?? []).map((e) => ({
    time: `${shortDay(e.occurredAt)} ${hm(e.occurredAt)}`, title: eventTitle[e.action] ?? e.action,
    ...(e.action === "restriction.applied" ? { tone: "warn" as const } : e.action === "restriction.released" ? { tone: "ok" as const } : {}),
  }));
  if (r.state !== "released" && r.state !== "cancelled") items.push({ time: "—", title: "Released (only when every unit confirms)", tone: "ok" });
  return items;
}

// ---- inquiries and message previews ----

export type PreviewParams = { targetName: string; at: string; status: string; amountMinor: number | null; currency: string | null; method: string | null };

export type InquiryLine = { id: string; subject: string; at: string; message: string; state: ApiInquiry["state"]; reply: string | null };
export const inquiryLines = (is: ApiInquiry[]): InquiryLine[] => [...is].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).map((i) => ({
  id: i.id, subject: i.subjectType === "payment" ? "Payment" : "Cooling restriction", at: `${longDay(i.createdAt)} ${hm(i.createdAt)}`, message: i.message, state: i.state, reply: i.reply,
}));
/** The payment message of a channel for this invoice (notifications.preview checks the channel; the text uses the
 * invoice itself, whose due date and amount the preview parameters do not carry). */
export function previewText(channel: string, inv: { number: string; amount: string; dueDate: string; status: InvoiceStatus }): { subject: string | null; body: string } {
  const subject = channel === "email" ? `${inv.status === "Paid" ? "Payment received" : "Payment reminder"} — ${inv.number}` : null;
  const body = inv.status === "Paid" ? `Hello, ${inv.number} (${inv.amount}) is paid — thank you.`
    : inv.status === "Processing" ? `Hello, your payment for ${inv.number} (${inv.amount}) is being processed. It is not paid yet.`
    : `Hello, ${inv.number} (${inv.amount}) ${inv.status === "Overdue" ? "was" : "is"} due on ${inv.dueDate}. Pay with a demo card or view payment instructions in the app.`;
  return { subject, body };
}
