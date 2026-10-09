"use client";

import Link from "next/link";
import { Badge, Banner, Card, Choice, EmptyState, LinkBtn, ListRow, Page, SummaryList, TextLink } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { stateTone } from "@ac/web/lib/restrictions";
import type { ClientInvoiceRow, ContractCard, RestrictionNotice, StatusFilter } from "@ac/web/lib/clientBilling";

export type PaymentsLive = {
  cards: ContractCard[]; selected: string | null; status: StatusFilter; invoices: ClientInvoiceRow[]; total: number; notice: (RestrictionNotice & { invoiceId: string }) | null;
};
const statusTone = { Overdue: "crit", Unpaid: "warn", Processing: "primary", Paid: "ok" } as const;
const contractTone = { Active: "ok", Upcoming: "primary", Expired: "muted" } as const;

/** Contracts & payments (FR-C10) in API mode: the contract and the invoice status filter live in the URL; the page is
 * read-only — payments start on the invoice page. */
export function PaymentsView({ live }: { live: PaymentsLive }) {
  const nav = useUrlPatch();
  const sel = live.cards.find((c) => c.id === live.selected) ?? null;
  if (!sel) {
    return <Page><EmptyState title="No contract — general maintenance" action={<LinkBtn href="/customer" size="sm">Back to monitoring</LinkBtn>}>Your units are monitored and controlled without a contract; invoices appear here once HQ sets up a contract.</EmptyState></Page>;
  }
  return (
    <Page>
      <div className="split-rev">
        <Card title={`CONTRACTS (${live.cards.length})`} className="self-start">
          <div className="flex flex-col gap-2">
            {live.cards.map((c) => (
              <ListRow key={c.id} selected={c.id === sel.id} onClick={() => nav({ contractId: c.id, status: null })}>
                <div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate">{c.name}</b><Badge tone={contractTone[c.status]}>{c.status}</Badge></div><div className="truncate text-xs text-muted">{c.sub}</div></div>
              </ListRow>
            ))}
          </div>
          <p className="mt-3 text-[11px] text-muted">Contract expiry does not stop monitoring or control.</p>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={sel.name} sub={`${sel.id.slice(0, 8)} · plan type ${sel.planType.toUpperCase()}`} action={<Badge tone={contractTone[sel.status]}>{sel.status}</Badge>}>
            <SummaryList items={[["Period", sel.period], ["Plan", sel.plan]]} />
            <h3 className="mb-2 mt-4 text-[11px] font-bold uppercase tracking-wide text-muted">Contract scope — equipment this contract relates to</h3>
            {sel.units.length === 0 ? <p className="text-xs text-muted">No unit in this contract.</p> : (
              <div className="flex flex-col divide-y divide-line">
                {sel.units.map((u) => <div key={u.id} className="flex flex-wrap items-center justify-between gap-2 py-2 text-[13px]"><span><b>{u.name}</b><span className="block text-xs text-muted">{u.place || "—"}</span></span><TextLink href={`/customer/units/${u.id}`}>View unit ›</TextLink></div>)}
              </div>
            )}
          </Card>
          <Card title="Invoices for this contract" sub={`${live.invoices.length} of ${live.total} invoice${live.total === 1 ? "" : "s"}`}
            action={<Choice value={live.status} onChange={(v: StatusFilter) => nav({ status: v === "all" ? null : v })} options={[{ id: "all", label: "All" }, { id: "unpaid", label: "Unpaid" }, { id: "overdue", label: "Overdue" }, { id: "processing", label: "Processing" }, { id: "paid", label: "Paid" }]} />}>
            <div className="flex flex-col gap-2">
              {live.invoices.map((i) => (
                <div key={i.id} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                  <div><b>{i.number}</b><div className="text-xs text-muted">{i.due}</div></div>
                  <div className="flex items-center gap-3"><b>{i.amount}</b><Badge tone={statusTone[i.status]} icon={i.status === "Overdue" ? "‼" : i.status === "Paid" ? "✓" : undefined}>{i.status}</Badge>
                    {i.payable ? <LinkBtn size="sm" variant="primary" href={`/customer/payments/${i.id}`}>Pay now</LinkBtn> : <Link href={`/customer/payments/${i.id}`} className="text-muted" aria-label={`Open ${i.number}`}>›</Link>}</div>
                </div>
              ))}
              {live.invoices.length === 0 && <p className="text-xs text-muted">{live.total ? "No invoice matches this filter." : "No invoices yet — this is not an overdue payment."}</p>}
            </div>
            <p className="mt-2 text-[11px] text-muted">Paid invoices have no payment button. “Processing” is not paid.</p>
          </Card>
          {live.notice && (
            <Banner tone={live.notice.state === "release_requested" ? "primary" : "warn"} action={<LinkBtn size="sm" href={`/customer/payments/${live.notice.invoiceId}?tab=restriction&restrictionId=${live.notice.id}`}>View restriction details</LinkBtn>}>
              <span className="flex flex-wrap items-center gap-2"><b>Cooling restriction{live.notice.state === "release_requested" ? " — release requested" : " active"}</b><Badge tone={stateTone(live.notice.state)}>{live.notice.label}</Badge></span>
              <div className="text-xs">{live.notice.units} — {live.notice.policy}. Related to payment, but separate from it: paying does not restore cooling immediately.</div>
            </Banner>
          )}
        </div>
      </div>
    </Page>
  );
}
