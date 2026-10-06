"use client";

import Link from "next/link";
import { useState } from "react";
import { BarChart, Btn, Card, Check, DataTable, Kpi, LinkBtn, Page, Tabs, Toggle, Badge } from "@/components/ui";
import { ExportReportModal } from "@/components/Features";
import { units, week } from "@/lib/client";

const days = ["Mon 14", "Tue 15", "Wed 16", "Thu 17", "Fri 18", "Sat 19", "Sun 20"];
const perUnit: Record<string, { actual: number[]; base: number[] }> = {
  "Bedroom AC": { actual: [10.2, 11.4, 10.8, 12.3, 11.0, 12.6, 11.7], base: [14, 14, 14, 14, 14, 14, 14] },
  "Bedroom AC #2": { actual: [6, 5.2, 6.1, 5.8, 6.4, 7, 6.6], base: [8, 8, 8, 8, 8, 8, 8] },
  "Living room AC": { actual: [9, 8.7, 9.3, 9.9, 10.1, 11.2, 10.5], base: [12, 12, 12, 12, 12, 12, 12] },
  "Kitchen AC": { actual: [7, 7.5, 6.8, 7.2, 7.9, 8.1, 7.4], base: [9, 9, 9, 9, 9, 9, 9] },
};

export default function Energy() {
  const [range, setRange] = useState<"today" | "7d" | "30d" | "custom">("7d");
  const [sel, setSel] = useState<string[]>(["Bedroom AC"]);
  const [view, setView] = useState<"chart" | "table">("chart");
  const [exp, setExp] = useState(false);
  const toggle = (u: string) => setSel((s) => (s.includes(u) ? (s.length > 1 ? s.filter((x) => x !== u) : s) : s.length >= 4 ? s : [...s, u]));
  const actual = days.map((_, i) => +sel.reduce((a, u) => a + perUnit[u].actual[i], 0).toFixed(1));
  const base = days.map((_, i) => sel.reduce((a, u) => a + perUnit[u].base[i], 0));
  const sumA = +actual.reduce((a, b) => a + b, 0).toFixed(1);
  const sumB = base.reduce((a, b) => a + b, 0);
  const saved = +(sumB - sumA).toFixed(1);
  return (
    <Page>
      <Card>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
          <Btn size="sm" className="order-last ml-auto" onClick={() => setExp(true)}>↓ Export</Btn>
          <div className="flex flex-wrap items-center gap-2"><span className="text-xs text-muted">Period</span><Tabs value={range} onChange={setRange} tabs={[{ id: "today", label: "Today" }, { id: "7d", label: "7d" }, { id: "30d", label: "30d" }, { id: "custom", label: "Custom" }]} /></div>
          <span className="text-xs text-muted">Sep 14 00:00 – Sep 21 00:00 (7 days) · Asia/Kuala_Lumpur</span>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-3 text-[13px]"><span className="text-xs text-muted">Units (up to 4)</span>{Object.keys(perUnit).map((u) => <Check key={u} label={u} checked={sel.includes(u)} onChange={() => toggle(u)} />)}</div>
      </Card>
      <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
        <Kpi label="Actual energy" value={sumA.toFixed(1)} unit="kWh" sub={`${sel.length === 1 ? sel[0] : sel.length + " units"} · 7 days · coverage 100%`} />
        <Kpi label="Estimated cost" value={(sumA * 0.5).toFixed(2)} unit="MYR" sub="0.5 MYR/kWh · tariff demo-v1 · taxes excluded" />
        <Kpi label="Savings vs baseline" value={saved.toFixed(1)} unit="kWh" tone="ok" sub={`${((saved / sumB) * 100).toFixed(1)}% · ${(saved * 0.5).toFixed(2)} MYR vs ${sumB.toFixed(1)} kWh baseline`} />
        <Kpi label="Data coverage" value={100} unit="%" sub="All 10,080 minutes observed" />
      </div>
      <Card title="Daily energy — Actual vs Baseline" sub={`kWh per day · ${sel.join(", ")} · Sep 14–20 · baseline = demo_fixed normal operation`} action={<Tabs value={view} onChange={setView} tabs={[{ id: "chart", label: "Chart" }, { id: "table", label: "Table" }]} />}>
        {view === "chart" ? (
          <>
            <BarChart labels={week} series={[base, actual]} unit="kWh" height={200} />
            <div className="mt-1 flex flex-wrap gap-4 text-[11px] text-muted"><span><i className="mr-1 inline-block h-2 w-2 bg-primary" />Actual (measured)</span><span><i className="mr-1 inline-block h-2 w-2 bg-[#c9d8ee]" />Baseline (normal operation)</span></div>
          </>
        ) : <DataTable rows={days.map((d, i) => ({ d, a: actual[i], b: base[i] }))} rowKey={(r) => r.d} cols={[{ key: "d", label: "Day", render: (r) => r.d }, { key: "a", label: "Actual (kWh)", render: (r) => r.a }, { key: "b", label: "Baseline (kWh)", render: (r) => r.b }, { key: "s", label: "Difference", render: (r) => (r.b - r.a).toFixed(1) }]} />}
        <p className="mt-3 text-xs text-muted">◷ <b>Peak window 17:00–21:00</b> — 34% of energy used in peak · highest draw 1.2 kW (Thu 18:40) · pre-cooling before 17:00 lowers peak cost</p>
      </Card>
      <Card title="Carbon impact" sub="Same period & units as above · factor-demo-2026 (0.5 kgCO2e/kWh, demo factor)" action={<LinkBtn size="sm" href="/customer/energy/offsets">ⓘ Learn about offsets</LinkBtn>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <div><div className="text-xs text-muted">Estimated emissions</div><div className="text-2xl font-bold">{(sumA * 0.5).toFixed(1)} <span className="text-sm text-muted">kgCO2e</span></div><div className="text-[11px] text-muted">{sumA} kWh actual × factor</div></div>
          <div><div className="text-xs text-muted">Estimated savings vs baseline</div><div className="text-2xl font-bold text-ok">{(saved * 0.5).toFixed(1)} <span className="text-sm text-muted">kgCO2e</span></div><div className="text-[11px] text-muted">{sumB} → {sumA} kWh</div></div>
          <div><div className="text-xs text-muted">Offsets <Badge tone="muted">Optional</Badge></div><div className="text-[13px]">Demo requests only — see records on the Offsets page</div></div>
        </div>
        <p className="mt-3 text-[11px] text-muted">Estimates only. Savings are not a tradable balance and are not converted into carbon credits.</p>
      </Card>
    <ExportReportModal open={exp} onClose={() => setExp(false)} />
    </Page>
  );
}
