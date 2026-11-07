// Customer contracts & payments (FR-C10–C12, DATA_SOURCE=API): the customer's own contracts, invoices, demo payments,
// cooling-restriction notice and inquiries projected for /customer/payments. Pure code shared by the Server Components
// and the client views. Texts in the display language (`t` / `i`, IR267). Contract periods, due dates and billing months
// are Kuala Lumpur business days (REV18-035) written in the user's language; payments, inquiries, restriction events and
// reminders are instants in the user's display time zone (IR44).
import { amount } from "@ac/web/lib/energy";
import {  businessDay, invoiceStatus, KL, type ApiContract, type ApiInquiry, type ApiInvoice, type ApiPayment, type InvoiceStatus } from "@ac/web/lib/billing";
import { policyText, stateLabel, type ApiCommand, type ApiRestriction, type RestrictionState } from "@ac/web/lib/restrictions";
import { money } from "@ac/web/lib/money";
import { EN, intlTag, showDate, showTime, translator, type I18n, type Locale, type T, textOf } from "@ac/web/lib/i18n";

const en = translator("en");
// The business-day helpers live with the billing types (HQ billing words its dates the same way, IR299).

/** The same day without the year (“1 Sept”), for the start of a range whose end shows the year. */
const plain = (s: string) => s.replaceAll(/[\u{A0}\u{2009}\u{202F}]/gu, " ");
const businessDayShort = (iso: string, locale: Locale) => plain(new Date(iso).toLocaleDateString(intlTag(locale), { timeZone: KL, month: "short", day: "numeric" }));

const planName: Record<string, string> = { rto: "RTO Plan", general: "General maintenance", energy: "Energy service", environment: "Environment service" };
const planLong: Record<string, string> = { rto: "Rent-to-own (RTO)", general: "General maintenance", energy: "Energy service", environment: "Environment service" };

export interface UnitPlace { name: string; place: string }
/** Unit names with their location path ("Home A › 1F › Bedroom") from units.list, properties.list and spaces.list. */
export function unitPlaces(us: { id: string; displayName: string; propertyId: string; spaceId: string | null }[], ps: { id: string; name: string }[],
  ss: { id: string; name: string; parentSpaceId: string | null }[]): Map<string, UnitPlace> {
  const property = new Map(ps.map((p) => [p.id, p.name]));
  const space = new Map(ss.map((x) => [x.id, x]));
  const path = (id: string | null) => {
    let out: string[] = [];
    let x = id ? space.get(id) : undefined;
    while (x) {
      out = [x.name, ...out];
      x = x.parentSpaceId ? space.get(x.parentSpaceId) : undefined;
    }
    return out;
  };
  return new Map(us.map((u) => [u.id, { name: u.displayName, place: [property.get(u.propertyId) ?? "", ...path(u.spaceId)].filter(Boolean).join(" › ") }]));
}
export type ClientContract = ApiContract & { activeRestrictionIds?: string[] };
export interface ContractCard {
  id: string; version: number; name: string; sub: string; period: string; plan: string; planType: string; status: "Active" | "Expired" | "Upcoming";
  units: (UnitPlace & { id: string })[]; restrictionIds: string[];
}
/** The customer's contracts, current ones first (a contract's end does not stop monitoring or control). `status` stays
the code the badge tone keys on; the view translates it. */
export function contractCards(cs: ClientContract[], unit: (id: string) => UnitPlace | undefined, now: Date, i: I18n = EN): ContractCard[] {
  const { t, display: { locale } } = i;
  const ms = now.getTime();
  const status = (k: ApiContract): ContractCard["status"] => {
    if (ms < Date.parse(k.startAt)) return "Upcoming";
    return ms >= Date.parse(k.endAt) ? "Expired" : "Active";
  };
  const rank = { Active: 0, Upcoming: 1, Expired: 2 };
  return [...cs].toSorted((a, b) => rank[status(a)] - rank[status(b)] || b.startAt.localeCompare(a.startAt)).map((k) => {
    return {
      id: k.id, version: k.version, name: textOf(planName, k.planType, t) ?? k.planType,
      sub: `${k.id.slice(0, 8)} · ${businessDayShort(k.startAt, locale)} → ${businessDay(k.endAt, locale)}`,
      period: `${businessDay(k.startAt, locale)} → ${businessDay(k.endAt, locale)}`, plan: textOf(planLong, k.planType, t) ?? k.planType, planType: k.planType, status: status(k),
      units: k.unitIds.map((id) => ({ id, ...(unit(id) ?? { name: id.slice(0, 8), place: "" }) })), restrictionIds: k.activeRestrictionIds ?? [],
    };
  });
}

export interface ClientInvoiceRow { id: string; version: number; number: string; amount: string; due: string; status: InvoiceStatus; payable: boolean; contractId: string }
/** Newest due date first; unpaid past the due date is Overdue, Processing is not paid (DD-C10). The due date is the
Kuala Lumpur business day; the payment date is the user's day. */
export function clientInvoiceRows(is: ApiInvoice[], now: Date, i: I18n = EN): ClientInvoiceRow[] {
  const { t, display } = i;
  return [...is].toSorted((a, b) => b.dueAt.localeCompare(a.dueAt)).map((inv) => {
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
export function statusQuery(f: StatusFilter): { overdueOnly?: true; status?: Exclude<StatusFilter, "all" | "overdue"> } {
  if (f === "all") return {};
  return f === "overdue" ? { overdueOnly: true } : { status: f };
}

// ---- one invoice (FR-C11) ----

export type ClientPayment = ApiPayment & { createdAt: string; updatedAt: string };
export const methodLabel: Record<string, string> = { demo_credit_card: "Credit card (demo)", demo_debit_card: "Debit card (demo)" };
export interface PaymentLine { id: string; version: number; label: string; at: string; status: ClientPayment["status"]; reference: string | null }
export function paymentLines(ps: ClientPayment[], i: I18n = EN): PaymentLine[] {
  const { t, display } = i;
  return [...ps].toSorted((a, b) => b.createdAt.localeCompare(a.createdAt)).map((p) => {
    return {
      id: p.id, version: p.version, label: `${p.id.slice(0, 8)} · ${p.method ? (textOf(methodLabel, p.method, t) ?? p.method) : t("Bank transfer (recorded by HQ)")}`,
      at: showTime(p.confirmedAt ?? p.updatedAt, display), status: p.status, reference: p.paymentReference,
    };
  });
}
/** The payment state the invoice page shows: the open payment (initiated or processing), else the last one. */
export function paymentPhase(ps: ClientPayment[]): { open: ClientPayment | null; last: ClientPayment | null } {
  const sorted = [...ps].toSorted((a, b) => b.createdAt.localeCompare(a.createdAt));
  return { open: sorted.find((p) => p.status === "initiated" || p.status === "processing") ?? null, last: sorted[0] ?? null };
}
/** The demo PSP reference of a payment (unique per payment, as payments.simulate requires for confirm). */
export const demoReference = (paymentId: string) => `DEMO-PAY-${paymentId.replaceAll('-', "").slice(-8).toUpperCase()}`;

// ---- cooling restriction notice (FR-C12) ----

const live = new Set<RestrictionState>(["scheduled", "requested", "applied", "release_requested"]);
export interface RestrictionNotice { id: string; state: RestrictionState; label: string; policy: string; units: string; live: boolean }
/** A unit's name with its place in the given brackets, or the start of its ID when it is unknown. */
function unitLabel(id: string, u: UnitPlace | undefined, around: (place: string) => string): string {
  if (!u) return id.slice(0, 8);
  return u.place ? `${u.name}${around(u.place)}` : u.name;
}
export function restrictionNotice(r: ApiRestriction, unit: (id: string) => UnitPlace | undefined, t: T = en): RestrictionNotice {
  const names = r.unitIds.map((id) => unitLabel(id, unit(id), (place) => ` (${place})`));
  return {
    id: r.id, state: r.state, label: t(stateLabel[r.state]), policy: policyText(r.policy, t), live: live.has(r.state),
    units: t(r.unitIds.length === 1 ? "{n} unit in scope · {names}" : "{n} units in scope · {names}", { n: r.unitIds.length, names: names.join(", ") }),
  };
}
export interface NoticeUnit { unitId: string; name: string; apply: string; release: string; releaseTone: "ok" | "crit" | "primary"; note: string; tone: "warn" | "ok" | "unknown" | "primary" | "muted" }
const applyText: Record<string, string> = { not_sent: "Not sent yet", sent_unknown: "Result unknown", applied: "Applied", not_applied: "Not applied" };
const releaseText: Record<string, string> = { none: "", waiting_reconcile: "Waiting for reconciliation", requested: "Release sent", released: "Released", not_required: "Release not needed", failed: "Release failed" };
const PENDING_TEXT: Record<string, string> = { offline: "the unit is offline", device_operation_running: "the unit is busy with a device update" };
const COMMAND_TEXT: Record<string, string> = { failed: "last command failed", expired: "last command expired" };
const RELEASE_TONE = new Map<string, NoticeUnit["releaseTone"]>([["released", "ok"], ["failed", "crit"]]);
/** A unit's colour: released, its result unknown, restricted, or on its way. */
function unitTone(u: ApiRestriction["perUnit"][number]): NoticeUnit["tone"] {
  if (u.releaseState === "released") return "ok";
  if (u.applyState === "sent_unknown") return "unknown";
  return u.applyState === "applied" ? "warn" : "primary";
}
/** Per-unit state as the customer sees it: never “released” before the unit confirms (FR-C12). */
export function noticeUnits(r: ApiRestriction, unit: (id: string) => UnitPlace | undefined, commands: Map<string, ApiCommand>, t: T = en): NoticeUnit[] {
  return r.perUnit.map((u) => {
    const last = u.releaseCommandIds.at(-1) ?? u.applyCommandIds.at(-1);
    const deviceCommand = last ? commands.get(last) : undefined;
    const waiting = u.releaseState === "requested" && deviceCommand && (deviceCommand.status === "requested" || deviceCommand.status === "sent");
    const note = [
      textOf(PENDING_TEXT, u.pendingReason, t),
      waiting ? t("waiting for the unit to confirm") : null,
      textOf(COMMAND_TEXT, deviceCommand?.status, t),
    ].filter(Boolean).join(" · ");
    const n = unit(u.unitId);
    const release = releaseText[u.releaseState];
    return {
      unitId: u.unitId,
      name: unitLabel(u.unitId, n, (place) => ` · ${place}`),
      apply: textOf(applyText, u.applyState, t) ?? u.applyState,
      release: release === undefined ? u.releaseState : release && t(release),
      releaseTone: RELEASE_TONE.get(u.releaseState) ?? "primary",
      tone: unitTone(u),
      note,
    };
  });
}
const eventTitle: Record<string, string> = {
  "restrictions.schedule": "Scheduled", "restrictions.execute": "Requested", "restriction.applied": "Applied", "restrictions.release": "Release requested",
  "payments.release": "Release requested after payment", "restrictions.cancel": "Cancelled — release requested", "restrictions.override": "Release requested by HQ",
  "restriction.released": "Released", "restrictions.defer": "Grace period granted", "restrictions.exempt": "Exception granted",
};
function eventTone(action: string): { tone?: "warn" | "ok" } {
  if (action === "restriction.applied") return { tone: "warn" };
  return action === "restriction.released" ? { tone: "ok" } : {};
}
/** The state events the customer may see (IR42 projection), then the pending “Released” step. */
export function noticeTimeline(r: ApiRestriction, i: I18n = EN): { time: string; title: string; tone?: "warn" | "ok" }[] {
  const { t, display } = i;
  const items: { time: string; title: string; tone?: "warn" | "ok" }[] = (r.events ?? []).map((e) => {
    return {
      time: showTime(e.occurredAt, display), title: textOf(eventTitle, e.action, t) ?? e.action,
      ...eventTone(e.action),
    };
  });
  if (r.state !== "released" && r.state !== "cancelled") items.push({ time: "—", title: t("Released (only when every unit confirms)"), tone: "ok" });
  return items;
}

// ---- inquiries and message previews ----

export interface PreviewParams { targetName: string; at: string; status: string; amountMinor: number | null; currency: string | null; method: string | null }

export interface InquiryLine { id: string; subject: string; at: string; message: string; state: ApiInquiry["state"]; reply: string | null }
export const inquiryLines = (is: ApiInquiry[], i: I18n = EN): InquiryLine[] => {
  return [...is].toSorted((a, b) => b.createdAt.localeCompare(a.createdAt)).map((q) => {
    return {
      id: q.id, subject: i.t(q.subjectType === "payment" ? "Payment" : "Cooling restriction"), at: showTime(q.createdAt, i.display), message: q.message, state: q.state, reply: q.reply,
    };
  });
};
const PREVIEW_BODY: Record<InvoiceStatus, string> = {
  Paid: "Hello, {number} ({amount}) is paid — thank you.",
  Processing: "Hello, your payment for {number} ({amount}) is being processed. It is not paid yet.",
  Overdue: "Hello, {number} ({amount}) was due on {date}. Pay with a demo card or view payment instructions in the app.",
  Unpaid: "Hello, {number} ({amount}) is due on {date}. Pay with a demo card or view payment instructions in the app.",
};
/** The payment message of a channel for this invoice (notifications.preview checks the channel; the text uses the
invoice itself, whose due date and amount the preview parameters do not carry), in the recipient's language. */
export function previewText(channel: string, inv: { number: string; amount: string; dueDate: string; status: InvoiceStatus }, t: T = en): { subject: string | null; body: string } {
  const p = { number: inv.number, amount: inv.amount, date: inv.dueDate };
  const subjectText = inv.status === "Paid" ? "Payment received — {number}" : "Payment reminder — {number}";
  const subject = channel === "email" ? t(subjectText, p) : null;
  return { subject, body: t(PREVIEW_BODY[inv.status], p) };
}

// ---- the overview's notice (DEC-90, IR610) ----

/** What a restriction in force does to the customer's units. */
function restrictedText(policy: ApiRestriction["policy"], n: number, t: T): string {
  if (policy.kind === "power_off") return t(n === 1 ? "{n} unit is switched off until it is paid" : "{n} units are switched off until it is paid", { n });
  return t(n === 1 ? "Cooling is limited to {value} °C or warmer on {n} unit until it is paid" : "Cooling is limited to {value} °C or warmer on {n} units until it is paid", { n, value: policy.minimumCoolingSetpoint });
}
/** One line of the overview's notice: what is due or restricted, and where to act on it. */
export interface DueLine { text: string; href: string; link: string }
/** The customer overview's notice while something is due or restricted (DEC-90, IR610): the overdue invoice with Pay
now — or how many are overdue and their total, with View invoices — and the restriction they caused, on how many
units, with View restriction details: the restriction's own detail, as the unit page links it (IR583). Nothing when
nothing is overdue. */
export function dueNotice(overdue: Pick<ApiInvoice, "id" | "number" | "amountMinor" | "currency">[], restriction: { r: Pick<ApiRestriction, "id" | "state" | "policy" | "unitIds" | "executeAfter">; invoiceId: string } | null, i: I18n = EN): DueLine[] | null {
  const [first] = overdue;
  if (!first) return null;
  const { t, display } = i;
  const lines: DueLine[] = [overdue.length === 1
    ? { text: t("Invoice {number} is overdue — {amount}", { number: first.number, amount: money(first.amountMinor, first.currency) }), href: `/customer/payments/${first.id}`, link: t("Pay now") }
    : { text: t("{n} invoices are overdue — {amount} in total", { n: overdue.length, amount: money(overdue.reduce((sum, x) => sum + x.amountMinor, 0), first.currency) }), href: "/customer/payments?status=overdue", link: t("View invoices") }];
  const r = restriction?.r;
  if (r && live.has(r.state)) {
    const n = r.unitIds.length;
    const text = r.state === "scheduled"
      ? t("A restriction starts {time} unless it is paid — {policy}", { time: showTime(r.executeAfter, display), policy: policyText(r.policy, t) })
      : restrictedText(r.policy, n, t);
    lines.push({ text, href: `/customer/payments/${restriction.invoiceId}?tab=restriction&restrictionId=${r.id}`, link: t("View restriction details") });
  }
  return lines;
}

export {billingMonth, businessDay, KL} from "@ac/web/lib/billing";