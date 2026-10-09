"use client";

import { useState } from "react";
import { Badge, BarChart, Banner, Btn, Card, Check, Choice, DataTable, DemoBadge, Field, Input, Kpi, LinkBtn, Modal, Page, Select, SummaryList, Tabs } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klLocal, one, type SummaryView } from "@ac/web/lib/energy";
import type { CompareRow, Period, PeriodKind } from "@ac/web/lib/clientEnergy";
import { exportReport, type ReportFile, type ReportInput } from "../actions";

export type EnergyLive = {
  units: { id: string; name: string }[]; unitIds: string[]; period: Period; baselines: { id: string; label: string }[]; baselineId: string | null;
  summary: SummaryView | null; raw?: { kWh: number | null; baseline: number | null; tariff: string; factor: string | null; coverage: number | null };
  daily: { label: string; actual: number | null; baseline: number | null }[]; compare: { rows: CompareRow[]; series: number[][] | null } | null;
  properties: { id: string; name: string }[]; prefs: { locale: string; timezone: string; monthlyReportEmail: boolean } | null; lastMonth: string; query: string; error?: string;
};
const colors = ["#005bea", "#f59e0b", "#10b981", "#8b5cf6"];

/** Energy & cost (FR-C06) in API mode: the period, units (up to 4) and baseline live in the URL; every figure comes from
 * energy.summary (IR68 wording: Reduction / Increase / No change / Cannot calculate). */
export function EnergyView({ live }: { live: EnergyLive }) {
  const nav = useUrlPatch();
  const [view, setView] = useState<"chart" | "table">("chart");
  const [exporting, setExporting] = useState(false);
  const [from, setFrom] = useState(klLocal(live.period.from));
  const [to, setTo] = useState(klLocal(live.period.to));
  const s = live.summary;
  const r = live.raw;
  const names = new Map(live.units.map((u) => [u.id, u.name]));
  const setUnits = (ids: string[]) => nav({ unitIds: ids.join(","), baselineId: null });
  const multi = live.unitIds.length > 1;
  const subject = multi ? `${live.unitIds.length} units` : names.get(live.unitIds[0]) ?? "—";
  if (!live.units.length) return <Page><Banner>No air conditioner is registered yet — energy appears here once HQ adds your units.</Banner></Page>;
  return (
    <Page>
      <Card>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
          <Btn size="sm" className="order-last ml-auto" onClick={() => setExporting(true)}>↓ Export</Btn>
          <div className="flex flex-wrap items-center gap-2"><span className="text-xs text-muted">Period</span>
            <Tabs value={live.period.kind} onChange={(k: PeriodKind) => nav({ period: k === "7d" ? null : k, from: null, to: null })} tabs={[{ id: "today", label: "Today" }, { id: "7d", label: "7d" }, { id: "30d", label: "30d" }, { id: "custom", label: "Custom" }]} />
          </div>
          <span className="text-xs text-muted">{live.period.label}</span>
        </div>
        {live.period.kind === "custom" && (
          <div className="mt-3 flex flex-wrap items-end gap-3">
            <Field label="From"><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
            <Field label="To"><Input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
            <Btn size="sm" variant="primary" onClick={() => nav({ period: "custom", from, to })}>Apply</Btn>
            {live.period.error && <span className="text-xs text-crit">{live.period.error}</span>}
          </div>
        )}
        <div className="mt-3 flex flex-wrap items-center gap-3 text-[13px]">
          <span className="text-xs text-muted">Baseline</span>
          <Select aria-label="Baseline" className="w-auto" value={live.baselineId ?? "none"} onChange={(e) => nav({ baselineId: e.target.value })}>
            {live.baselines.map((b) => <option key={b.id} value={b.id}>{b.label}</option>)}<option value="none">No baseline</option>
          </Select>
          {!live.baselines.length && <span className="text-[11px] text-muted">No baseline covers exactly these units — actual energy stays viewable (DD-C06).</span>}
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2 text-[13px]">
          <span className="text-xs text-muted">Units</span>
          {live.unitIds.map((u) => <Badge key={u} tone="primary">{names.get(u)}{live.unitIds.length > 1 && <button type="button" aria-label={`Remove ${names.get(u)}`} className="ml-1" onClick={() => setUnits(live.unitIds.filter((x) => x !== u))}>✕</button>}</Badge>)}
          {live.unitIds.length < 4 && (
            <Select aria-label="Compare unit" className="w-auto" value="" onChange={(e) => e.target.value && setUnits([...live.unitIds, e.target.value])}>
              <option value="">+ Compare unit</option>{live.units.filter((u) => !live.unitIds.includes(u.id)).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
            </Select>
          )}
          <span className="text-[11px] text-muted">up to 4 units</span>
        </div>
      </Card>
      {live.error && <Banner tone="warn">{live.error}</Banner>}
      {s && r && <>
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <Kpi label="Actual energy" value={r.kWh === null ? "—" : one(r.kWh)} unit={r.kWh === null ? undefined : "kWh"} sub={`${subject} · ${live.period.days} day${live.period.days === 1 ? "" : "s"} · coverage ${s.coverage}`} />
          <Kpi label="Estimated cost" value={s.cost.split(" ")[0]} unit={s.cost === "—" ? undefined : s.cost.split(" ")[1]} sub={`${r.tariff} · taxes excluded`} />
          <Kpi label="Savings vs baseline" value={s.comparable ? s.difference : "—"} tone={s.comparable ? (s.increase ? "warn" : "ok") : undefined}
            sub={s.comparable ? `${s.percent} · ${s.savedCost} vs ${s.baseline} baseline` : live.baselineId ? "Cannot calculate — baseline not comparable for this period" : "No baseline selected"} />
          <Kpi label="Data coverage" value={s.coverage} sub={r.coverage === 1 ? "All expected minutes observed" : "Missing readings are not filled in"} />
        </div>
        <Card title={multi ? "Daily actual energy by unit" : "Daily energy — Actual vs Baseline"} sub={`kWh per day · ${multi ? "max 4 units · baselines are compared in the table" : `${subject}${r.baseline !== null ? " · baseline shown as an even daily share" : ""}`}`}
          action={<Tabs value={view} onChange={setView} tabs={[{ id: "chart", label: "Chart" }, { id: "table", label: "Table" }]} />}>
          {view === "chart" && live.daily.every((d) => !d.actual) ? <p className="rounded-xl bg-surface2 p-4 text-center text-[13px] text-muted">No valid power readings in this period — the chart appears once the units report minute readings (coverage {s.coverage}).</p>
            : view === "chart" ? (multi ? (live.compare?.series
            ? <><BarChart labels={live.daily.map((d) => d.label)} series={live.compare.series} unit="kWh" height={200} colors={colors} />
              <div className="mt-1 flex flex-wrap gap-4 text-[11px] text-muted">{live.unitIds.map((u, i) => <span key={u}><i className="mr-1 inline-block h-2 w-2" style={{ background: colors[i] }} />{names.get(u)}</span>)}</div></>
            : <p className="text-[13px] text-muted">The per-unit daily chart covers up to 7 days — see the table for this period.</p>)
            : <><BarChart labels={live.daily.map((d) => d.label)} series={r.baseline !== null ? [live.daily.map((d) => d.baseline ?? 0), live.daily.map((d) => d.actual ?? 0)] : [live.daily.map((d) => d.actual ?? 0)]} unit="kWh" height={200} />
              <div className="mt-1 flex flex-wrap gap-4 text-[11px] text-muted"><span><i className="mr-1 inline-block h-2 w-2 bg-primary" />Actual (measured)</span>{r.baseline !== null && <span><i className="mr-1 inline-block h-2 w-2 bg-[#c9d8ee]" />Baseline (normal operation)</span>}</div></>)
            : multi && live.compare ? (
              <DataTable rows={live.compare.rows} rowKey={(x) => x.unitId} cols={[
                { key: "u", label: "Unit", render: (x) => <b>{x.name}</b> }, { key: "a", label: "Actual (kWh)", render: (x) => x.actual }, { key: "b", label: "Baseline (kWh)", render: (x) => x.baseline },
                { key: "d", label: "Difference vs baseline", render: (x) => <Badge tone={x.tone}>{x.difference}</Badge> }, { key: "c", label: "Coverage", render: (x) => x.coverage },
              ]} />
            ) : (
              <DataTable rows={live.daily} rowKey={(d) => d.label} cols={[
                { key: "d", label: "Day", render: (d) => d.label }, { key: "a", label: "Actual (kWh)", render: (d) => (d.actual === null ? "—" : one(d.actual)) },
                { key: "b", label: "Baseline (kWh)", render: (d) => (d.baseline === null ? "—" : one(d.baseline)) },
              ]} />
            )}
          {multi && <p className="mt-3 text-[11px] text-muted">Totals across units are shown only when all units share the same calculation boundary; increases are labelled, never shown as a negative saving.</p>}
          {s.warnings.length > 0 && <div className="mt-3"><Banner tone="warn">{s.warnings.join(" · ")}</Banner></div>}
          <details className="mt-3 text-[12px]"><summary className="cursor-pointer font-semibold text-muted">Calculation conditions</summary><div className="mt-2"><SummaryList items={s.conditions} /></div></details>
        </Card>
        <Card title="Carbon impact" sub={`Same period & units as above · ${r.factor ?? "emission factor missing"}`} action={<LinkBtn size="sm" href={`/customer/energy/offsets?${live.query}`}>ⓘ Learn about offsets</LinkBtn>}>
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <div><div className="text-xs text-muted">Estimated emissions</div><div className="text-2xl font-bold">{s.emissions}</div><div className="text-[11px] text-muted">{r.kWh === null ? "no valid readings" : `${one(r.kWh)} kWh actual × factor`}</div></div>
            <div><div className="text-xs text-muted">Estimated savings vs baseline</div><div className={`text-2xl font-bold ${s.comparable ? (s.increase ? "text-warn" : "text-ok") : "text-muted"}`}>{s.savedEmissions}</div><div className="text-[11px] text-muted">{s.comparable ? `${s.baseline} → ${s.actual}` : "needs a comparable baseline"}</div></div>
            <div><div className="text-xs text-muted">Offsets <Badge tone="muted">Optional</Badge></div><div className="text-[13px]">Demo requests only — see records on the Offsets page</div></div>
          </div>
          <p className="mt-3 text-[11px] text-muted">Estimates only. Savings are not a tradable balance and are not converted into carbon credits.</p>
        </Card>
      </>}
      {exporting && <ExportModal live={live} onClose={() => setExporting(false)} />}
    </Page>
  );
}

const sectionLabel = { energy_cost: "Energy & cost by AC (kWh, MYR)", month_comparison: "Comparison with last month", co2_offsets: "CO₂ and offsets", alerts_maintenance: "Alerts & maintenance done" } as const;
type Section = keyof typeof sectionLabel;

function ExportModal({ live, onClose }: { live: EnergyLive; onClose: () => void }) {
  const [pending, run] = useAction();
  const months = Array.from({ length: 12 }, (_, i) => { const d = new Date(`${live.lastMonth}-01T00:00:00Z`); d.setUTCMonth(d.getUTCMonth() - i); return d.toISOString().slice(0, 7); });
  const [month, setMonth] = useState(live.lastMonth);
  const [props, setProps] = useState(live.properties.map((p) => p.id));
  const [sections, setSections] = useState<Section[]>(["energy_cost", "month_comparison", "co2_offsets"]);
  const [format, setFormat] = useState<"pdf" | "csv">("pdf");
  const [email, setEmail] = useState(live.prefs?.monthlyReportEmail ?? false);
  const [file, setFile] = useState<ReportFile | null>(null);
  const label = (m: string) => new Date(`${m}-01T00:00:00Z`).toLocaleDateString("en-US", { month: "long", year: "numeric", timeZone: "UTC" });
  const err = !props.length ? "Choose at least one location" : !sections.length ? "Choose at least one section" : undefined;
  const download = () => {
    const input: ReportInput = { month, propertyIds: props, sections, format };
    const prefs = live.prefs && email !== live.prefs.monthlyReportEmail ? { locale: live.prefs.locale, timezone: live.prefs.timezone, value: email } : null;
    run(() => exportReport(input, prefs), "Report ready", setFile);
  };
  return (
    <Modal open onClose={onClose} title="Export report" footer={<><Btn onClick={onClose}>Close</Btn>{!file && <Btn variant="primary" disabled={pending || !!err} onClick={download}>Download</Btn>}</>}>
      {file ? (
        <div className="flex flex-col gap-3">
          <Banner tone="ok" action={<DemoBadge />}><b>Report ready</b><div className="text-xs">{file.fileName} · {Math.max(1, Math.round(file.size / 1024))} KB · {file.mime === "text/csv" ? "CSV" : "PDF"} — a demo file; nothing is e-mailed now.</div></Banner>
          {live.prefs && email !== live.prefs.monthlyReportEmail && <p className="text-xs text-muted">Monthly e-mail {email ? "turned on" : "turned off"} in your preferences.</p>}
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <Field label="Month" hint="Completed months only"><Select value={month} onChange={(e) => setMonth(e.target.value)}>{months.map((m) => <option key={m} value={m}>{label(m)}</option>)}</Select></Field>
          <Field label="Locations"><div className="flex flex-wrap gap-3">{live.properties.map((p) => <Check key={p.id} label={p.name} checked={props.includes(p.id)} onChange={(on) => setProps(on ? [...props, p.id] : props.filter((x) => x !== p.id))} />)}</div></Field>
          <Field label="Include"><div className="flex flex-col gap-1">{(Object.keys(sectionLabel) as Section[]).map((k) => <Check key={k} label={sectionLabel[k]} checked={sections.includes(k)} onChange={(on) => setSections(on ? [...sections, k] : sections.filter((x) => x !== k))} />)}</div></Field>
          <Field label="Format"><Choice value={format} onChange={(v: "pdf" | "csv") => setFormat(v)} options={[{ id: "pdf", label: "PDF" }, { id: "csv", label: "CSV (raw data)" }]} /></Field>
          {live.prefs && <Check label="Email this report to me every month" checked={email} onChange={setEmail} />}
          {err && <p className="text-xs text-crit">{err}</p>}
        </div>
      )}
    </Modal>
  );
}
