"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Btn, Card, DataTable, Field, Input, ListRow, Modal, Page, Select, Steps, SummaryList, useToast } from "@ac/web/components/ui";

type R = { id: string; meta: string; policy: string; st: "Scheduled" | "Requested" | "Applied" | "Release requested" | "Released"; prog: string; tone: "primary" | "warn" | "ok" | "unknown" };
const seed: R[] = [
  { id: "restriction-limited-a", meta: "contract-rto-a · customer-a", policy: "Temperature limit ≥ 24 °C · 1 unit", st: "Applied", prog: "1/1 applied", tone: "warn" },
  { id: "restriction-rto-b", meta: "contract-rto-e · customer-e", policy: "Power off · 1 unit", st: "Scheduled", prog: "executes after 09-25 09:00", tone: "primary" },
  { id: "restriction-rto-c", meta: "contract-rto-c · customer-c", policy: "Power off · 2 units", st: "Released", prog: "released 09-08", tone: "ok" },
  { id: "restriction-x06", meta: "contract-rto-x06 · customer-a", policy: "Temperature limit ≥ 25 °C · 2 units", st: "Release requested", prog: "1/2 released · 1 offline", tone: "unknown" },
];
const flow = ["Scheduled", "Requested", "Applied", "Release requested", "Released"];

export default function Restrictions() {
  const toast = useToast();
  const [list, setList] = useState(seed);
  const [sel, setSel] = useState(seed[0]);
  const [modal, setModal] = useState(false);
  const [f, setF] = useState({ contract: "", policy: "Temperature limit", exec: "", notice: "" });
  const [tried, setTried] = useState(false);
  const setSt = (st: R["st"]) => { setList((l) => l.map((x) => (x.id === sel.id ? { ...x, st } : x))); setSel({ ...sel, st }); };
  const cur = flow.indexOf(sel.st);
  const [openedAt] = useState(() => Date.now()); // reference time for the 24 h rule (render stays pure)
  const execErr = tried && (!f.exec ? "Execute-after is required" : (new Date(f.exec).getTime() - openedAt) < 24 * 3600e3 ? "Execute-after must be at least 24 h after the notice" : undefined);
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex flex-wrap gap-2">{flow.map((s) => <span key={s} className="rounded-full bg-surface2 px-2.5 py-1 text-xs"><span className="text-muted">{s}</span> <b>{list.filter((x) => x.st === s).length}</b></span>)}</div><Btn size="sm" variant="primary" onClick={() => setModal(true)}>+ Schedule restriction</Btn></div>
      <div className="split-rev">
        <Card title="Restrictions" sub="restriction.read · id ↑" className="self-start"><div className="flex flex-col gap-2">{list.map((x) => <ListRow key={x.id} selected={sel.id === x.id} onClick={() => setSel(x)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="truncate text-[13px]">{x.id}</b><Badge tone={x.tone}>{x.st}</Badge></div><div className="text-[11px] text-muted">{x.meta}</div><div className="text-[11px] text-muted">{x.policy} · {x.prog}</div></div></ListRow>)}</div></Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={sel.id} sub={`${sel.meta.split(" · ")[0]} v1 · customer-a · rules demo-v1 · version 3`} action={<Link className="text-xs font-semibold text-primary" href={`/admin/restrictions/${sel.id}`}>Exception / override →</Link>}>
            <Steps steps={flow} current={cur} />
            <div className="mt-4"><SummaryList cols={2} items={[["Policy", sel.policy.split(" · ")[0] + (sel.policy.includes("24") ? " — cooling setpoint ≥ 24 °C" : "")], ["Notice sent", "2026-09-12 08:00 · 1 client notified"], ["Executed after", "2026-09-13 08:00 · notice ≥ 24 h"], ["Grace / exception", "None"], ["Reason shown to the customer", "“Demo: invoice overdue since 2026-09-10”"]]} /></div>
            <div className="mt-3 flex flex-wrap gap-2">{sel.st === "Scheduled" && <Btn variant="primary" onClick={() => { setSt("Applied"); toast("Executed — restriction applied"); }}>Execute (confirm at executeAfter)</Btn>}{sel.st === "Release requested" && <><Btn onClick={() => toast("Retry sent to offline unit", "warn")}>Retry</Btn><Btn onClick={() => { setSt("Released"); toast("Reconciled — released"); }}>Reconcile</Btn></>}{sel.st === "Applied" && <Btn onClick={() => { setSt("Release requested"); toast("Release requested"); }}>Request release…</Btn>}</div>
          </Card>
          <Card title="Cause invoices" action={<Link className="text-xs font-semibold text-primary" href="/admin/billing">Open in Billing →</Link>}><p className="text-[13px]"><b>invoice-overdue-a</b> · 120.00 MYR <span className="text-muted">Aug 2026 · due 09-10</span></p><p className="mt-1 text-xs text-muted">1 of 1 cause invoices unpaid. When every cause invoice is paid, this restriction moves to release_requested automatically.</p></Card>
          <Card title="Units"><DataTable rows={[{ u: "unit-limited · Lobby AC", a: "—", r: sel.st === "Released" ? "released" : "—", o: "09-13 08:00:20 · setpoint ≥ 24 °C" }, ...(sel.id === "restriction-x06" ? [{ u: "unit-offline-rto · Study AC", a: "applied", r: "waiting (offline)", o: "no response" }] : [])]} rowKey={(r) => r.u} cols={[{ key: "u", label: "Unit", render: (r) => <b>{r.u}</b> }, { key: "a", label: "Apply", render: (r) => r.a }, { key: "r", label: "Release", render: (r) => r.r }, { key: "o", label: "Observed", render: (r) => r.o, hideBelow: "sm" }]} />
            <p className="mt-2 text-[11px] text-muted">Retry and Reconcile become available for units that are pending, failed or waiting for reconciliation. Release is only possible after payment, a grace period or exception, or an override. The cause invoice is still unpaid.</p></Card>
        </div>
      </div>
      <Modal open={modal} onClose={() => { setModal(false); setTried(false); }} title="Schedule restriction" footer={<><Btn onClick={() => setModal(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!f.contract || !f.exec || execErr) return; setList((l) => [...l, { id: "restriction-new", meta: `${f.contract} · customer-a`, policy: f.policy + " · 1 unit", st: "Scheduled", prog: "scheduled", tone: "primary" }]); toast("Restriction scheduled; notice sent"); setModal(false); setTried(false); }}>Schedule</Btn></>}>
        <Field label="Contract" error={tried && !f.contract ? "No client can view every unit (IR05) — pick an eligible contract" : undefined}><Select value={f.contract} onChange={(e) => setF({ ...f, contract: e.target.value })}><option value="">Select eligible contract…</option><option>contract-rto-a</option><option>contract-rto-b</option></Select></Field>
        <Field label="Policy"><Select value={f.policy} onChange={(e) => setF({ ...f, policy: e.target.value })}><option>Temperature limit</option><option>Power off</option></Select></Field>
        <Field label="Execute after (≥ 24 h from notice)" error={execErr}><Input type="datetime-local" value={f.exec} onChange={(e) => setF({ ...f, exec: e.target.value })} /></Field>
      </Modal>
    </Page>
  );
}
