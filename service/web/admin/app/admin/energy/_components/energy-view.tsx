"use client";
const DEMO_FIXED = "demo_fixed";

import { useId, useState } from "react";
import { Badge, Banner, Btn, Card, Check, Choice, EmptyState, Field, Input, ListRow, Page, Select, SummaryList, Tabs, Textarea } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { baselineDraft, baselineErrors, baselineInput, type BaselineDraft, type BaselineRow, type SummaryView } from "@ac/web/lib/energy";
import { focusField } from "@ac/web/lib/focusField";
import { deleteBaseline, saveBaseline } from "../actions";
import { DeleteDialog } from "@ac/web/components/DeleteDialog";
import { keysOf } from "@ac/web/lib/keysOf";
import { Icon } from "@ac/web/components/Icon";
import { DateTimeSelect } from "@ac/web/components/TimeSelect";

interface UnitOption { id: string; label: string; customerId: string; propertyId: string }
export interface EnergyLive {
  tab: "analysis" | "baselines"; canWrite: boolean; scope: { customerId?: string; propertyId?: string; unitId?: string };
  customers: { id: string; name: string }[]; properties: { id: string; name: string; customerId: string }[]; units: UnitOption[]; baselines: BaselineRow[];
  analysis?: { unitIds: string[]; from: string; to: string; baselineId: string | null; summary: SummaryView | null; error?: string };
  list?: { rows: BaselineRow[]; from: string; to: string; selectedId: string | null };
}

const TEXT_TONE = new Map([["ok", "text-ok"], ["crit", "text-crit"], ["muted", "text-muted-foreground"]]);
/** The difference's tone: muted when not comparable, otherwise an increase is critical and a saving ok. */
function differenceTone(comparable: boolean, increase: boolean): "ok" | "crit" | "muted" {
  if (!comparable) return "muted";
  return increase ? "crit" : "ok";
}
const kpi = (label: string, value: string, sub: string, tone?: "ok" | "crit" | "muted") => (
    <div className="rounded-2xl border border-line bg-surface p-4"><div className="text-xs text-muted-foreground">{label}</div><div className={`text-xl font-bold ${TEXT_TONE.get(tone ?? "") ?? ""}`}>{value}</div><div className="text-[11px] text-muted-foreground">{sub}</div></div>
  );

/** Energy-saving analysis (FR-A13) in API mode: the scope, period and baseline live in the URL and the server reads
energy.summary; baselines are listed for their scope and saved as new versions with a Server Action. Texts in the
display language; periods are typed and shown in Kuala Lumpur time, which the labels name (IR296). */
export function EnergyView({ live }: Readonly<{ live: EnergyLive }>) {
  const t = useT();
  const nav = useUrlPatch();
  return (
    <Page>
      <Tabs value={live.tab} onChange={(id) => { nav({ tab: id === "analysis" ? null : id, baselineId: null }); }} tabs={[{ id: "analysis", label: t("Analysis") }, { id: "baselines", label: t("Baselines"), count: live.baselines.length }]} />
      {live.tab === "analysis" && live.analysis ? <Analysis live={live} a={live.analysis} /> : null}{!(live.tab === "analysis" && live.analysis) && live.list && <Baselines live={live} l={live.list} />}{!(live.tab === "analysis" && live.analysis) && !live.list && null}
    </Page>
  );
}

function Analysis({ live, a }: Readonly<{ live: EnergyLive; a: NonNullable<EnergyLive["analysis"]> }>) {
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
  const setUnits = (ids: string[]) => { nav({ unitIds: ids.length > 0 ? ids.join(",") : null }); };
  return (
    <>
      <Card title={t("Scope")}>
        <div className="flex flex-wrap items-end gap-3">
          <Field label={t("Customer")}><Select value={live.scope.customerId ?? ""} onChange={(e) => { nav({ customerId: e.target.value || null, unitIds: null, baselineId: null }); }}><option value="">{t("All customers")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
          <Field label={t("Baseline")}><Select value={a.baselineId ?? ""} onChange={(e) => { nav({ baselineId: e.target.value || null, unitIds: null, from: null, to: null }); }}><option value="">{t("No baseline")}</option>{live.baselines.map((b) => <option key={b.id} value={b.id}>{b.id.slice(0, 8)} · {b.dates} · {b.value} · {b.units}</option>)}</Select></Field>
          <Field label={t("From (Kuala Lumpur)")}><DateTimeSelect value={from} onChange={(v) => { setFrom(v); }} /></Field>
          <Field label={t("To")}><DateTimeSelect value={to} onChange={(v) => { setTo(v); }} /></Field>
          <Btn size="sm" disabled={!from || !to || (from === a.from && to === a.to)} onClick={() => { nav({ from, to }); }}>{t("Apply period")}</Btn>
        </div>
        <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{units.map((u) => <Check key={u.id} label={u.label} checked={a.unitIds.includes(u.id)} onChange={(on) => { setUnits(on ? [...a.unitIds, u.id] : a.unitIds.filter((x) => x !== u.id)); }} />)}</div>
        {a.error ? <p className="mt-2 flex items-center gap-1 text-xs text-crit"><Icon name="error" />{a.error}</p> : null}{!a.error && a.unitIds.length === 0 && <p className="mt-2 text-xs text-muted-foreground">{t("Choose at least one unit.")}</p>}{!a.error && a.unitIds.length > 0 && s && <p className="mt-2 text-xs">{s.comparable ? <span className="inline-flex items-center gap-1 text-ok"><Icon name="check" />{t("comparable with the baseline")}</span> : null}{!s.comparable && a.baselineId && <span className="inline-flex items-center gap-1 text-crit"><Icon name="error" />{t("No difference can be calculated — see the quality warnings.")}</span>}{!s.comparable && !a.baselineId && <span className="text-muted-foreground">{t("Choose a baseline to compare.")}</span>}</p>}
      </Card>
      {s && (
        <>
          <div className="grid-fluid" style={{ ["--min" as string]: "190px" }}>
            {kpi(t("Actual"), s.actual, t("coverage {pct}", { pct: s.coverage }))}
            {kpi(t("Baseline"), s.baseline, t(a.baselineId ? "the selected baseline version" : "none selected"))}
            {kpi(t("Difference"), s.difference, t("{percent} · not adjusted", { percent: s.percent }), differenceTone(s.comparable, s.increase))}
            {kpi(t("Emissions"), s.emissions, t("{saved} · demo factor", { saved: s.savedEmissions }))}
          </div>
          <div className="split">
            <Card title={t("Cost")}>
              <SummaryList items={[[t("Actual cost"), s.cost], [t("Saved cost"), s.savedCost]]} />
              <p className="mt-3 text-xs text-muted-foreground">{t("Actual below the baseline shows “Reduction”, above it “Increase” (100 → 120 kWh is “Increase 20.0 kWh / Increase 20.0%”). Increases are never hidden or clamped to zero; a missing or zero baseline shows “Cannot calculate”. Results are not weather-adjusted.")}</p>
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

function Baselines({ live, l }: Readonly<{ live: EnergyLive; l: NonNullable<EnergyLive["list"]> }>) {
  const t = useT();
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const [creating, setCreating] = useState(false);
  const [removing, setRemoving] = useState(false);
  const sel = creating ? null : l.rows.find((r) => r.id === l.selectedId) ?? null;
  const selKey = sel ? `${sel.id}:${String(sel.version)}` : "none";
  const key = creating ? "new" : selKey;
  const [source, setSource] = useState(key);
  const [d, setD] = useState<BaselineDraft>(baselineDraft(sel?.b));
  const [tried, setTried] = useState(false);
  if (source !== key) {
    setSource(key);
    setD(baselineDraft(sel?.b));
    setTried(false);
  }
  const errors = baselineErrors(d, t);
  const set = (patch: Partial<BaselineDraft>) => { setD((x) => ({ ...x, ...patch })); };
  const props = live.properties.filter((p) => !live.scope.customerId || p.customerId === live.scope.customerId);
  const scopeUnits = live.units.filter((u) => (!live.scope.customerId || u.customerId === live.scope.customerId) && (!live.scope.propertyId || u.propertyId === live.scope.propertyId));
  const [from, setFrom] = useState(l.from);
  const [to, setTo] = useState(l.to);
  const customer = live.customers.find((c) => c.id === live.scope.customerId)?.name;
  const ids = { unitIds: useId(), period: useId(), baselineKWh: useId(), boundaryId: useId(), boundary: useId(), assumptions: useId(), source: useId() };
  const save = () => {
    setTried(true);
    // a failed save takes focus to the first field to fix, top to bottom (IR586, as IR573)
    const bad = keysOf(ids).find((k) => errors[k]);
    if (bad) { focusField(ids[bad]); return; }
    run(() => saveBaseline(baselineInput(d, sel?.id), sel?.version), (b) => (sel ? t("Saved as baseline version {v}", { v: b.version }) : t("Baseline created")), (b) => { setCreating(false); nav({ baselineId: b.id }); });
  };
  const fieldError = (k: string) => (tried ? errors[k] : undefined);
  return (
    <>
      <div className="flex flex-wrap items-end gap-3">
        <span className="pb-2 text-xs font-semibold text-muted-foreground">{t("Scope")}</span>
        <Field label={t("Customer")}><Select value={live.scope.customerId ?? ""} onChange={(e) => { nav({ customerId: e.target.value || null, propertyId: null, unitId: null, baselineId: null }); }}><option value="">{t("All customers")}</option>{live.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select></Field>
        <Field label={t("Property")}><Select value={live.scope.propertyId ?? ""} onChange={(e) => { nav({ propertyId: e.target.value || null, unitId: null, baselineId: null }); }}><option value="">{t("All properties")}</option>{props.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select></Field>
        <Field label={t("Unit")}><Select value={live.scope.unitId ?? ""} onChange={(e) => { nav({ unitId: e.target.value || null, baselineId: null }); }}><option value="">{t("All units")}</option>{scopeUnits.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <Field label={t("Period starts from (Kuala Lumpur)")}><DateTimeSelect value={from} onChange={(v) => { setFrom(v); }} /></Field>
        <Field label={t("Period starts before")}><DateTimeSelect value={to} onChange={(v) => { setTo(v); }} /></Field>
        <Btn size="sm" disabled={from === l.from && to === l.to} onClick={() => { nav({ from: from || null, to: to || null, baselineId: null }); }}>{t("Apply")}</Btn>
        <span className="ml-auto pb-2 text-xs text-muted-foreground">{t(l.rows.length === 1 ? "1 baseline in scope" : "{n} baselines in scope", { n: l.rows.length })}</span>
      </div>
      <div className="split-rev">
        <Card title={customer ? t("Baselines · {customer}", { customer }) : t("Baselines · all customers")} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => { setCreating(true); }}><Icon name="add" /> {t("New baseline")}</Btn>} className="self-start">
          {l.rows.length === 0 ? <EmptyState title={t("No baselines")}>{t("No baseline matches this scope.")}</EmptyState> : <div className="flex flex-col gap-2">{l.rows.map((x) => <ListRow key={x.id} selected={sel?.id === x.id} onClick={() => { setCreating(false); nav({ baselineId: x.id }); }}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.id.slice(0, 8)}</b><Badge tone={x.method === DEMO_FIXED ? "muted" : "primary"}>{t(x.method === DEMO_FIXED ? "v{v} · modeled" : "v{v} · measured", { v: x.version })}</Badge></div><div className="text-[11px] text-muted-foreground">{x.units} · {x.period}</div><div className="text-[11px] text-muted-foreground">{x.method} · {x.value} · {x.boundaryId}</div></div></ListRow>)}</div>}
        </Card>
        {creating || sel ? (
          <Card title={sel ? <span className="flex flex-wrap items-center gap-2">{t("Baseline {id}", { id: sel.id.slice(0, 8) })}<Badge>{t("Version {v}", { v: sel.version })}</Badge><Badge tone="warn">{t("Editing → v{n}", { n: sel.version + 1 })}</Badge></span> : t("New baseline")}
            sub={sel ? t("Saving creates version {next} — the version number is assigned automatically", { next: sel.version + 1 }) : t("Saving creates version 1")} className="self-start">
            <div className="flex flex-col gap-4">
              <section><h3 className="mb-1 text-[13px] font-bold">{t("Units · the same set is required for comparison")}</h3><div id={ids.unitIds} className="flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{live.units.map((u) => <Check key={u.id} label={u.label} checked={d.unitIds.includes(u.id)} onChange={(on) => { set({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) }); }} />)}</div>{fieldError("unitIds") && <p className="mt-1 text-xs text-crit">{fieldError("unitIds")}</p>}</section>
              <section className="grid-fluid" style={{ ["--min" as string]: "190px" }}>
                <Field label={t("Period from (Kuala Lumpur)")} error={fieldError("period")}><DateTimeSelect id={ids.period} value={d.from} onChange={(v) => { set({ from: v }); }} /></Field>
                <Field label={t("Period to")} hint={t("Up to 366 days")}><DateTimeSelect value={d.to} onChange={(v) => { set({ to: v }); }} /></Field>
              </section>
              <Field label={t("Method")}><Choice value={d.method} onChange={(v) => { set({ method: v }); }} options={[{ id: "demo_fixed" as const, label: `demo_fixed · ${t("entered value")}` }, { id: "demo_period_comparison" as const, label: `demo_period_comparison · ${t("measured")}` }]} /></Field>
              {d.method === DEMO_FIXED ? <div className="max-w-xs"><Field label={t("Baseline energy (kWh)")} error={fieldError("baselineKWh")} hint={t("Fictional demo value · ≥ 0 · not used for the demo period comparison")}><Input id={ids.baselineKWh} type="number" min="0" step="0.1" value={d.baselineKWh} onChange={(e) => { set({ baselineKWh: e.target.value }); }} /></Field></div> : <p className="text-xs text-muted-foreground">{t("Measured from the period’s readings when saved; no value is entered.")}</p>}
              <Field label={t("Boundary")} error={fieldError("boundaryId")}><div id={ids.boundaryId}><Choice value={d.boundaryId} onChange={(v) => { set({ boundaryId: v }); }} options={[{ id: "ac_input_electricity" as const, label: t("AC input electricity") }, { id: "whole_building_electricity" as const, label: t("Whole-building electricity") }]} /></div></Field>
              <Field label={t("Boundary description")} error={fieldError("boundary")} hint={t("1–500 characters")}><Input id={ids.boundary} value={d.boundary} maxLength={500} onChange={(e) => { set({ boundary: e.target.value }); }} placeholder={t("AC unit input electricity only")} /></Field>
              <Field label={t("Assumptions")} error={fieldError("assumptions")} hint={t("1–2000 characters")}><Textarea id={ids.assumptions} value={d.assumptions} maxLength={2000} onChange={(e) => { set({ assumptions: e.target.value }); }} /></Field>
              <Field label={t("Source")} error={fieldError("source")}><Input id={ids.source} value={d.source} maxLength={500} onChange={(e) => { set({ source: e.target.value }); }} placeholder={t("Demo source (fictional)")} /></Field>
              {sel && <Banner tone="warn">{t("MRV reports that already reference version {v} keep version {v}. Results are labelled “adjusted” only if an adjustment model exists — none does in this demo.", { v: sel.version })}</Banner>}
              {live.canWrite && <div className="flex justify-end gap-2">{creating ? <Btn onClick={() => { setCreating(false); }}>{t("Cancel")}</Btn> : <Btn disabled={pending} onClick={() => { setD(baselineDraft(sel?.b)); setTried(false); }}>{t("Discard")}</Btn>}{sel && <Btn variant="danger" disabled={pending} onClick={() => { setRemoving(true); }}>{t("Delete baseline…")}</Btn>}<Btn variant="primary" disabled={pending} onClick={save}>{sel ? t("Save as version {n}", { n: sel.version + 1 }) : t("Create baseline")}</Btn></div>}
            </div>
          </Card>
        ) : <Card title={t("Baseline")}><EmptyState title={t("Nothing selected")}>{t("Choose a baseline or create one.")}</EmptyState></Card>}
      </div>
      {removing && sel && <DeleteDialog title={t("Delete baseline")} text={t("Delete baseline {id}? Every version goes. A baseline an MRV report cites cannot be deleted.", { id: sel.id.slice(0, 8) })} pending={pending}
        onClose={() => { setRemoving(false); }} onConfirm={(reason) => { run(() => deleteBaseline(sel.id, sel.version, reason), t("Baseline deleted"), () => { setRemoving(false); nav({ baselineId: null }); }); }} />}
    </>
  );
}
