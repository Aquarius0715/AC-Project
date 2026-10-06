"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, Field, Input, ListRow, Page, Select, SummaryList, Tabs, useToast } from "@/components/ui";

const baselines = [
  { id: "baseline-energy-100", name: "baseline-energy-100", sub: "unit-online-rto · 100.0 kWh · demo_fixed", kwh: 100, units: ["unit-online-rto"] },
  { id: "baseline-2units-100", name: "baseline-2units-100", sub: "unit-limited + unit-online-rto · 100.0 kWh", kwh: 100, units: ["unit-limited", "unit-online-rto"] },
];
const allUnits = ["unit-online-rto", "unit-limited", "unit-offline-rto"];

export default function Energy() {
  const toast = useToast();
  const [tab, setTab] = useState<"analysis" | "baselines">("analysis");
  const [units, setUnits] = useState<string[]>(["unit-online-rto"]);
  const [bid, setBid] = useState(baselines[0].id);
  const [actual, setActual] = useState(80);
  const [sel, setSel] = useState(baselines[0]);
  const [name, setName] = useState("");
  const [kwh, setKwh] = useState("100");
  const b = baselines.find((x) => x.id === bid)!;
  const mismatch = b.units.slice().sort().join() !== units.slice().sort().join();
  const diff = b.kwh - actual;
  const inc = diff < 0;
  return (
    <Page>
      <Tabs value={tab} onChange={setTab} tabs={[{ id: "analysis", label: "Analysis" }, { id: "baselines", label: "Baselines", count: 2 }]} />
      {tab === "analysis" ? (
        <>
          <Card title="Scope">
            <div className="flex flex-wrap items-end gap-4"><div className="flex flex-wrap items-center gap-3 text-[13px]">{allUnits.map((u) => <Check key={u} label={u} checked={units.includes(u)} onChange={(v) => setUnits((s) => (v ? [...s, u] : s.length > 1 ? s.filter((x) => x !== u) : s))} />)}</div><div className="min-w-[200px]"><Field label="Baseline"><Select value={bid} onChange={(e) => setBid(e.target.value)}>{baselines.map((x) => <option key={x.id}>{x.id}</option>)}</Select></Field></div><div className="w-32"><Field label="Actual (kWh, demo)"><Input type="number" value={actual} onChange={(e) => setActual(+e.target.value)} /></Field></div></div>
            <p className="mt-2 text-xs">{mismatch ? <span className="text-crit">✕ Unit set mismatch → no difference can be calculated.</span> : <span className="text-ok">✓ comparable</span>}</p>
          </Card>
          <div className="grid-fluid" style={{ ["--min"as string]: "190px" }}>
            <div className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">Actual</div><div className="text-2xl font-bold">{actual.toFixed(1)} kWh</div><div className="text-[11px] text-muted">measured · coverage 100%</div></div>
            <div className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">Baseline</div><div className="text-2xl font-bold">{b.kwh.toFixed(1)} kWh</div><div className="text-[11px] text-muted">demo_fixed · fictional value</div></div>
            <div className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">Difference</div>{mismatch ? <div className="text-xl font-bold text-muted">Cannot calculate</div> : <div className={`text-2xl font-bold ${inc ? "text-crit" : "text-ok"}`}>{inc ? "Increase" : "Reduction"} {Math.abs(diff).toFixed(1)} kWh</div>}<div className="text-[11px] text-muted">{mismatch ? "Different unit set" : `${inc ? "Increase" : "Reduction"} ${Math.abs((diff / b.kwh) * 100).toFixed(1)}% · not adjusted`}</div></div>
            <div className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">Emissions</div><div className="text-2xl font-bold">{(actual * 0.5).toFixed(1)} kgCO₂e</div><div className="text-[11px] text-muted">{mismatch ? "—" : `${inc ? "Increase" : "Reduction"} ${Math.abs(diff * 0.5).toFixed(1)} kgCO₂e · demo factor`}</div></div>
          </div>
          <div className="split">
            <Card title="Baseline vs actual">
              <div className="flex flex-col gap-3">{[["Baseline", b.kwh, "#c9d8ee"], ["Actual", actual, "#005bea"]].map(([l, v, c]) => <div key={l as string}><div className="mb-1 flex justify-between text-xs"><span>{l as string}</span><b>{(v as number).toFixed(1)} kWh</b></div><div className="h-5 rounded bg-surface2"><div className="h-full rounded" style={{ width: `${Math.min(100, ((v as number) / Math.max(b.kwh, actual, 1)) * 100)}%`, background: c as string }} /></div></div>)}</div>
              <div className="mt-3 grid-fluid" style={{ ["--min"as string]: "150px" }}><div className="text-xs"><span className="text-muted">Cost (actual)</span><br /><b>{(actual * 0.22).toFixed(2)} MYR</b></div><div className="text-xs"><span className="text-muted">Saved cost</span><br /><b>{mismatch ? "—" : `${inc ? "Increase" : "Reduction"} ${Math.abs(diff * 0.22).toFixed(2)} MYR`}</b></div><div className="text-xs"><span className="text-muted">Tariff</span><br /><b>tariff-demo-v1</b></div></div>
              <p className="mt-3 text-xs text-muted">How differences are shown: actual below baseline → “Reduction”; actual above baseline → “Increase” (e.g. 100 → 120 kWh shows Increase 20.0 kWh / 20.0%). Negative savings are never hidden or clamped to zero. No baseline or 0 kWh baseline → “Cannot calculate” for the percentage.</p>
            </Card>
            <Card title="Calculation conditions"><SummaryList items={[["Boundary", "ac_input_electricity — AC input electricity only"], ["Baseline", `${b.id} v1 · demo_fixed · 2026-09-14 08:00–09:00`], ["Baseline assumptions", "Fixed demo baseline for the exact unit set and hour"], ["Emission factor", "factor-demo-2026 v1 · 0.5 kgCO₂e/kWh · demo, not a real regional factor"], ["Coverage", "100% of expected readings"], ["Quality warnings", mismatch ? "Different unit set — difference blocked" : "none"]]} /><p className="mt-2 text-[11px] text-muted">Missing readings, a different unit set or boundary would be listed here and block the difference.</p></Card>
          </div>
        </>
      ) : (
        <div className="split-rev">
          <Card title="Baselines" action={<Btn size="sm" variant="primary" onClick={() => toast("Baseline draft created")}>+ New</Btn>} className="self-start"><div className="flex flex-col gap-2">{baselines.map((x) => <ListRow key={x.id} selected={sel.id === x.id} onClick={() => setSel(x)}><div><b className="text-[13px]">{x.name}</b><div className="text-[11px] text-muted">{x.sub}</div></div></ListRow>)}</div></Card>
          <Card title={sel.name} sub="demo_fixed · version 1"><SummaryList items={[["Units", sel.units.join(", ")], ["Value", `${sel.kwh.toFixed(1)} kWh`], ["Hour", "2026-09-14 08:00–09:00"], ["Method", "demo_fixed (fictional value)"]]} />
            <div className="mt-3 grid-fluid" style={{ ["--min"as string]: "160px" }}><Field label="New version value (kWh)"><Input type="number" value={kwh} onChange={(e) => setKwh(e.target.value)} /></Field></div>
            <div className="mt-3 flex justify-end"><Btn variant="primary" onClick={() => toast("Baseline saved as version 2")}>Save as v2</Btn></div></Card>
        </div>
      )}
    </Page>
  );
}
