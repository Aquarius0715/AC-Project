"use client";

import Link from "next/link";
import { useState } from "react";
import { BarChart, Banner, Card, ConnBadge, Page, SummaryList } from "@/components/ui";

/** Phase 1A demo view of the contractor unit page (fixture values). */
export function MockPartnerUnit({ id }: { id: string }) {
  const [ended, setEnded] = useState(false);
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2"><div><h1 className="text-lg font-bold">Rooftop unit · {id}</h1><p className="text-xs text-muted">Diagnosis-scoped view for job-p02 — no billing, payments, or other contracts.</p></div><button className="text-[11px] text-muted underline" onClick={() => setEnded((e) => !e)}>{ended ? "show live" : "preview: delegation ended"}</button></div>
      {ended && <Banner tone="crit">Delegation ended while open (FORBIDDEN) — live values cleared; only a history snapshot remains.</Banner>}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Unit register"><SummaryList items={[["Location", "customer-b · Tower A rooftop"], ["Model", "cap-split-std v2"], ["Maintenance scope", "General + refrigerant"], ["Connection", <ConnBadge key="c" s="online" />], ["Last seen", "2026-09-14 09:35"]]} /><p className="mt-2 text-[11px] text-muted">Read-only — no remote-control actions available to contractors.</p></Card>
          <Card title="Alert evidence">{[["Vibration anomaly detected", "Suspected — evidence: amplitude 2.1x baseline · 2026-09-13 22:40"], ["Refrigerant pressure low", "Inspection record — logged by tech-external-a · 2026-09-14 08:10"]].map(([a, b]) => <div key={a} className="border-t border-line py-2 first:border-0"><b className="text-[13px]">{a}</b><div className="text-xs text-muted">{b}</div></div>)}</Card>
          {!ended && <Card title="Readings (diagnosis only)" sub="observed 2026-09-14 09:35 · live"><SummaryList items={[["Supply air temperature", "14.2 °C · good"], ["Refrigerant pressure (low side)", "3.1 bar · good"], ["Vibration amplitude", "2.1× baseline · suspect"], ["Power state", "On · cooling"]]} /></Card>}
          {!ended && <Card title="Readings — last 24 h" sub="09-13 09:35 → 09-14 09:35"><div className="grid-fluid" style={{ ["--min"as string]: "260px" }}><div><div className="mb-1 text-xs font-semibold">Vibration amplitude (× baseline) · now 2.1</div><BarChart labels={["a", "b", "c", "d", "e", "f"]} series={[[1, 1.1, 1, 1.4, 2.1, 2.1]]} colors={["#b45309"]} height={110} /></div><div><div className="mb-1 text-xs font-semibold">Refrigerant pressure, low side (bar) · now 3.1</div><BarChart labels={["a", "b", "c", "d", "e", "f"]} series={[[3.4, 3.3, 3.3, 3.2, 3.1, 3.1]]} height={110} /></div></div><p className="mt-2 text-[11px] text-muted">Diagnosis only — read-only values, no control. Highlighted bars cross the alert threshold.</p></Card>}
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Job context" action={<Link className="text-xs font-semibold text-primary" href="/partner/jobs/job-p09">Job →</Link>}><SummaryList items={[["Job", "job-p02 · Reactive"], ["Technician", "Not assigned yet"], ["Access window", "09-14 01:00 → 09-22 00:00"]]} /><p className="mt-2 text-xs text-muted">Access ends in 14 h. After that, live values are cleared and only a history snapshot remains.</p></Card>
          <Card title="Our past work on this unit">{[["job-c01 · Preventive maintenance", "2026-06-12 · tech-external-a"], ["job-b88 · Filter replacement", "2026-03-04 · tech-external-a2"]].map(([a, b]) => <div key={a} className="border-t border-line py-2 text-[13px] first:border-0"><b>{a}</b><div className="text-xs text-muted">{b}</div></div>)}<p className="mt-1 text-[11px] text-muted">Only contractor-a’s own jobs are listed — other contractors’ work and customer billing are hidden.</p></Card>
        </div>
      </div>
    </Page>
  );
}
