"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, DataTable, Field, LineChart, Modal, Page, Select, Tabs, Textarea, useToast } from "@/components/ui";

const series = {
  co2: { label: "CO2", unit: "ppm", pts: [620, 600, 580, 560, 640, 760, 880, 760, 700, 820, 940, 1000], min: 400, max: 1200, th: 1000 },
  pm25: { label: "PM2.5", unit: "µg/m³", pts: [9, 8, 8, 10, 11, 12, 14, 13, 12, 12, 12, 12], min: 0, max: 40, th: 35 },
  temp: { label: "Temp", unit: "°C", pts: [26, 26, 25, 25, 26, 27, 28, 28, 27, 28, 28, 28], min: 20, max: 34, th: 30 },
  hum: { label: "Humidity", unit: "%", pts: [55, 56, 58, 60, 62, 60, 59, 60, 61, 60, 60, 60], min: 30, max: 80, th: 70 },
} as const;

export default function AirQuality() {
  const toast = useToast();
  const [metric, setMetric] = useState<keyof typeof series>("co2");
  const [win, setWin] = useState<"1h" | "24h" | "7d">("24h");
  const [view, setView] = useState<"chart" | "table">("chart");
  const [log, setLog] = useState(false);
  const [method, setMethod] = useState("Opened window");
  const [log2, setLog2] = useState<{ t: string; m: string }[]>([{ t: "Yesterday 18:10", m: "Ran ventilation fan · 20 min" }]);
  const [missing, setMissing] = useState(false);
  const s = series[metric];
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-line bg-surface px-4 py-3 text-xs">
        <div className="flex items-center gap-2"><span className="text-muted">Viewing</span><select className="rounded-control border border-line px-2.5 py-1.5 font-semibold" aria-label="Unit"><option>Bedroom (Bedroom AC)</option><option>Living room AC</option></select></div>
        <span className="text-muted">Updated 09:12:30 · <button className="underline" onClick={() => setMissing((m) => !m)}>{missing ? "show normal" : "show missing-data state"}</button></span>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "210px" }}>
        {[
          { l: "CO2", v: missing ? "—" : "1000", u: "ppm", b: missing ? <Badge tone="unknown">Not measured</Badge> : <Badge tone="warn" icon="⚠">High</Badge>, s: missing ? "No data in last 15 min — never shown as normal" : "Observed 09:12 · 5-min average", n: missing ? "" : "Ventilation recommended (≥ 1000 ppm)" },
          { l: "PM2.5", v: "12", u: "µg/m³", b: <Badge tone="ok" icon="✓">Within guide</Badge>, s: "Observed 09:12", n: "No current advice" },
          { l: "Temperature", v: "28.0", u: "°C", b: <Badge tone="muted" icon="●">Measured</Badge>, s: "Observed 09:12:30", n: "Setpoint is on Unit Control" },
          { l: "Humidity", v: "60", u: "%", b: <Badge tone="muted" icon="●">Measured</Badge>, s: "Observed 09:12:30", n: "No current advice" },
        ].map((m) => (
          <div key={m.l} className="rounded-2xl border border-line bg-surface p-4"><div className="flex justify-between text-xs text-muted"><span>{m.l}</span>{m.b}</div><div className="text-[28px] font-bold">{m.v} <span className="text-sm font-medium text-muted">{m.u}</span></div><div className="text-[11px] text-muted">{m.s}</div><div className="mt-1 text-xs font-semibold">{m.n}</div></div>
        ))}
      </div>
      <Card><div className="flex flex-wrap items-center justify-between gap-2 text-[13px]"><span><b>Allergen (dust mite)</b> <Badge tone="warn" icon="⚠">Detected</Badge></span><span className="text-xs text-muted">Source: demo_observation · 09:00 · cleaning recommended</span></div></Card>
      {!missing && <Banner tone="warn" action={<Btn size="sm" variant="primary" onClick={() => setLog(true)}>Log ventilation</Btn>}><b>Ventilation recommended</b><div className="text-xs text-muted">CO2 1000 ppm in Bedroom (≥ 1000 ppm guide). Open a window or run your ventilation fan, then log it — the record is kept in this room’s ventilation history.</div></Banner>}
      <Card title={`${s.label} — last ${win === "24h" ? "24 hours" : win === "1h" ? "hour" : "7 days"}`} sub={`${s.unit} · Bedroom · rolling window ending 09:12 · 5-min averages`} action={<div className="flex flex-wrap gap-2"><Tabs value={metric} onChange={setMetric} tabs={(Object.keys(series) as (keyof typeof series)[]).map((k) => ({ id: k, label: series[k].label }))} /><Tabs value={win} onChange={setWin} tabs={[{ id: "1h", label: "1h" }, { id: "24h", label: "24h" }, { id: "7d", label: "7d" }]} /><Tabs value={view} onChange={setView} tabs={[{ id: "chart", label: "Chart" }, { id: "table", label: "Table" }]} /></div>}>
        {view === "chart" ? <><LineChart points={missing ? s.pts.map((p, i) => (i > 4 && i < 8 ? null : p)) : [...s.pts]} min={s.min} max={s.max} threshold={s.th} height={200} labels={["09:12 yesterday", "15:00", "21:00", "03:00", "09:12 now"]} /><div className="mt-1 flex gap-4 text-[11px] text-muted"><span className="text-primary">— {s.label} ({s.unit})</span><span className="text-crit">- - {s.th} {s.unit} guide</span></div></> : <DataTable rows={s.pts.map((p, i) => ({ t: `${(9 + i * 2) % 24}:12`, p }))} rowKey={(r) => r.t + r.p} cols={[{ key: "t", label: "Time", render: (r) => r.t }, { key: "p", label: `${s.label} (${s.unit})`, render: (r) => r.p }]} />}
      </Card>
      <Card title="Ventilation history">{log2.map((l, i) => <div key={i} className="flex flex-wrap justify-between gap-2 border-b border-line py-2 text-[13px] last:border-0"><span>{l.m}</span><span className="text-xs text-muted">{l.t}</span></div>)}</Card>
      <Modal open={log} onClose={() => setLog(false)} title="Log ventilation" footer={<><Btn onClick={() => setLog(false)}>Cancel</Btn><Btn variant="primary" onClick={() => { setLog2((l) => [{ t: "Today 09:15", m: method }, ...l]); setLog(false); toast("Ventilation logged"); }}>Save record</Btn></>}>
        <Field label="What did you do?"><Select value={method} onChange={(e) => setMethod(e.target.value)}><option>Opened window</option><option>Ran ventilation fan</option><option>Manual ventilation</option></Select></Field>
        <Field label="Note (optional)"><Textarea placeholder="e.g. 15 minutes" /></Field>
      </Modal>
    </Page>
  );
}
