// /customer/payments/[id] (FR-C11, FR-C12, SCR-C11): in API mode a Server Component reads invoices.get (with its
// payments), the contract and unit names, the invoice's restriction (restrictions.forInvoice; commands.get for each
// unit's latest command), the inquiries about the invoice and the restriction (inquiries.list) and the payment
// reminders the customer received (notifications.list, type payment_reminder). Demo payments, payment instructions,
// message previews and inquiries are Server Actions (actions.ts). URL keys: tab (payment, restriction, inquiry),
// restrictionId, inquiryId. Texts in the user's display language; the due date and billing month are Kuala Lumpur
// business days, every event time is in the user's display time zone (IR267). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { amount } from "@ac/web/lib/energy";
import { i18nOf, showDate, showTime } from "@ac/web/lib/i18n";
import { invoiceStatus, type ApiInquiry, type ApiInvoiceDetail } from "@ac/web/lib/billing";
import type { ApiCommand, ApiRestriction } from "@ac/web/lib/restrictions";
import {
  billingMonth, businessDay, contractCards, inquiryLines, KL, noticeTimeline, noticeUnits, paymentLines, paymentPhase, restrictionNotice, unitPlaces,
  type ClientContract, type ClientPayment,
} from "@ac/web/lib/clientBilling";
import { InvoiceDemo } from "./_components/invoice-demo";
import { InvoiceView, type InvoiceLive } from "./_components/invoice-view";

type Unit = { id: string; displayName: string; propertyId: string; spaceId: string | null };
type Reminder = { id: string; channel: string; occurredAt: string; target: { kind: string; id: string } };

export default async function CustomerInvoicePage({ params, searchParams }: PageProps<"/customer/payments/[id]">) {
  const { id } = await params;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  await connection();
  if (!apiMode()) return <InvoiceDemo id={id} tab={one("tab")} />;
  let inv: ApiInvoiceDetail & { createdAt: string; paymentRefs: ClientPayment[] };
  try {
    inv = await coreOp("invoices.get", { id });
  } catch (e) {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.fieldErrors.id)) notFound(); // another customer's invoice reads as absent (D01)
    throw e;
  }
  const [now, contracts, units, properties, spaces, restrictions, inquiries, reminders, display] = await Promise.all([
    coreNow(), coreAll<ClientContract>("contracts.list"), coreAll<Unit>("units.list"), coreAll<{ id: string; name: string }>("properties.list"),
    coreAll<{ id: string; name: string; parentSpaceId: string | null }>("spaces.list"),
    coreOp<{ items: ApiRestriction[] }>("restrictions.forInvoice", { invoiceId: id, query: { limit: 20 } }).then((p) => p.items),
    coreAll<ApiInquiry>("inquiries.list", { filters: { invoiceId: id } }),
    coreAll<Reminder>("notifications.list", { filters: { type: "payment_reminder" } }).then((ns) => ns.filter((n) => n.target.kind === "invoice" && n.target.id === id)),
    coreDisplay(),
  ]);
  const i18n = i18nOf(display);
  const { t } = i18n;
  const loc = display.locale;
  const place = unitPlaces(units, properties, spaces);
  const contract = contractCards(contracts.filter((k) => k.id === inv.contractId), (u) => place.get(u), now, i18n)[0] ?? null;
  const { open, last } = paymentPhase(inv.paymentRefs);
  const r = restrictions.find((x) => x.id === one("restrictionId")) ?? restrictions.find((x) => ["scheduled", "requested", "applied", "release_requested"].includes(x.state)) ?? restrictions[0] ?? null;
  const tab = one("tab") === "restriction" && r ? "restriction" : one("tab") === "inquiry" ? "inquiry" : "payment";
  let restriction: InvoiceLive["restriction"] = null;
  if (r) {
    const ids = r.perUnit.flatMap((u) => [u.applyCommandIds.at(-1), u.releaseCommandIds.at(-1)]).filter((x): x is string => !!x);
    const [commands, about] = await Promise.all([
      tab === "restriction" ? Promise.all(ids.map((c) => coreOp<ApiCommand>("commands.get", { id: c }).catch(() => null))) : Promise.resolve([]),
      coreAll<ApiInquiry>("inquiries.list", { filters: { restrictionId: r.id } }),
    ]);
    const byId = new Map(commands.filter((c): c is ApiCommand => !!c).map((c) => [c.id, c]));
    const notice = restrictionNotice(r, (u) => place.get(u), t);
    const causes = (r.causeInvoiceIds ?? []).length;
    restriction = {
      notice, units: noticeUnits(r, (u) => place.get(u), byId, t), timeline: noticeTimeline(r, i18n), inquiries: inquiryLines(about, i18n),
      reason: causes > 1 ? t("{n} unpaid invoices, including {number}", { n: causes, number: inv.number })
        : t("Unpaid invoice {number} ({amount}, due {date})", { number: inv.number, amount: amount(inv.amountMinor, inv.currency), date: businessDay(inv.dueAt, loc) }),
      noticeAt: t("{date} · in-app (demo)", { date: showDate(r.noticeAt, display) }), start: `${showTime(r.executeAfter, display)} — ${notice.policy}`,
      released: r.releaseIntent ? `${showTime(r.releaseIntent.at, display)} — ${t(r.releaseIntent.source === "payment" ? "automatically after payment confirmation" : "by HQ")}` : t("not yet — all cause invoices must be paid"),
    };
  }
  const st = invoiceStatus(inv, now);
  const live: InvoiceLive = {
    tab, otherZone: display.timeZone !== KL,
    invoice: {
      id: inv.id, version: inv.version, number: inv.number, amount: amount(inv.amountMinor, inv.currency),
      due: t("Due {date} · billing period {month}", { date: businessDay(inv.dueAt, loc), month: billingMonth(inv.period.from, loc) }), dueDate: businessDay(inv.dueAt, loc), status: st, payable: st === "Unpaid" || st === "Overdue",
    },
    contract: contract && {
      name: contract.name, id: contract.id,
      scope: `${t(contract.units.length === 1 ? "{n} unit" : "{n} units", { n: contract.units.length })}${contract.units.length ? ` (${contract.units.map((u) => u.name).join(", ")})` : ""}`,
    },
    payments: paymentLines(inv.paymentRefs, i18n), open: open && { id: open.id, version: open.version, status: open.status, method: open.method },
    last: last && { status: last.status, reference: last.paymentReference, method: last.method, id: last.id },
    restriction, inquiries: inquiryLines(inquiries, i18n), inquiryId: one("inquiryId") ?? null,
    reminders: reminders.map((n) => ({ id: n.id, channel: n.channel, at: showDate(n.occurredAt, display) })),
  };
  return <InvoiceView live={live} />;
}
