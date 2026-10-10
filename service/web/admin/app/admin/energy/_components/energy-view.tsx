"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, EmptyState, Field, Input, ListRow, Page, Select, SummaryList, Tabs, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { baselineDraft, baselineErrors, baselineInput, type BaselineDraft, type BaselineRow, type SummaryView } from "@ac/web/lib/energy";
import { saveBaseline } from "../actions";

type UnitOption = { id: string; label: string; customerId: string; propertyId: string };
export type EnergyLive = {
  tab: "analysis" | "baselines"; canWrite: boolean; scope: { customerId?: string; propertyId?: string; unitId?: string };
  customers: { id: string; name: string }[]; properties: { id: string; name: string; customerId: string }[]; units: UnitOption[]; baselines: BaselineRow[];
  analysis?: { unitIds: string[]; from: string; to: string; baselineId: string | null; summary: SummaryView | null; error?: string };
  list?: { rows: BaselineRow[]; from: string; to: string; selectedId: string | null };
};

const kpi = (label: string, value: string, sub: string, tone?: "ok" | "crit" | "muted") => (
  <div className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted">{label}</div><div className={`text-xl font-bold ${tone === "ok" ? "text-ok" : tone === "crit" ? "text-crit" : tone === "muted" ? "text-muted" : ""}`}>{value}</div><div className="text-[11px] text-muted">{sub}</div></div>
);

/** Energy-saving analysis (FR-A13) in API mode: the scope, period and baseline live in the URL and the server reads
 * energy.summary; baselines are listed for their scope and saved as new versions with a Server Action. Texts in the
 * display language; periods are typed and shown in Kuala Lumpur time, which the labels name (IR296). */
export function EnergyView({ live }: { live: EnergyLive }) {
  const t = useT();
  const nav = useUrlPatch();
  return (
    <Page>
      <Tabs value={live.tab} onChange={(id) => nav({ tab: id === "analysis" ? null : id, baselineId: null })} tabs={[{ id: "analysis", label: t("Analysis") }, { id: "baselines", label: t("Baselines"), count: live.baselines.length }]} />
      {live.tab === "analysis" && live.analysis ? <Analysis live={live} a={live.analysis} /> : live.list ? <Baselines live={live} l={live.list} /> : null}
    </Page>
  );
}

function Analysis({ live, a }: { live: EnergyLive; a: NonNullable<EnergyLive["analysis"]> }) {
  const t = useT();
  const nav = useUrlPatch();
  const units = live.units.filter((u) => !live.scope.customerId || u.customerId === live.scope.customerId);
  const [from, setFrom] = useState(a.from);
  const [to, setTo] = useState(a.to);
  const [source, setSource] = useState(`${a.from}|${a.to}`);
  if (source !== `${a.from}|${a.to}`) { // the inputs restart from each server result
    setSource(`${a.from}|${a.to}`);
    setFrom(a.from);
    setTo(a.to);
  }
  const s = a.summary;
  const setUnits = (ids: string[]) => nav({ unitIds: ids.length ? ids.join(",") : null });
  return (
    <>
      <Card title={t("Scope")}>
        <div className="flex flex-wrap items-end gap-3">
          <Field label={t("Customer")}><Select value={live.scope.customerId ?? ""} onChange={(e) => nav({ customerId: e.target.value || null, unitIds: null, baselineId: null })}><option value="">{t("All customers")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
          <Field label={t("Baseline")}><Select value={a.baselineId ?? ""} onChange={(e) => nav({ baselineId: e.target.value || null, unitIds: null, from: null, to: null })}><option value="">{t("No baseline")}</option>{live.baselines.map((b) => <option key={b.id} value={b.id}>{b.id.slice(0, 8)} · {b.value} · {b.units}</option>)}</Select></Field>
          <Field label={t("From (Kuala Lumpur)")}><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
          <Field label={t("To")}><Input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
          <Btn size="sm" disabled={!from || !to || (from === a.from && to === a.to)} onClick={() => nav({ from, to })}>{t("Apply period")}</Btn>
        </div>
        <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{units.map((u) => <Check key={u.id} label={u.label} checked={a.unitIds.includes(u.id)} onChange={(on) => setUnits(on ? [...a.unitIds, u.id] : a.unitIds.filter((x) => x !== u.id))} />)}</div>
        {a.error ? <p className="mt-2 text-xs text-crit">✕ {a.error}</p> : a.unitIds.length === 0 ? <p className="mt-2 text-xs text-muted">{t("Choose at least one unit.")}</p> : s && <p className="mt-2 text-xs">{s.comparable ? <span className="text-ok">{t("✓ comparable with the baseline")}</span> : a.baselineId ? <span className="text-crit">{t("✕ No difference can be calculated — see the quality warnings.")}</span> : <span className="text-muted">{t("Choose a baseline to compare.")}</span>}</p>}
      </Card>
      {s && (
        <>
          <div className="grid-fluid" style={{ ["--min" as string]: "190px" }}>
            {kpi(t("Actual"), s.actual, t("coverage {pct}", { pct: s.coverage }))}
            {kpi(t("Baseline"), s.baseline, a.baselineId ? t("the selected baseline version") : t("none selected"))}
            {kpi(t("Difference"), s.difference, t("{percent} · not adjusted", { percent: s.percent }), !s.comparable ? "muted" : s.increase ? "crit" : "ok")}
            {kpi(t("Emissions"), s.emissions, t("{saved} · demo factor", { saved: s.savedEmissions }))}
          </div>
          <div className="split">
            <Card title={t("Cost")}>
              <SummaryList items={[[t("Actual cost"), s.cost], [t("Saved cost"), s.savedCost]]} />
              <p className="mt-3 text-xs text-muted">{t("Actual below the baseline shows “Reduction”, above it “Increase” (100 → 120 kWh is “Increase 20.0 kWh / Increase 20.0%”). Increases are never hidden or clamped to zero; a missing or zero baseline shows “Cannot calculate”. Results are not weather-adjusted.")}</p>
            </Card>
            <Card title={t("Calculation conditions")}>
              <SummaryList items={s.conditions} />
              {s.warnings.length > 0 && <div className="mt-3 flex flex-col gap-1">{s.warnings.map((w) => <Banner key={w} tone="warn">{w}</Banner>)}</div>}
            </Card>
          </div>
        </>
      )}
    </>
  );
}

function Baselines({ live, l }: { live: EnergyLive; l: NonNullable<EnergyLive["list"]> }) {
  const t = useT();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [creating, setCreating] = useState(false);
  const sel = creating ? null : l.rows.find((r) => r.id === l.selectedId) ?? null;
  const key = creating ? "new" : sel ? `${sel.id}:${sel.version}` : "none";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<BaselineDraft>(baselineDraft(sel?.b));
  const [tried, setTried] = useState(false);
  if (source !== key) {
    setSource(key);
    setD(baselineDraft(sel?.b));
    setTried(false);
  }
  const errors = baselineErrors(d, t);
  const set = (patch: Partial<BaselineDraft>) => setD((x) => ({ ...x, ...patch }));
  const props = live.properties.filter((p) => !live.scope.customerId || p.customerId === live.scope.customerId);
  const scopeUnits = live.units.filter((u) => (!live.scope.customerId || u.customerId === live.scope.customerId) && (!live.scope.propertyId || u.propertyId === live.scope.propertyId));
  const [from, setFrom] = useState(l.from);
  const [to, setTo] = useState(l.to);
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveBaseline(baselineInput(d, sel?.id), sel?.version), (b) => (sel ? t("Saved as baseline version {v}", { v: b.version }) : t("Baseline created")), (b) => { setCreating(false); nav({ baselineId: b.id }); });
  };
  const err = (k: string) => (tried ? errors[k] : undefined);
  return (
    <>
      <div className="flex flex-wrap items-end gap-3">
        <Field label={t("Customer")}><Select value={live.scope.customerId ?? ""} onChange={(e) => nav({ customerId: e.target.value || null, propertyId: null, unitId: null, baselineId: null })}><option value="">{t("All customers")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
        <Field label={t("Property")}><Select value={live.scope.propertyId ?? ""} onChange={(e) => nav({ propertyId: e.target.value || null, unitId: null, baselineId: null })}><option value="">{t("All properties")}</option>{props.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select></Field>
        <Field label={t("Unit")}><Select value={live.scope.unitId ?? ""} onChange={(e) => nav({ unitId: e.target.value || null, baselineId: null })}><option value="">{t("All units")}</option>{scopeUnits.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <Field label={t("Period starts from (Kuala Lumpur)")}><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label={t("Period starts before")}><Input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
        <Btn size="sm" disabled={from === l.from && to === l.to} onClick={() => nav({ from: from || null, to: to || null, baselineId: null })}>{t("Apply")}</Btn>
      </div>
      <div className="split-rev">
        <Card title={t("Baselines")} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => setCreating(true)}>{t("+ New")}</Btn>} className="self-start">
          {l.rows.length === 0 ? <EmptyState title={t("No baselines")}>{t("No baseline matches this scope.")}</EmptyState> : <div className="flex flex-col gap-2">{l.rows.map((x) => <ListRow key={x.id} selected={sel?.id === x.id} onClick={() => { setCreating(false); nav({ baselineId: x.id }); }}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.id.slice(0, 8)} · v{x.version}</b><Badge tone={x.method === "demo_fixed" ? "muted" : "primary"}>{x.method}</Badge></div><div className="text-[11px] text-muted">{x.units} · {x.value} · {x.boundaryId}</div><div className="text-[11px] text-muted">{x.period}</div></div></ListRow>)}</div>}
        </Card>
        {creating || sel ? (
          <Card title={sel ? t("Baseline {id}", { id: sel.id.slice(0, 8) }) : t("New baseline")} sub={sel ? t("Version {v} · saving creates version {next}", { v: sel.version, next: sel.version + 1 }) : t("Saving creates version 1")}>
            <div className="flex flex-col gap-4">
              <section><h3 className="mb-1 text-[13px] font-bold">{t("Units")}</h3><div className="flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{live.units.map((u) => <Check key={u.id} label={u.label} checked={d.unitIds.includes(u.id)} onChange={(on) => set({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) })} />)}</div>{err("unitIds") && <p className="mt-1 text-xs text-crit">{err("unitIds")}</p>}</section>
              <section className="grid-fluid" style={{ ["--min" as string]: "190px" }}>
                <Field label={t("Period start (Kuala Lumpur)")} error={err("period")}><Input type="datetime-local" value={d.from} onChange={(e) => set({ from: e.target.value })} /></Field>
                <Field label={t("Period end")}><Input type="datetime-local" value={d.to} onChange={(e) => set({ to: e.target.value })} /></Field>
                <Field label={t("Method")}><Select value={d.method} onChange={(e) => set({ method: e.target.value as BaselineDraft["method"] })}><option value="demo_fixed">demo_fixed ({t("entered value")})</option><option value="demo_period_comparison">demo_period_comparison ({t("measured")})</option></Select></Field>
                {d.method === "demo_fixed" ? <Field label={t("Baseline (kWh, fictional)")} error={err("baselineKWh")}><Input type="number" min="0" step="0.1" value={d.baselineKWh} onChange={(e) => set({ baselineKWh: e.target.value })} /></Field> : <p className="self-end text-xs text-muted">{t("Measured from the period’s readings when saved; no value is entered (SR29).")}</p>}
                <Field label={t("Boundary")} error={err("boundaryId")}><Select value={d.boundaryId} onChange={(e) => set({ boundaryId: e.target.value as BaselineDraft["boundaryId"] })}><option value="ac_input_electricity">ac_input_electricity</option><option value="whole_building_electricity">whole_building_electricity</option></Select></Field>
              </section>
              <Field label={t("Boundary description")} error={err("boundary")}><Input value={d.boundary} maxLength={500} onChange={(e) => set({ boundary: e.target.value })} placeholder={t("AC unit input electricity only")} /></Field>
              <Field label={t("Assumptions")} error={err("assumptions")}><Textarea value={d.assumptions} maxLength={2000} onChange={(e) => set({ assumptions: e.target.value })} /></Field>
              <Field label={t("Source")} error={err("source")}><Input value={d.source} maxLength={500} onChange={(e) => set({ source: e.target.value })} placeholder={t("Demo source (fictional)")} /></Field>
              {sel && <p className="rounded-xl bg-surface2 p-3 text-xs">{t("Existing MRV reports keep the baseline version they reference; saved versions are never recalculated.")}</p>}
              {live.canWrite && <div className="flex justify-end gap-2">{creating && <Btn onClick={() => setCreating(false)}>{t("Cancel")}</Btn>}<Btn variant="primary" disabled={pending} onClick={save}>{sel ? t("Save as v{n}", { n: sel.version + 1 }) : t("Create baseline")}</Btn></div>}
            </div>
          </Card>
        ) : <Card title={t("Baseline")}><EmptyState title={t("Nothing selected")}>{t("Choose a baseline or create one.")}</EmptyState></Card>}
      </div>
    </>
  );
}
