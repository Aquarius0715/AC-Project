// /customer/payments (FR-C10, SCR-C10): in API mode a Server Component reads the customer's own contracts.list, the
// selected contract's invoices (invoices.list with the URL status filter), the unit names (units.list, properties.list,
// spaces.list) and, for a contract with an active restriction, the notice (restrictions.forInvoice of one of its
// invoices). URL keys: contractId, status. Read-only; payments start on the invoice page. Texts in the user's display
// language; due dates and contract periods are Kuala Lumpur business days (IR267). The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow } from "@ac/web/lib/dal";
import { restrictionFor } from "@ac/web/lib/restrictionLookup";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiInvoice } from "@ac/web/lib/billing";
import {
  clientInvoiceRows, contractCards, KL, restrictionNotice, statusFilters, statusQuery, unitPlaces, type ClientContract, type RestrictionNotice,
} from "@ac/web/lib/clientBilling";
import { PaymentsDemo } from "./_components/payments-demo";
import { PaymentsView } from "./_components/payments-view";
import { queryReader } from "@ac/web/lib/searchParams";


export default async function CustomerPaymentsPage({ searchParams }: Readonly<PageProps<"/customer/payments">>) {
  interface Unit { id: string; displayName: string; propertyId: string; spaceId: string | null }
  await connection();
  if (!apiMode()) return <PaymentsDemo />;
  const sp = await searchParams;
  const one = queryReader(sp);
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
  if (sel && sel.restrictionIds.length > 0) {
    const found = await restrictionFor(all, (x) => sel.restrictionIds.includes(x.id)); // every invoice, causes first (IR715)
    if (found) notice = { ...restrictionNotice(found.r, (id) => place.get(id), i18n.t), invoiceId: found.invoice.id };
  }
  return <PaymentsView live={{ cards, status, notice, selected: sel?.id ?? null, invoices: clientInvoiceRows(shown, now, i18n), total: all.length, otherZone: display.timeZone !== KL }} />;
}
