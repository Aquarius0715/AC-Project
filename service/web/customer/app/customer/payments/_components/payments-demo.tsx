"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Card, LinkButton, ListRow, Page, SummaryList, TextLink } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";
import { demoContracts as contracts } from "../_lib/demo-contracts";

/** The Phase 1A demo of /customer/payments (fixtures; DATA_SOURCE≠API). */
export function PaymentsDemo() {
  const [sel, setSel] = useState(contracts[0]);
  return (
    <Page>
      <div className="split-rev">
        <Card title={`CONTRACTS (${String(contracts.length)})`} className="self-start">
          <div className="flex flex-col gap-2">{contracts.map((c) => <ListRow key={c.id} selected={sel.id === c.id} onClick={() => { setSel(c); }}><div className="min-w-0"><b>{c.name}</b><div className="truncate text-xs text-muted-foreground">{c.sub}</div></div></ListRow>)}</div>
          <p className="mt-3 text-[11px] text-muted-foreground">Contract expiry does not stop monitoring or control.</p>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={sel.name} sub={`${sel.id} · plan type ${sel.plan.split(" ", 1)[0] ?? sel.plan}`}>
            <SummaryList items={[["Period", sel.period], ["Plan", sel.plan]]} />
            <h3 className="mt-4 mb-2 text-[11px] font-bold uppercase tracking-wide text-muted-foreground">Contract scope — equipment this contract relates to</h3>
            <div className="flex flex-col divide-y divide-line">{sel.units.map(([n, l]) => <div key={n} className="flex flex-wrap items-center justify-between gap-2 py-2 text-[13px]"><span><b>{n}</b><span className="block text-xs text-muted-foreground">{l}</span></span><TextLink href="/customer/properties">View unit <Icon name="chevronRight" /></TextLink></div>)}</div>
          </Card>
          <Card title="Invoices for this contract" sub={`${String(sel.invoices.length)} invoices`}>
            <div className="flex flex-col gap-2">
              {sel.invoices.map(({ id, due, paid, amount }) => (
                <div key={id} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3">
                  <div><b>{id}</b><div className="text-xs text-muted-foreground">Due {due}{paid ? ` · paid ${paid}` : ""}</div></div>
                  <div className="flex items-center gap-3"><b>{amount} MYR</b>{paid === null ? <><Badge tone="crit">Unpaid</Badge><LinkButton size="sm" variant="primary" href={`/customer/payments/${id}`}>Pay</LinkButton></> : <><Badge tone="ok" icon="check">Paid</Badge><Link href={`/customer/payments/${id}`} className="text-muted-foreground" aria-label={`Open invoice ${id}`}><Icon name="chevronRight" /></Link></>}</div>
                </div>
              ))}
              {sel.invoices.length === 0 && <p className="text-xs text-muted-foreground">No invoices.</p>}
            </div>
            <p className="mt-2 text-[11px] text-muted-foreground">Paid invoices have no payment button. “Processing” is not paid.</p>
          </Card>
          {sel.invoices.some((i) => i.restricted) && <Banner tone="warn" action={<LinkButton size="sm" href="/customer/payments/invoice-overdue-a?tab=restriction">Details</LinkButton>}><b>Cooling restriction active</b><div className="text-xs">1 unit in scope · Lobby AC (Office A) — minimum cooling setpoint 24 °C applied. Related to payment, but separate from it: paying does not restore cooling immediately.</div></Banner>}
        </div>
      </div>
    </Page>
  );
}
