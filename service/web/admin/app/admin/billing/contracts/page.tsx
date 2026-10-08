"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Field, Input, ListRow, Modal, Page, Select, Tabs, useToast } from "@ac/web/components/ui";

const seed = [
  { id: "contract-general-a", cust: "customer-a", units: "1 unit · unit-non-rto", price: 50, start: "2026-01-01", end: "2027-01-01", plan: "General" as const, v: 1, blocked: false },
  { id: "contract-rto-a", cust: "customer-a", units: "3 units", price: 120, start: "2026-01-01", end: "2027-01-01", plan: "RTO" as const, v: 1, blocked: true },
  { id: "contract-rto-b", cust: "customer-b", units: "1 unit · unit-other-customer", price: 85, start: "2026-03-01", end: "2027-03-01", plan: "RTO" as const, v: 1, blocked: false },
];
type Plan = "All" | "RTO" | "General" | "Energy" | "Environment";

export default function Contracts() {
  const toast = useToast();
  const [list, setList] = useState(seed);
  const [f, setF] = useState<Plan>("All");
  const [sel, setSel] = useState(seed[1]);
  const [price, setPrice] = useState("120.00");
  const [end, setEnd] = useState(sel.end);
  const [elig, setElig] = useState(true);
  const [modal, setModal] = useState(false);
  const [nc, setNc] = useState({ cust: "", price: "", start: "", end: "" });
  const [tried, setTried] = useState(false);
  const shown = list.filter((c) => f === "All" || c.plan === f);
  const pick = (c: (typeof seed)[number]) => { setSel(c); setPrice(c.price.toFixed(2)); setEnd(c.end); };
  const priceErr = +price < 0 ? "Price must be ≥ 0" : undefined;
  const endErr = end <= sel.start ? "End must be after start" : undefined;
  return (
    <Page>
      <Tabs value={f} onChange={setF} tabs={(["All", "RTO", "General", "Energy", "Environment"] as Plan[]).map((k) => ({ id: k, label: k, count: k === "All" ? list.length : list.filter((c) => c.plan === k).length }))} />
      <div className="split-rev">
        <Card title="Contracts" sub="id ↑" action={<Btn size="sm" variant="primary" onClick={() => setModal(true)}>+ New</Btn>} className="self-start">
          <div className="flex flex-col gap-2">{shown.map((c) => <ListRow key={c.id} selected={sel.id === c.id} onClick={() => pick(c)}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{c.id}</b><b className="text-[13px]">{c.price.toFixed(2)} MYR</b></div><div className="text-[11px] text-muted">{c.cust} · {c.units}</div><div className="text-[11px] text-muted">{c.start} → {c.end}</div></div></ListRow>)}</div>
          <p className="mt-3 text-[11px] text-muted">Units without a contract can still be monitored and maintained.</p>
        </Card>
        <Card title={sel.id} sub={`Version ${sel.v} · Editing → v${sel.v + 1}`} action={<Badge tone="primary">{sel.plan}</Badge>}>
          <div className="flex flex-col gap-5">
            <p className="text-xs text-muted">{sel.cust} (org-{sel.cust}) {sel.blocked ? "· 1 active restriction: restriction-limited-a (applied)" : ""}</p>
            <section><h3 className="mb-1 text-[13px] font-bold">Customer & units</h3><p className="mb-2 text-xs text-muted">Only this customer’s active units can be included.</p><div className="flex flex-col gap-1.5">{[["Bedroom AC · unit-online-rto", "Home A › 1F › Bedroom", true], ["unit-offline-rto", "Home A › 2F › Study", true], ["unit-limited", "Home A › 1F › Living room", true], ["unit-non-rto", "Home B · also on contract-general-a", false]].map(([a, b, c]) => <Check key={a as string} checked={c as boolean} disabled={sel.blocked} label={<span>{a as string} <span className="text-xs text-muted">{b as string}</span></span>} />)}</div></section>
            <section className="grid-fluid" style={{ ["--min"as string]: "150px" }}><Field label="Start"><Input type="date" defaultValue={sel.start} disabled /></Field><Field label="End" error={endErr}><Input type="date" value={end} disabled={sel.blocked} onChange={(e) => setEnd(e.target.value)} /></Field><Field label="Price" error={priceErr}><Input type="number" value={price} disabled={sel.blocked} onChange={(e) => setPrice(e.target.value)} /></Field><Field label="Currency"><Select disabled={sel.blocked}><option>MYR</option></Select></Field></section>
            <section><h3 className="mb-1 text-[13px] font-bold">Restriction eligibility</h3><p className="mb-2 text-xs text-muted">Only RTO contracts can be restriction eligible, and only when set explicitly here.</p><Check label="Allow cooling restrictions for unpaid invoices" checked={elig && sel.plan === "RTO"} disabled={sel.blocked || sel.plan !== "RTO"} onChange={setElig} /><div className="mt-2 max-w-xs"><Field label="Rules version · required when eligible" hint="Demo rules — not approved production rules"><Select disabled={sel.blocked}><option>demo-v1</option></Select></Field></div></section>
            {sel.blocked && <Banner tone="warn" action={<Link className="text-xs font-semibold text-primary" href="/admin/restrictions">Open restriction-limited-a →</Link>}>Save is disabled (SR19): restriction-limited-a is applied on unit-limited. Cancel the notice or release the restriction first — contract changes never cancel or release restrictions.</Banner>}
            <section className="rounded-xl bg-surface2 p-3 text-xs"><b className="text-[13px]">What saving does</b><ul className="mt-1 list-disc pl-5"><li>Creates version {sel.v + 1} — version {sel.v} stays in history</li><li>Invoices already issued keep version {sel.v} and their original amounts</li><li>New invoices will use version {sel.v + 1} ({(+price || 0).toFixed(2)} MYR)</li></ul></section>
            <div className="flex justify-end gap-2">{sel.blocked && <Badge tone="muted">Read-only while a restriction is active</Badge>}<Btn variant="primary" disabled={sel.blocked || !!priceErr || !!endErr} onClick={() => { setList((l) => l.map((c) => (c.id === sel.id ? { ...c, price: +price, v: c.v + 1, end } : c))); toast(`Saved as version ${sel.v + 1}`); }}>Save as v{sel.v + 1}</Btn></div>
          </div>
        </Card>
      </div>
      <Modal open={modal} onClose={() => { setModal(false); setTried(false); }} title="New contract" footer={<><Btn onClick={() => setModal(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setTried(true); if (!nc.cust.trim() || !nc.price.trim() || !nc.start || !nc.end || nc.end <= nc.start) return; toast("Contract created"); setModal(false); setTried(false); }}>Create</Btn></>}>
        <Field label="Customer" error={tried && !nc.cust.trim() ? "Required" : undefined}><Select value={nc.cust} onChange={(e) => setNc({ ...nc, cust: e.target.value })}><option value="">Select…</option><option>customer-a</option><option>customer-b</option></Select></Field>
        <div className="grid-fluid" style={{ ["--min"as string]: "140px" }}><Field label="Start" error={tried && !nc.start ? "Required" : undefined}><Input type="date" value={nc.start} onChange={(e) => setNc({ ...nc, start: e.target.value })} /></Field><Field label="End" error={tried && nc.end && nc.end <= nc.start ? "Must be after start" : tried && !nc.end ? "Required" : undefined}><Input type="date" value={nc.end} onChange={(e) => setNc({ ...nc, end: e.target.value })} /></Field><Field label="Price" error={tried && !nc.price.trim() ? "Required" : undefined}><Input type="number" value={nc.price} onChange={(e) => setNc({ ...nc, price: e.target.value })} /></Field></div>
      </Modal>
    </Page>
  );
}
