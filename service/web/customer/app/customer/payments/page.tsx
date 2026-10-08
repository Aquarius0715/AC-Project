"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Card, LinkBtn, ListRow, Page, SummaryList, TextLink } from "@ac/web/components/ui";

const contracts = [
  { id: "contract-rto-a", name: "RTO Plan", sub: "contract-rto-a · Jan–Dec 2026", period: "Jan 1 – Dec 31, 2026", plan: "Rent-to-own (RTO)", units: [["Bedroom AC", "Home A › 1F › Bedroom"], ["Meeting room AC", "Office A › Meeting room"], ["Lobby AC", "Office A · not in a room"]], invoices: [["invoice-overdue-a", "Due Sep 10, 2026", "120.00 MYR", "unpaid"], ["invoice-0198", "Due Aug 10 · paid Aug 8", "80.00 MYR", "paid"], ["invoice-0175", "Due Jul 10 · paid Jul 9", "80.00 MYR", "paid"]] },
  { id: "contract-gm-a", name: "General maintenance", sub: "contract-gm-a · Mar 2026 – Feb 2027", period: "Mar 1, 2026 – Feb 28, 2027", plan: "General maintenance", units: [["Study AC", "Home A › 2F › Study"]], invoices: [["invoice-sep-a", "Due Sep 30", "50.00 MYR", "unpaid"]] },
  { id: "contract-es-a", name: "Energy service", sub: "contract-es-a · 2025", period: "2025", plan: "Energy service", units: [["Living room AC", "Home A › 1F"]], invoices: [] },
];

export default function Payments() {
  const [sel, setSel] = useState(contracts[0]);
  return (
    <Page>
      <div className="split-rev">
        <Card title={`CONTRACTS (${contracts.length})`} className="self-start">
          <div className="flex flex-col gap-2">{contracts.map((c) => <ListRow key={c.id} selected={sel.id === c.id} onClick={() => setSel(c)}><div className="min-w-0"><b>{c.name}</b><div className="truncate text-xs text-muted">{c.sub}</div></div></ListRow>)}</div>
          <p className="mt-3 text-[11px] text-muted">Contract expiry does not stop monitoring or control.</p>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={sel.name} sub={`${sel.id} · plan type ${sel.plan.split(" ")[0]}`}>
            <SummaryList items={[["Period", sel.period], ["Plan", sel.plan]]} />
            <h3 className="mt-4 mb-2 text-[11px] font-bold uppercase tracking-wide text-muted">Contract scope — equipment this contract relates to</h3>
            <div className="flex flex-col divide-y divide-line">{sel.units.map(([n, l]) => <div key={n} className="flex flex-wrap items-center justify-between gap-2 py-2 text-[13px]"><span><b>{n}</b><span className="block text-xs text-muted">{l}</span></span><TextLink href="/customer/properties">View unit ›</TextLink></div>)}</div>
          </Card>
          <Card title="Invoices for this contract" sub={`${sel.invoices.length} invoices`}>
            <div className="flex flex-col gap-2">
              {sel.invoices.map(([id, due, amt, st]) => (
                <div key={id} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                  <div><b>{id}</b><div className="text-xs text-muted">{due}</div></div>
                  <div className="flex items-center gap-3"><b>{amt}</b>{st === "unpaid" ? <><Badge tone="crit">Unpaid</Badge><LinkBtn size="sm" variant="primary" href={`/customer/payments/${id}`}>Pay</LinkBtn></> : <><Badge tone="ok" icon="✓">Paid</Badge><Link href={`/customer/payments/${id}`} className="text-muted">›</Link></>}</div>
                </div>
              ))}
              {sel.invoices.length === 0 && <p className="text-xs text-muted">No invoices.</p>}
            </div>
            <p className="mt-2 text-[11px] text-muted">Paid invoices have no payment button. “Processing” is not paid.</p>
          </Card>
          {sel.id === "contract-rto-a" && <Banner tone="warn" action={<LinkBtn size="sm" href="/customer/payments/invoice-overdue-a?tab=restriction">Details</LinkBtn>}><b>Cooling restriction active</b><div className="text-xs">1 unit in scope · Lobby AC (Office A) — minimum cooling setpoint 24 °C applied. Related to payment, but separate from it: paying does not restore cooling immediately.</div></Banner>}
        </div>
      </div>
    </Page>
  );
}
