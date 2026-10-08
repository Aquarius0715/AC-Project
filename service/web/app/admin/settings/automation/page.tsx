"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, DataTable, Field, Input, ListRow, Page, Select, Toggle, useToast, cx } from "@/components/ui";

const pols = [
  { id: "p-shave", name: "Peak shaving", sub: "When peak active → fan low", pri: 70, group: "ACROSS CUSTOMERS", on: true },
  { id: "policy-peak-a", name: "Peak-price pre-cool", sub: "When tariff > 0.6 MYR/kWh → set 26 °C", pri: 60, group: "CUSTOMER-A", on: true },
  { id: "p-occ", name: "Occupancy stop", sub: "When not occupied → power off", pri: 50, group: "CUSTOMER-A", on: true },
  { id: "p-solar", name: "Solar surplus cooling", sub: "When solar > 2 kW → set 24 °C", pri: 40, group: "CUSTOMER-A", on: false },
];

export default function AutomationPolicies() {
  const toast = useToast();
  const [sel, setSel] = useState(pols[1]);
  const [showOff, setShowOff] = useState(true);
  const [trigger, setTrigger] = useState("Tariff");
  const [val, setVal] = useState("0.6");
  const [temp, setTemp] = useState("26");
  const [facts, setFacts] = useState([{ unit: "unit-online-rto", fact: "tariff", value: "0.7", q: "now" }, { unit: "unit-limited", fact: "tariff", value: "", q: "no reading" }]);
  const [result, setResult] = useState<null | { unit: string; d: string }[]>(null);
  const [fired, setFired] = useState(false);
  const [conflict, setConflict] = useState(false);
  const tempErr = +temp < 16 || +temp > 30 ? "Outside 16–30 °C on at least one unit" : undefined;
  const simulate = () => setResult(facts.map((f) => (f.value === "" ? { unit: f.unit, d: "missing_data — tariff reading unavailable, action skipped" } : +f.value > +val ? { unit: f.unit, d: `${sel.name} → set ${temp} °C` } : { unit: f.unit, d: "no rule matched" })));
  return (
    <Page>
      <div className="flex items-center justify-between gap-3"><span className="text-xs text-muted">HQ automation policies · priority ↓</span><Check label="Show disabled" checked={showOff} onChange={setShowOff} /></div>
      <div className="split-rev">
        <Card title="HQ automation policies" action={<Btn size="sm" variant="primary" onClick={() => toast("New policy draft created")}>+ New</Btn>} className="self-start">
          {["ACROSS CUSTOMERS", "CUSTOMER-A"].map((g) => <div key={g} className="mb-3"><div className="mb-1 text-[10px] font-bold tracking-wide text-muted">{g}</div><div className="flex flex-col gap-2">{pols.filter((p) => p.group === g && (showOff || p.on)).map((p) => <ListRow key={p.id} selected={sel.id === p.id} onClick={() => { setSel(p); setResult(null); setFired(false); }}><div className="min-w-0 flex-1"><b className="text-[13px]">{p.name}</b><div className="text-[11px] text-muted">{p.sub}</div><div className="text-[11px] text-muted">Priority {p.pri}{!p.on && " · Off"}</div></div></ListRow>)}</div></div>)}
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={sel.name} sub={`${sel.id} · HQ tier · owner hq-operator`} action={<Badge tone={sel.on ? "ok" : "muted"}>{sel.on ? "Enabled" : "Disabled"}</Badge>}>
            <div className="flex flex-col gap-5">
              <section className="grid-fluid" style={{ ["--min"as string]: "160px" }}><Field label="Name"><Input defaultValue={sel.name} /></Field><Field label="Priority" hint="0–100 · higher wins in its tier"><Input type="number" defaultValue={sel.pri} /></Field><Field label="Timezone"><Select><option>Asia/Kuala_Lumpur</option></Select></Field></section>
              <section><h3 className="mb-1 text-[13px] font-bold">Units</h3><div className="flex flex-wrap gap-2">{["Bedroom AC · unit-online-rto", "Lobby AC · unit-limited"].map((u) => <span key={u} className="rounded-full bg-surface2 px-3 py-1 text-xs">{u} ✕</span>)}<Btn size="sm" variant="ghost">Add units…</Btn></div></section>
              <section><h3 className="mb-1 text-[13px] font-bold">When</h3><p className="mb-2 text-xs text-muted">Missing or stale data never triggers the action — the unit is skipped with a reason.</p><Choice value={trigger as "Tariff"} onChange={setTrigger} options={["Occupancy", "Tariff", "Peak", "Solar", "Battery"].map((k) => ({ id: k as "Tariff", label: k }))} /><div className="mt-2 flex flex-wrap items-end gap-2"><span className="pb-2 text-[13px]">When the electricity tariff is &gt;</span><div className="w-24"><Input type="number" step="0.1" value={val} onChange={(e) => setVal(e.target.value)} /></div><span className="pb-2 text-[13px]">MYR / kWh</span></div></section>
              <section><h3 className="mb-1 text-[13px] font-bold">Then</h3><p className="mb-2 text-xs text-muted">Only actions every target unit supports, and that no active restriction blocks, can run.</p><div className="flex flex-wrap items-end gap-2"><span className="pb-2 text-[13px]">Set the temperature to</span><div className="w-24"><Field label="" error={tempErr}><Input type="number" value={temp} onChange={(e) => setTemp(e.target.value)} /></Field></div><span className="pb-2 text-[13px]">°C</span></div>{!tempErr && <p className="text-xs text-ok">✓ within 16–30 °C on both units</p>}</section>
              <section className="rounded-xl bg-surface2 p-3"><h3 className="mb-2 text-[13px] font-bold">How conflicts are resolved</h3><p className="mb-2 text-xs text-muted">Evaluated per unit — at most one action per unit per evaluation.</p><ol className="grid-fluid text-xs" style={{ ["--min"as string]: "180px" }}><li><b>1 Capabilities & active restrictions</b><br />always first</li><li><b>2 HQ policies</b><br />higher priority wins · ties: ascending ID</li><li><b>3 Customer rules</b><br />only when no HQ policy applies</li></ol><p className="mt-2 text-xs">This policy is in tier 2 with priority {sel.pri}. “Peak shaving” (70) wins when both match on the same unit.</p></section>
              {conflict && <Banner tone="warn">CONFLICT — saved from v1 while v2 exists; your input was kept. Reload to merge.</Banner>}
              <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-[11px] text-muted">No unsaved changes · form loaded from policy v1</span><div className="flex gap-2"><Btn onClick={() => setConflict(true)}>Save (simulate old version)</Btn><Btn variant="primary" disabled={!!tempErr} onClick={() => { setConflict(false); toast("Policy saved as v2"); }}>Save</Btn></div></div>
            </div>
          </Card>
          <Card title="Simulate" sub="Evaluates all enabled policies for the chosen units with synthetic facts. Creates no commands.">
            <div className="scroll-x"><table className="w-full min-w-[460px] text-[13px]"><thead className="text-left text-[11px] uppercase text-muted"><tr><th>Unit</th><th>Fact</th><th>Value</th><th>Quality</th></tr></thead><tbody>{facts.map((f, i) => <tr key={f.unit} className="border-t border-line"><td className="py-2 pr-2 font-semibold">{f.unit}</td><td>{f.fact}</td><td className="pr-2"><Input type="number" step="0.1" value={f.value} placeholder="— no reading" onChange={(e) => setFacts((s) => s.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))} /></td><td className="text-xs text-muted">{f.value ? `${f.value} MYR/kWh · now` : "— no reading"}</td></tr>)}</tbody></table></div>
            <div className="mt-3 flex flex-wrap gap-2"><Btn onClick={() => setFacts((s) => [...s, { unit: `unit-${s.length + 1}`, fact: "tariff", value: "0.5", q: "now" }])}>+ Add fact</Btn><Btn variant="primary" onClick={simulate}>Simulate</Btn></div>
            {result && <div className="mt-3"><div className="mb-1 text-[11px] font-bold tracking-wide text-muted">RESULT PER UNIT</div><DataTable rows={result} rowKey={(r) => r.unit} cols={[{ key: "u", label: "Unit", render: (r) => <b>{r.unit}</b> }, { key: "d", label: "Decision / reason", render: (r) => <span className={cx(r.d.startsWith("missing") && "text-warn")}>{r.d}</span> }]} /></div>}
            <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-line pt-3"><p className="max-w-xl text-xs text-muted">Fire (demo) runs the same evaluation for real: selected units get a command through the shared command rules. Uses a one-time key so repeating it does not duplicate commands.</p><Btn variant="danger" disabled={!result || fired} onClick={() => { setFired(true); toast("Fired — command cmd-0500 created (shared Command)"); }}>Fire (demo)</Btn></div>
            {fired && <div className="mt-2"><Banner tone="ok">Fire result → shared Command cmd-0500 for unit-online-rto (set 26 °C). unit-limited skipped (missing_data).</Banner></div>}
          </Card>
        </div>
      </div>
    </Page>
  );
}
