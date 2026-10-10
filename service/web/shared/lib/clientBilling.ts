// Customer contracts & payments (FR-C10–C12, DATA_SOURCE=api): the customer's own contracts, invoices, demo payments,
// cooling-restriction notice and inquiries projected for /customer/payments. Pure code shared by the Server Components
// and the client views. Texts in the display language (`t` / `i`, IR267). Contract periods, due dates and billing months
// are Kuala Lumpur business days (REV18-035) written in the user's language; payments, inquiries, restriction events and
// reminders are instants in the user's display time zone (IR44).
import { amount } from "@ac/web/lib/energy";
import { billingMonth, businessDay, invoiceStatus, KL, type ApiContract, type ApiInquiry, type ApiInvoice, type ApiPayment, type InvoiceStatus } from "@ac/web/lib/billing";
import { policyText, stateLabel, type ApiCommand, type ApiRestriction, type RestrictionState } from "@ac/web/lib/restrictions";
import { EN, intlTag, showDate, showTime, translator, type I18n, type Locale, type T } from "@ac/web/lib/i18n";

const en = translator("en");
// The business-day helpers live with the billing types (HQ billing words its dates the same way, IR299).
export { billingMonth, businessDay, KL };
/** The same day without the year (“1 Sept”), for the start of a range whose end shows the year. */
const plain = (s: string) => s.replace(/[\u00a0\u2009\u202f]/g, " ");
const businessDayShort = (iso: string, locale: Locale) => plain(new Date(iso).toLocaleDateString(intlTag(locale), { timeZone: KL, month: "short", day: "numeric" }));

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
/** The customer's contracts, current ones first (a contract's end does not stop monitoring or control). `status` stays
 * the code the badge tone keys on; the view translates it. */
export function contractCards(cs: ClientContract[], unit: (id: string) => UnitPlace | undefined, now: Date, i: I18n = EN): ContractCard[] {
  const { t, display: { locale } } = i;
  const ms = now.getTime();
  const status = (k: ApiContract): ContractCard["status"] => (ms < Date.parse(k.startAt) ? "Upcoming" : ms >= Date.parse(k.endAt) ? "Expired" : "Active");
  const rank = { Active: 0, Upcoming: 1, Expired: 2 };
  return [...cs].sort((a, b) => rank[status(a)] - rank[status(b)] || b.startAt.localeCompare(a.startAt)).map((k) => ({
    id: k.id, version: k.version, name: planName[k.planType] ? t(planName[k.planType]) : k.planType,
    sub: `${k.id.slice(0, 8)} · ${businessDayShort(k.startAt, locale)} → ${businessDay(k.endAt, locale)}`,
    period: `${businessDay(k.startAt, locale)} → ${businessDay(k.endAt, locale)}`, plan: planLong[k.planType] ? t(planLong[k.planType]) : k.planType, planType: k.planType, status: status(k),
    units: k.unitIds.map((id) => ({ id, ...(unit(id) ?? { name: id.slice(0, 8), place: "" }) })), restrictionIds: k.activeRestrictionIds ?? [],
  }));
}

export type ClientInvoiceRow = { id: string; version: number; number: string; amount: string; due: string; status: InvoiceStatus; payable: boolean; contractId: string };
/** Newest due date first; unpaid past the due date is Overdue, Processing is not paid (DD-C10). The due date is the
 * Kuala Lumpur business day; the payment date is the user's day. */
export function clientInvoiceRows(is: ApiInvoice[], now: Date, i: I18n = EN): ClientInvoiceRow[] {
  const { t, display } = i;
  return [...is].sort((a, b) => b.dueAt.localeCompare(a.dueAt)).map((inv) => {
    const st = invoiceStatus(inv, now);
    return {
      id: inv.id, version: inv.version, number: inv.number, amount: amount(inv.amountMinor, inv.currency),
      due: [t("Due {date}", { date: businessDay(inv.dueAt, display.locale) }), ...(inv.paidAt ? [t("paid {date}", { date: showDate(inv.paidAt, display) })] : [])].join(" · "),
      status: st, payable: st === "Unpaid" || st === "Overdue", contractId: inv.contractId,
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
export function paymentLines(ps: ClientPayment[], i: I18n = EN): PaymentLine[] {
  const { t, display } = i;
  return [...ps].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).map((p) => ({
    id: p.id, version: p.version, label: `${p.id.slice(0, 8)} · ${p.method ? (methodLabel[p.method] ? t(methodLabel[p.method]) : p.method) : t("Bank transfer (recorded by HQ)")}`,
    at: showTime(p.confirmedAt ?? p.updatedAt, display), status: p.status, reference: p.paymentReference,
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
export function restrictionNotice(r: ApiRestriction, unit: (id: string) => UnitPlace | undefined, t: T = en): RestrictionNotice {
  const names = r.unitIds.map((id) => { const u = unit(id); return u ? `${u.name}${u.place ? ` (${u.place})` : ""}` : id.slice(0, 8); });
  return {
    id: r.id, state: r.state, label: t(stateLabel[r.state]), policy: policyText(r.policy, t), live: live.includes(r.state),
    units: t(r.unitIds.length === 1 ? "{n} unit in scope · {names}" : "{n} units in scope · {names}", { n: r.unitIds.length, names: names.join(", ") }),
  };
}
export type NoticeUnit = { unitId: string; name: string; apply: string; release: string; releaseTone: "ok" | "crit" | "primary"; note: string; tone: "warn" | "ok" | "unknown" | "primary" | "muted" };
const applyText: Record<string, string> = { not_sent: "Not sent yet", sent_unknown: "Result unknown", applied: "Applied", not_applied: "Not applied" };
const releaseText: Record<string, string> = { none: "", waiting_reconcile: "Waiting for reconciliation", requested: "Release sent", released: "Released", not_required: "Release not needed", failed: "Release failed" };
/** Per-unit state as the customer sees it: never “released” before the unit confirms (FR-C12). */
export function noticeUnits(r: ApiRestriction, unit: (id: string) => UnitPlace | undefined, commands: Map<string, ApiCommand>, t: T = en): NoticeUnit[] {
  return r.perUnit.map((u) => {
    const last = u.releaseCommandIds.at(-1) ?? u.applyCommandIds.at(-1);
    const cmd = last ? commands.get(last) : undefined;
    const waiting = u.releaseState === "requested" && cmd && (cmd.status === "requested" || cmd.status === "sent");
    const note = [
      u.pendingReason === "offline" ? t("the unit is offline") : null,
      waiting ? t("waiting for the unit to confirm") : null,
      cmd?.status === "failed" ? t("last command failed") : cmd?.status === "expired" ? t("last command expired") : null,
    ].filter(Boolean).join(" · ");
    const n = unit(u.unitId);
    const release = releaseText[u.releaseState];
    return {
      unitId: u.unitId, name: n ? `${n.name}${n.place ? ` · ${n.place}` : ""}` : u.unitId.slice(0, 8), apply: applyText[u.applyState] ? t(applyText[u.applyState]) : u.applyState,
      release: release === undefined ? u.releaseState : release && t(release), releaseTone: u.releaseState === "released" ? "ok" : u.releaseState === "failed" ? "crit" : "primary",
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
export function noticeTimeline(r: ApiRestriction, i: I18n = EN): { time: string; title: string; tone?: "warn" | "ok" }[] {
  const { t, display } = i;
  const items: { time: string; title: string; tone?: "warn" | "ok" }[] = (r.events ?? []).map((e) => ({
    time: showTime(e.occurredAt, display), title: eventTitle[e.action] ? t(eventTitle[e.action]) : e.action,
    ...(e.action === "restriction.applied" ? { tone: "warn" as const } : e.action === "restriction.released" ? { tone: "ok" as const } : {}),
  }));
  if (r.state !== "released" && r.state !== "cancelled") items.push({ time: "—", title: t("Released (only when every unit confirms)"), tone: "ok" });
  return items;
}

// ---- inquiries and message previews ----

export type PreviewParams = { targetName: string; at: string; status: string; amountMinor: number | null; currency: string | null; method: string | null };

export type InquiryLine = { id: string; subject: string; at: string; message: string; state: ApiInquiry["state"]; reply: string | null };
export const inquiryLines = (is: ApiInquiry[], i: I18n = EN): InquiryLine[] => [...is].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).map((q) => ({
  id: q.id, subject: i.t(q.subjectType === "payment" ? "Payment" : "Cooling restriction"), at: showTime(q.createdAt, i.display), message: q.message, state: q.state, reply: q.reply,
}));
/** The payment message of a channel for this invoice (notifications.preview checks the channel; the text uses the
 * invoice itself, whose due date and amount the preview parameters do not carry), in the recipient's language. */
export function previewText(channel: string, inv: { number: string; amount: string; dueDate: string; status: InvoiceStatus }, t: T = en): { subject: string | null; body: string } {
  const p = { number: inv.number, amount: inv.amount, date: inv.dueDate };
  const subject = channel === "email" ? t(inv.status === "Paid" ? "Payment received — {number}" : "Payment reminder — {number}", p) : null;
  const body = inv.status === "Paid" ? t("Hello, {number} ({amount}) is paid — thank you.", p)
    : inv.status === "Processing" ? t("Hello, your payment for {number} ({amount}) is being processed. It is not paid yet.", p)
    : t(inv.status === "Overdue" ? "Hello, {number} ({amount}) was due on {date}. Pay with a demo card or view payment instructions in the app." : "Hello, {number} ({amount}) is due on {date}. Pay with a demo card or view payment instructions in the app.", p);
  return { subject, body };
}
