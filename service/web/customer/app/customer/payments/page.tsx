// /customer/payments (FR-C10, SCR-C10): in API mode a Server Component reads the customer's own contracts.list, the
// selected contract's invoices (invoices.list with the URL status filter), the unit names (units.list, properties.list,
// spaces.list) and, for a contract with an active restriction, the notice (restrictions.forInvoice of one of its
// invoices). URL keys: contractId, status. Read-only; payments start on the invoice page. Texts in the user's display
// language; due dates and contract periods are Kuala Lumpur business days (IR267). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiInvoice } from "@ac/web/lib/billing";
import type { ApiRestriction } from "@ac/web/lib/restrictions";
import {
  clientInvoiceRows, contractCards, KL, restrictionNotice, statusFilters, statusQuery, unitPlaces, type ClientContract, type RestrictionNotice,
} from "@ac/web/lib/clientBilling";
import { PaymentsDemo } from "./_components/payments-demo";
import { PaymentsView } from "./_components/payments-view";

type Unit = { id: string; displayName: string; propertyId: string; spaceId: string | null };

export default async function CustomerPaymentsPage({ searchParams }: PageProps<"/customer/payments">) {
  await connection();
  if (!apiMode()) return <PaymentsDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [now, contracts, units, properties, spaces, display] = await Promise.all([
    coreNow(), coreAll<ClientContract>("contracts.list"), coreAll<Unit>("units.list"), coreAll<{ id: string; name: string }>("properties.list"),
    coreAll<{ id: string; name: string; parentSpaceId: string | null }>("spaces.list"), coreDisplay(),
  ]);
  const i18n = i18nOf(display);
  const place = unitPlaces(units, properties, spaces);
  const cards = contractCards(contracts, (id) => place.get(id), now, i18n);
  const sel = cards.find((c) => c.id === one("contractId")) ?? cards[0] ?? null;
  const status = statusFilters.find((s) => s === one("status")) ?? "all";
  const [shown, all] = sel
    ? await Promise.all([
      coreAll<ApiInvoice>("invoices.list", { filters: { contractId: sel.id, ...statusQuery(status) } }),
      coreAll<ApiInvoice>("invoices.list", { filters: { contractId: sel.id } }),
    ])
    : [[], []];
  // the active restriction of the contract, found through one of its invoices (the customer has no restriction list)
  let notice: (RestrictionNotice & { invoiceId: string }) | null = null;
  if (sel && sel.restrictionIds.length) {
    for (const i of [...all].sort((a, b) => (a.status === "paid" ? 1 : 0) - (b.status === "paid" ? 1 : 0)).slice(0, 3)) {
      const page = await coreOp<{ items: ApiRestriction[] }>("restrictions.forInvoice", { invoiceId: i.id, query: { limit: 10 } });
      const r = page.items.find((x) => sel.restrictionIds.includes(x.id));
      if (r) {
        notice = { ...restrictionNotice(r, (id) => place.get(id), i18n.t), invoiceId: i.id };
        break;
      }
    }
  }
  return <PaymentsView live={{ cards, selected: sel?.id ?? null, status, invoices: clientInvoiceRows(shown, now, i18n), total: all.length, notice, otherZone: display.timeZone !== KL }} />;
}
