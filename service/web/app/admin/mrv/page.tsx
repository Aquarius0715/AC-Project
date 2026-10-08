"use client";

import { useState } from "react";
import { Banner, Btn, Card, DataTable, Field, Input, ListRow, Modal, Page, Select, SummaryList, Tabs, Textarea, useToast, cx } from "@/components/ui";
import { useUrlTab } from "@/lib/useUrlTab";

const reports = [
  { id: "mrv-a14-a", meta: "org-customer-a · unit-online-rto · 09-14 08:00–09:00", sub: "40.0 kgCO₂e · v1", ok: true, versions: 1 },
  { id: "mrv-aug-a", meta: "org-customer-a · 3 units · Aug 2026", sub: "Calculation incomplete · v2", ok: false, versions: 2 },
];

export default function MRV() {
  const toast = useToast();
  const [tab, setTab] = useUrlTab<"reports" | "factors">({ reports: "reports", factors: "factors" }, "reports");
  const [sel, setSel] = useState(reports[0]);
  const [ver, setVer] = useState(1);
  const [cmp, setCmp] = useState(false);
  const [review, setReview] = useState("Inputs match the pilot agreement; demo only.");
  const [reviewed, setReviewed] = useState(true);
  const [nw, setNw] = useState(false);
  const [f, setF] = useState({ cust: "org-customer-a", start: "2026-09-14T08:00", end: "2026-09-14T09:00", unit: "unit-online-rto" });
  const [prev, setPrev] = useState(false);
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "reports", label: "Reports", count: 2 }, { id: "factors", label: "Emission factors", count: 1 }]} />
      {tab === "reports" ? (
        <div className="split-rev">
          <Card title="Scope 2 reports" action={<Btn size="sm" variant="primary" onClick={() => setNw(true)}>+ New</Btn>} className="self-start"><div className="flex flex-col gap-2">{reports.map((r) => <ListRow key={r.id} selected={sel.id === r.id} onClick={() => { setSel(r); setVer(r.versions); }}><div className="min-w-0"><b className="text-[13px]">{r.id}</b><div className="text-[11px] text-muted">{r.meta}</div><div className={cx("text-[11px]", r.ok ? "text-muted" : "text-warn")}>{r.sub}</div></div></ListRow>)}</div><p className="mt-3 text-[11px] text-muted">Every report is a demo — “Demo — unverified”. Demo review is not external certification.</p></Card>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title="Scope 2 electricity — demo summary" sub={`${sel.id} · org-customer-a`} action={<div className="flex items-center gap-2"><Select aria-label="Version" className="w-auto" value={ver} onChange={(e) => setVer(+e.target.value)}>{Array.from({ length: sel.versions }, (_, i) => <option key={i} value={i + 1}>Version {i + 1}{i + 1 === sel.versions ? " (latest)" : ""}</option>)}</Select>{sel.versions > 1 && <Btn size="sm" onClick={() => setCmp(true)}>Compare versions →</Btn>}</div>}>
              {!sel.ok ? <Banner tone="warn">Calculation incomplete (draft) — missing readings block the totals. Nothing is shown as zero.</Banner> : (
                <div className="grid-fluid" style={{ ["--min"as string]: "170px" }}>{[["Electricity used", "80.0 kWh", "coverage 100%"], ["Scope 2 emissions", "40.0 kgCO₂e", "80.0 kWh × 0.5 kgCO₂e/kWh"], ["Energy vs baseline", "Reduction 20.0 kWh", "Reduction 20.0% · not adjusted"], ["Emissions vs baseline", "Reduction 10.0 kgCO₂e", "shown separately from energy"]].map(([a, b, c]) => <div key={a} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{a}</div><b>{b}</b><div className="text-[10px] text-muted">{c}</div></div>)}</div>
              )}
            </Card>
            <Card title="Calculation conditions (snapshot)"><SummaryList items={[["Customer", "org-customer-a"], ["Sites / units", "Home A · unit-online-rto"], ["Period", "2026-09-14 08:00–09:00 (Asia/Kuala_Lumpur)"], ["Boundary", "ac_input_electricity — AC input electricity only"], ["Baseline", "baseline-energy-100 v1 · demo_fixed · 100.0 kWh"], ["Emission factor", "factor-demo-2026 v1 · Demo region (fictional) · 2026 · 0.5 kgCO₂e/kWh"], ["Factor source", "1A demo fixed; not a real regional factor"]]} /><p className="mt-2 text-[11px] text-muted">Snapshots are stored with the report. Later changes to the baseline or factor do not change this version.</p></Card>
            <Card title="Evidence"><ul className="list-disc pl-5 text-[13px]"><li>m-online-kwh-0800-0900 · measured</li><li>baseline-energy-100 v1 snapshot</li><li>factor-demo-2026 v1 snapshot</li></ul></Card>
            <Card title="Demo review">{reviewed ? <><p className="text-[13px]">“{review}”</p><p className="text-[11px] text-muted">hq-operator · version {ver} · 2026-09-14 10:05</p><p className="mt-1 text-xs text-muted">This version already has a demo review; sending the same review again returns the existing result. Only the latest version can be reviewed, and old versions are read-only.</p></> : <><Field label="Review comment"><Textarea value={review} onChange={(e) => setReview(e.target.value)} /></Field><div className="mt-2"><Btn variant="primary" disabled={!sel.ok || ver !== sel.versions} onClick={() => { setReviewed(true); toast("Demo review recorded"); }}>Submit demo review</Btn></div></>}</Card>
          </div>
        </div>
      ) : (
        <Card title="Emission factors"><DataTable rows={[{ id: "factor-demo-2026", v: "v1", region: "Demo region (fictional)", year: 2026, f: "0.5 kgCO₂e/kWh" }]} rowKey={(r) => r.id} cols={[{ key: "i", label: "Factor", render: (r) => <b>{r.id}</b> }, { key: "v", label: "Version", render: (r) => r.v }, { key: "r", label: "Region", render: (r) => r.region, hideBelow: "sm" }, { key: "y", label: "Year", render: (r) => r.year, hideBelow: "sm" }, { key: "f", label: "Value", render: (r) => r.f }]} /><p className="mt-2 text-[11px] text-muted">1A demo fixed; not a real regional factor.</p></Card>
      )}
      <Modal open={cmp} onClose={() => setCmp(false)} title="Compare conditions · v1 (demo reviewed) ↔ v2 (draft)" footer={<Btn onClick={() => setCmp(false)}>Close</Btn>}><DataTable rows={[{ k: "Electricity used", a: "80.0 kWh", b: "80.0 kWh" }, { k: "Scope 2 emissions", a: "40.0 kgCO₂e", b: "40.0 kgCO₂e" }, { k: "Baseline", a: "baseline-energy-100 v1", b: "baseline-energy-100 v1" }, { k: "Factor", a: "factor-demo-2026 v1", b: "factor-demo-2026 v2" }]} rowKey={(r) => r.k} cols={[{ key: "k", label: "Field", render: (r) => r.k }, { key: "a", label: "v1", render: (r) => r.a }, { key: "b", label: "v2", render: (r) => <span className={r.a !== r.b ? "font-bold text-warn" : ""}>{r.b}</span> }]} /></Modal>
      <Modal open={nw} onClose={() => { setNw(false); setPrev(false); }} title="New Scope 2 report" wide footer={<><Btn onClick={() => setNw(false)}>Cancel</Btn>{!prev ? <Btn variant="primary" onClick={() => setPrev(true)}>Preview</Btn> : <Btn variant="primary" onClick={() => { toast("Report created (demo — unverified)"); setNw(false); setPrev(false); }}>Create report</Btn>}</>}>
        <div className="grid-fluid" style={{ ["--min"as string]: "200px" }}><Field label="Customer"><Select value={f.cust} onChange={(e) => setF({ ...f, cust: e.target.value })}><option>org-customer-a</option><option>org-customer-b</option></Select></Field><Field label="Unit(s)"><Input value={f.unit} onChange={(e) => setF({ ...f, unit: e.target.value })} /></Field><Field label="Start"><Input type="datetime-local" value={f.start} onChange={(e) => setF({ ...f, start: e.target.value })} /></Field><Field label="End"><Input type="datetime-local" value={f.end} onChange={(e) => setF({ ...f, end: e.target.value })} /></Field></div>
        {prev && <Banner tone="ok">Preview: 80.0 kWh × 0.5 kgCO₂e/kWh = 40.0 kgCO₂e · coverage 100% · baseline-energy-100 v1 · factor-demo-2026 v1</Banner>}
      </Modal>
    </Page>
  );
}
