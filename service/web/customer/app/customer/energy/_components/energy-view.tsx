"use client";

import { useState } from "react";
import { Badge, BarChart, Banner, Btn, Card, Check, Choice, DataTable, DemoBadge, Field, Input, Kpi, LinkBtn, Modal, Page, Select, SummaryList, Tabs } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klLocal, one, type SummaryView } from "@ac/web/lib/energy";
import type { CompareRow, Period, PeriodKind } from "@ac/web/lib/clientEnergy";
import { intlTag } from "@ac/web/lib/i18n";
import { useLocale, useT } from "@ac/web/components/I18n";
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
  const t = useT();
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
  const subject = multi ? t("{n} units", { n: live.unitIds.length }) : names.get(live.unitIds[0]) ?? "—";
  const days = t(live.period.days === 1 ? "{n} day" : "{n} days", { n: live.period.days });
  if (!live.units.length) return <Page><Banner>{t("No air conditioner is registered yet — energy appears here once HQ adds your units.")}</Banner></Page>;
  return (
    <Page>
      <Card>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
          <Btn size="sm" className="order-last ml-auto" onClick={() => setExporting(true)}>{t("↓ Export")}</Btn>
          <div className="flex flex-wrap items-center gap-2"><span className="text-xs text-muted">{t("Period")}</span>
            <Tabs value={live.period.kind} onChange={(k: PeriodKind) => nav({ period: k === "7d" ? null : k, from: null, to: null })} tabs={[{ id: "today", label: t("Today") }, { id: "7d", label: t("7d") }, { id: "30d", label: t("30d") }, { id: "custom", label: t("Custom") }]} />
          </div>
          <span className="text-xs text-muted">{live.period.label}</span>
        </div>
        {live.period.kind === "custom" && (
          <div className="mt-3 flex flex-wrap items-end gap-3">
            {/* the periods are Kuala Lumpur days (REV18-035), so the custom range is typed in Kuala Lumpur time and says so */}
            <Field label={t("From")} hint="Asia/Kuala_Lumpur"><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
            <Field label={t("To")} hint="Asia/Kuala_Lumpur"><Input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
            <Btn size="sm" variant="primary" onClick={() => nav({ period: "custom", from, to })}>{t("Apply")}</Btn>
            {live.period.error && <span className="text-xs text-crit">{live.period.error}</span>}
          </div>
        )}
        <div className="mt-3 flex flex-wrap items-center gap-3 text-[13px]">
          <span className="text-xs text-muted">{t("Baseline")}</span>
          <Select aria-label={t("Baseline")} className="w-auto" value={live.baselineId ?? "none"} onChange={(e) => nav({ baselineId: e.target.value })}>
            {live.baselines.map((b) => <option key={b.id} value={b.id}>{b.label}</option>)}<option value="none">{t("No baseline")}</option>
          </Select>
          {!live.baselines.length && <span className="text-[11px] text-muted">{t("No baseline covers exactly these units — actual energy stays viewable (DD-C06).")}</span>}
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2 text-[13px]">
          <span className="text-xs text-muted">{t("Units")}</span>
          {live.unitIds.map((u) => <Badge key={u} tone="primary">{names.get(u)}{live.unitIds.length > 1 && <button type="button" aria-label={t("Remove {name}", { name: names.get(u) ?? "" })} className="ml-1" onClick={() => setUnits(live.unitIds.filter((x) => x !== u))}>✕</button>}</Badge>)}
          {live.unitIds.length < 4 && (
            <Select aria-label={t("Compare unit")} className="w-auto" value="" onChange={(e) => e.target.value && setUnits([...live.unitIds, e.target.value])}>
              <option value="">{t("+ Compare unit")}</option>{live.units.filter((u) => !live.unitIds.includes(u.id)).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
            </Select>
          )}
          <span className="text-[11px] text-muted">{t("up to 4 units")}</span>
        </div>
      </Card>
      {live.error && <Banner tone="warn">{live.error}</Banner>}
      {s && r && <>
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <Kpi label={t("Actual energy")} value={r.kWh === null ? "—" : one(r.kWh)} unit={r.kWh === null ? undefined : "kWh"} sub={t("{subject} · {days} · coverage {coverage}", { subject, days, coverage: s.coverage })} />
          <Kpi label={t("Estimated cost")} value={s.cost.split(" ")[0]} unit={s.cost === "—" ? undefined : s.cost.split(" ")[1]} sub={t("{tariff} · taxes excluded", { tariff: r.tariff })} />
          <Kpi label={t("Savings vs baseline")} value={s.comparable ? s.difference : "—"} tone={s.comparable ? (s.increase ? "warn" : "ok") : undefined}
            sub={s.comparable ? t("{percent} · {cost} vs {baseline} baseline", { percent: s.percent, cost: s.savedCost, baseline: s.baseline }) : t(live.baselineId ? "Cannot calculate — baseline not comparable for this period" : "No baseline selected")} />
          <Kpi label={t("Data coverage")} value={s.coverage} sub={t(r.coverage === 1 ? "All expected minutes observed" : "Missing readings are not filled in")} />
        </div>
        <Card title={t(multi ? "Daily actual energy by unit" : "Daily energy — Actual vs Baseline")} sub={multi ? t("kWh per day · max 4 units · baselines are compared in the table") : t(r.baseline !== null ? "kWh per day · {subject} · baseline shown as an even daily share" : "kWh per day · {subject}", { subject })}
          action={<Tabs value={view} onChange={setView} tabs={[{ id: "chart", label: t("Chart") }, { id: "table", label: t("Table") }]} />}>
          {view === "chart" && live.daily.every((d) => !d.actual) ? <p className="rounded-xl bg-surface2 p-4 text-center text-[13px] text-muted">{t("No valid power readings in this period — the chart appears once the units report minute readings (coverage {coverage}).", { coverage: s.coverage })}</p>
            : view === "chart" ? (multi ? (live.compare?.series
            ? <><BarChart labels={live.daily.map((d) => d.label)} series={live.compare.series} unit="kWh" height={200} colors={colors} />
              <div className="mt-1 flex flex-wrap gap-4 text-[11px] text-muted">{live.unitIds.map((u, i) => <span key={u}><i className="mr-1 inline-block h-2 w-2" style={{ background: colors[i] }} />{names.get(u)}</span>)}</div></>
            : <p className="text-[13px] text-muted">{t("The per-unit daily chart covers up to 7 days — see the table for this period.")}</p>)
            : <><BarChart labels={live.daily.map((d) => d.label)} series={r.baseline !== null ? [live.daily.map((d) => d.baseline ?? 0), live.daily.map((d) => d.actual ?? 0)] : [live.daily.map((d) => d.actual ?? 0)]} unit="kWh" height={200} />
              <div className="mt-1 flex flex-wrap gap-4 text-[11px] text-muted"><span><i className="mr-1 inline-block h-2 w-2 bg-primary" />{t("Actual (measured)")}</span>{r.baseline !== null && <span><i className="mr-1 inline-block h-2 w-2 bg-[#c9d8ee]" />{t("Baseline (normal operation)")}</span>}</div></>)
            : multi && live.compare ? (
              <DataTable rows={live.compare.rows} rowKey={(x) => x.unitId} cols={[
                { key: "u", label: t("Unit"), render: (x) => <b>{x.name}</b> }, { key: "a", label: t("Actual (kWh)"), render: (x) => x.actual }, { key: "b", label: t("Baseline (kWh)"), render: (x) => x.baseline },
                { key: "d", label: t("Difference vs baseline"), render: (x) => <Badge tone={x.tone}>{x.difference}</Badge> }, { key: "c", label: t("Coverage"), render: (x) => x.coverage },
              ]} />
            ) : (
              <DataTable rows={live.daily} rowKey={(d) => d.label} cols={[
                { key: "d", label: t("Day"), render: (d) => d.label }, { key: "a", label: t("Actual (kWh)"), render: (d) => (d.actual === null ? "—" : one(d.actual)) },
                { key: "b", label: t("Baseline (kWh)"), render: (d) => (d.baseline === null ? "—" : one(d.baseline)) },
              ]} />
            )}
          {multi && <p className="mt-3 text-[11px] text-muted">{t("Totals across units are shown only when all units share the same calculation boundary; increases are labelled, never shown as a negative saving.")}</p>}
          {s.warnings.length > 0 && <div className="mt-3"><Banner tone="warn">{s.warnings.join(" · ")}</Banner></div>}
          <details className="mt-3 text-[12px]"><summary className="cursor-pointer font-semibold text-muted">{t("Calculation conditions")}</summary><div className="mt-2"><SummaryList items={s.conditions} /></div></details>
        </Card>
        <Card title={t("Carbon impact")} sub={t("Same period & units as above · {factor}", { factor: r.factor ?? t("emission factor missing") })} action={<LinkBtn size="sm" href={`/customer/energy/offsets?${live.query}`}>{t("ⓘ Learn about offsets")}</LinkBtn>}>
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <div><div className="text-xs text-muted">{t("Estimated emissions")}</div><div className="text-2xl font-bold">{s.emissions}</div><div className="text-[11px] text-muted">{r.kWh === null ? t("no valid readings") : t("{kWh} kWh actual × factor", { kWh: one(r.kWh) })}</div></div>
            <div><div className="text-xs text-muted">{t("Estimated savings vs baseline")}</div><div className={`text-2xl font-bold ${s.comparable ? (s.increase ? "text-warn" : "text-ok") : "text-muted"}`}>{s.savedEmissions}</div><div className="text-[11px] text-muted">{s.comparable ? `${s.baseline} → ${s.actual}` : t("needs a comparable baseline")}</div></div>
            <div><div className="text-xs text-muted">{t("Offsets")} <Badge tone="muted">{t("Optional")}</Badge></div><div className="text-[13px]">{t("Demo requests only — see records on the Offsets page")}</div></div>
          </div>
          <p className="mt-3 text-[11px] text-muted">{t("Estimates only. Savings are not a tradable balance and are not converted into carbon credits.")}</p>
        </Card>
      </>}
      {exporting && <ExportModal live={live} onClose={() => setExporting(false)} />}
    </Page>
  );
}

const sectionLabel = { energy_cost: "Energy & cost by AC (kWh, MYR)", month_comparison: "Comparison with last month", co2_offsets: "CO₂ and offsets", alerts_maintenance: "Alerts & maintenance done" } as const;
type Section = keyof typeof sectionLabel;

function ExportModal({ live, onClose }: { live: EnergyLive; onClose: () => void }) {
  const t = useT();
  const locale = useLocale();
  const [pending, run] = useAction();
  const months = Array.from({ length: 12 }, (_, i) => { const d = new Date(`${live.lastMonth}-01T00:00:00Z`); d.setUTCMonth(d.getUTCMonth() - i); return d.toISOString().slice(0, 7); });
  const [month, setMonth] = useState(live.lastMonth);
  const [props, setProps] = useState(live.properties.map((p) => p.id));
  const [sections, setSections] = useState<Section[]>(["energy_cost", "month_comparison", "co2_offsets"]);
  const [format, setFormat] = useState<"pdf" | "csv">("pdf");
  const [email, setEmail] = useState(live.prefs?.monthlyReportEmail ?? false);
  const [file, setFile] = useState<ReportFile | null>(null);
  const label = (m: string) => new Date(`${m}-01T00:00:00Z`).toLocaleDateString(intlTag(locale), { month: "long", year: "numeric", timeZone: "UTC" });
  const err = !props.length ? t("Choose at least one location") : !sections.length ? t("Choose at least one section") : undefined;
  const download = () => {
    const input: ReportInput = { month, propertyIds: props, sections, format };
    const prefs = live.prefs && email !== live.prefs.monthlyReportEmail ? { locale: live.prefs.locale, timezone: live.prefs.timezone, value: email } : null;
    run(() => exportReport(input, prefs), t("Report ready"), setFile);
  };
  return (
    <Modal open onClose={onClose} title={t("Export report")} footer={<><Btn onClick={onClose}>{t("Close")}</Btn>{!file && <Btn variant="primary" disabled={pending || !!err} onClick={download}>{t("Download")}</Btn>}</>}>
      {file ? (
        <div className="flex flex-col gap-3">
          <Banner tone="ok" action={<DemoBadge />}><b>{t("Report ready")}</b><div className="text-xs">{t("{file} · {kb} KB · {format} — a demo file; nothing is e-mailed now.", { file: file.fileName, kb: Math.max(1, Math.round(file.size / 1024)), format: file.mime === "text/csv" ? "CSV" : "PDF" })}</div></Banner>
          {live.prefs && email !== live.prefs.monthlyReportEmail && <p className="text-xs text-muted">{t(email ? "Monthly e-mail turned on in your preferences." : "Monthly e-mail turned off in your preferences.")}</p>}
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <Field label={t("Month")} hint={t("Completed months only")}><Select value={month} onChange={(e) => setMonth(e.target.value)}>{months.map((m) => <option key={m} value={m}>{label(m)}</option>)}</Select></Field>
          <Field label={t("Locations")}><div className="flex flex-wrap gap-3">{live.properties.map((p) => <Check key={p.id} label={p.name} checked={props.includes(p.id)} onChange={(on) => setProps(on ? [...props, p.id] : props.filter((x) => x !== p.id))} />)}</div></Field>
          <Field label={t("Include")}><div className="flex flex-col gap-1">{(Object.keys(sectionLabel) as Section[]).map((k) => <Check key={k} label={t(sectionLabel[k])} checked={sections.includes(k)} onChange={(on) => setSections(on ? [...sections, k] : sections.filter((x) => x !== k))} />)}</div></Field>
          <Field label={t("Format")}><Choice value={format} onChange={(v: "pdf" | "csv") => setFormat(v)} options={[{ id: "pdf", label: "PDF" }, { id: "csv", label: t("CSV (raw data)") }]} /></Field>
          {live.prefs && <Check label={t("Email this report to me every month")} checked={email} onChange={setEmail} />}
          {err && <p className="text-xs text-crit">{err}</p>}
        </div>
      )}
    </Modal>
  );
}
