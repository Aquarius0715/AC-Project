"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DemoBadge, EmptyState, Field, Input, ListRow, Modal, Page, Select, SummaryList, Tabs, Textarea, Timeline } from "@ac/web/components/ui";
import { useI18n, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { BaselineRow } from "@ac/web/lib/energy";
import {
  factorErrors, factorInput, mrvConditions, mrvDraftErrors, mrvDraftOf, mrvView, statusWord, versionKey,
  type ApiFactor, type ApiMRVReport, type FactorDraft, type MRVDraft, type MRVView as Figures, type ReportRow,
} from "@ac/web/lib/mrv";
import { previewReport, recordReview, saveFactor, saveReportDraft } from "../actions";

type UnitOption = { id: string; label: string; organizationId: string };
export type MRVLive = {
  tab: "reports" | "factors"; canWrite: boolean; canReview: boolean; canFactors: boolean;
  orgs: { id: string; name: string }[]; units: UnitOption[]; baselines: BaselineRow[]; factors: ApiFactor[];
  reports?: {
    rows: ReportRow[]; scope: { organizationId?: string; unitId?: string; status?: string; from: string; to: string };
    selected?: { report: ApiMRVReport; latestVersion: number; versions: { version: number; status: string }[]; view: Figures; review: { time: string; title: string; detail: string }[] };
  };
};

/** The digital MRV demo workspace (FR-A14) in API mode: filters, the selected report and version live in the URL;
 * previews, drafts, demo reviews and factor versions are Server Actions. Every figure stays "Demo — unverified". Texts
 * in the display language; report periods are typed and shown in Kuala Lumpur time, which the labels name (IR297). */
export function MRVView({ live }: { live: MRVLive }) {
  const t = useT();
  const nav = useUrlPatch();
  return (
    <Page>
      <Tabs value={live.tab} onChange={(id) => nav({ tab: id === "reports" ? null : id, reportId: null, reportVersion: null })} tabs={[{ id: "reports", label: t("Reports"), count: live.reports?.rows.length }, { id: "factors", label: t("Emission factors"), count: live.factors.length }]} />
      {live.tab === "reports" && live.reports ? <Reports live={live} r={live.reports} /> : <Factors live={live} />}
    </Page>
  );
}

function FiguresGrid({ v }: { v: Figures }) {
  const t = useT();
  if (v.incomplete) return <Banner tone="warn">{t("Calculation incomplete — a factor, the boundary or full coverage is missing, so nothing is shown as zero.")}</Banner>;
  const tiles: [string, string, string][] = [
    [t("Electricity used"), v.electricity, t("coverage {pct}", { pct: v.summary.coverage })], [t("Scope 2 emissions"), v.emissions, t("electricity × the factor version")],
    [t("Energy vs baseline"), v.energyVsBaseline, t("{percent} · not adjusted", { percent: v.energyPercent })], [t("Emissions vs baseline"), v.emissionsVsBaseline, t("shown separately from energy")],
  ];
  return (
    <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
      {tiles.map(([a, b, c]) => (
        <div key={a} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{a}</div><b>{b}</b><div className="text-[10px] text-muted">{c}</div></div>
      ))}
    </div>
  );
}

function Reports({ live, r }: { live: MRVLive; r: NonNullable<MRVLive["reports"]> }) {
  const i = useI18n(), { t } = i;
  const nav = useUrlPatch();
  const [pending, run] = useAction();
  const sel = r.selected;
  const [comment, setComment] = useState("");
  const [draft, setDraft] = useState<null | { id?: string; version?: number; d: MRVDraft; evidenceIds: string[] }>(null);
  const [preview, setPreview] = useState<Figures | null>(null);
  const [tried, setTried] = useState(false);
  const [from, setFrom] = useState(r.scope.from);
  const [to, setTo] = useState(r.scope.to);
  const isLatest = sel ? sel.report.version === sel.latestVersion : false;
  const scopeUnits = live.units.filter((u) => !r.scope.organizationId || u.organizationId === r.scope.organizationId);
  const openDraft = (fromReport?: ApiMRVReport) => {
    setPreview(null);
    setTried(false);
    setDraft(fromReport ? { id: fromReport.id, version: sel?.latestVersion, d: mrvDraftOf(fromReport.conditions), evidenceIds: fromReport.evidenceIds } : { d: mrvDraftOf(), evidenceIds: [] });
  };
  const d = draft?.d;
  const errors = d ? mrvDraftErrors(d, t) : {};
  const setD = (patch: Partial<MRVDraft>) => { setPreview(null); setDraft((x) => (x ? { ...x, d: { ...x.d, ...patch } } : x)); };
  const doPreview = () => {
    setTried(true);
    if (!d || Object.keys(errors).length > 0) return;
    run(() => previewReport(mrvConditions(d)), t("Preview ready — nothing is saved"), (p) => setPreview(mrvView(p, live.orgs, live.units, i)));
  };
  const doSave = () => {
    if (!draft || !d || Object.keys(errors).length > 0) return;
    run(() => saveReportDraft(mrvConditions(d), draft.evidenceIds, draft.id, draft.version), (v) => (draft.id ? t("Saved as version {v} (draft)", { v: v.version }) : t("Report draft created")), (v) => { setDraft(null); nav({ reportId: v.id, reportVersion: null }); });
  };
  const doReview = () => {
    if (!sel || comment.trim().length < 1) return;
    run(() => recordReview(sel.report.id, sel.report.version, comment.trim()), (v) => t("Demo review recorded — version {v}", { v }), () => { setComment(""); nav({ reportVersion: null }); });
  };
  const orgUnits = d ? live.units.filter((u) => u.organizationId === d.organizationId) : [];
  const evidence = draft?.evidenceIds.length ?? 0;
  return (
    <>
      <div className="flex flex-wrap items-end gap-3">
        <Field label={t("Customer")}><Select value={r.scope.organizationId ?? ""} onChange={(e) => nav({ organizationId: e.target.value || null, unitId: null, reportId: null, reportVersion: null })}><option value="">{t("All customers")}</option>{live.orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select></Field>
        <Field label={t("Unit")}><Select value={r.scope.unitId ?? ""} onChange={(e) => nav({ unitId: e.target.value || null, reportId: null, reportVersion: null })}><option value="">{t("All units")}</option>{scopeUnits.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <Field label={t("Status")}><Select value={r.scope.status ?? ""} onChange={(e) => nav({ status: e.target.value || null, reportId: null, reportVersion: null })}><option value="">{t("All")}</option><option value="draft">{t("Draft")}</option><option value="demo_reviewed">{t("Demo reviewed")}</option></Select></Field>
        <Field label={t("Period starts from (Kuala Lumpur)")}><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label={t("Period starts before")}><Input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
        <Btn size="sm" disabled={from === r.scope.from && to === r.scope.to} onClick={() => nav({ from: from || null, to: to || null, reportId: null, reportVersion: null })}>{t("Apply")}</Btn>
      </div>
      <div className="split-rev">
        <Card title={t("Scope 2 reports")} action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => openDraft()}>{t("+ New")}</Btn>} className="self-start">
          {r.rows.length === 0 ? <EmptyState title={t("No reports")}>{t("No report matches these filters.")}</EmptyState> : <div className="flex flex-col gap-2">{r.rows.map((x) => <ListRow key={x.id} selected={sel?.report.id === x.id} onClick={() => nav({ reportId: x.id, reportVersion: null })}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.id.slice(0, 8)} · v{x.version}</b><Badge tone={x.status === "demo_reviewed" ? "ok" : "muted"}>{x.status === "demo_reviewed" ? t("Demo reviewed") : t("Draft")}</Badge></div><div className="text-[11px] text-muted">{x.org} · {x.units} · {x.period}</div><div className={`text-[11px] ${x.incomplete ? "text-warn" : "text-muted"}`}>{x.result}</div></div></ListRow>)}</div>}
          <p className="mt-3 text-[11px] text-muted">{t("Every report is a demo — “Demo — unverified”. A demo review is not external certification.")}</p>
        </Card>
        {!sel ? <Card title={t("Report")}><EmptyState title={t("Nothing selected")}>{t("Choose a report or create one.")}</EmptyState></Card> : (
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={t("Scope 2 electricity — demo summary")} sub={`${sel.report.id.slice(0, 8)} · ${statusWord(sel.report.status, t)}`} action={<div className="flex items-center gap-2"><DemoBadge /><Select aria-label={t("Version")} className="w-auto" value={sel.report.version} onChange={(e) => nav({ reportVersion: Number(e.target.value) === sel.latestVersion ? null : e.target.value })}>{sel.versions.map((v) => <option key={v.version} value={v.version}>{t(v.version === sel.latestVersion ? "Version {n} (latest) · {status}" : "Version {n} · {status}", { n: v.version, status: statusWord(v.status, t) })}</option>)}</Select>{live.canWrite && isLatest && <Btn size="sm" onClick={() => openDraft(sel.report)}>{t("New version…")}</Btn>}</div>}>
              <FiguresGrid v={sel.view} />
              {!isLatest && <p className="mt-2 text-[11px] text-muted">{t("An earlier version — read only. Its conditions and results never change.")}</p>}
            </Card>
            <Card title={t("Calculation conditions (snapshot)")}><SummaryList items={sel.view.conditions} />{sel.view.summary.warnings.length > 0 && <div className="mt-2 flex flex-col gap-1">{sel.view.summary.warnings.map((w) => <Banner key={w} tone="warn">{w}</Banner>)}</div>}<p className="mt-2 text-[11px] text-muted">{t("Snapshots are stored with the version; later baseline or factor versions do not change it.")}</p></Card>
            <Card title={t("Evidence")}>{sel.report.evidenceIds.length === 0 ? <p className="text-xs text-muted">{t("No evidence attached.")}</p> : <ul className="list-disc pl-5 text-[13px]">{sel.report.evidenceIds.map((e) => <li key={e} className="font-mono text-xs">{e}</li>)}</ul>}</Card>
            <Card title={t("Demo review")}>
              {sel.review.length > 0 && <Timeline items={sel.review} />}
              {sel.report.status === "demo_reviewed" ? <p className="text-xs text-muted">{t("This version is demo reviewed. Sending the same review again returns the existing result.")}</p>
                : !isLatest ? <p className="text-xs text-muted">{t("Only the latest version can be reviewed.")}</p>
                : sel.view.incomplete ? <p className="text-xs text-muted">{t("An incomplete calculation cannot be reviewed.")}</p>
                : live.canReview && <><Field label={t("Review comment")}><Textarea value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} /></Field><div className="mt-2"><Btn variant="primary" disabled={pending || !comment.trim()} onClick={doReview}>{t("Submit demo review")}</Btn></div></>}
            </Card>
          </div>
        )}
      </div>
      <Modal open={!!draft} onClose={() => setDraft(null)} title={draft?.id ? t("New report version") : t("New Scope 2 report")} wide footer={<><Btn onClick={() => setDraft(null)}>{t("Cancel")}</Btn><Btn disabled={pending} onClick={doPreview}>{t("Preview")}</Btn><Btn variant="primary" disabled={pending || !preview} onClick={doSave}>{t("Save draft")}</Btn></>}>
        {d && <>
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <Field label={t("Customer organization")} error={tried ? errors.organizationId : undefined}><Select value={d.organizationId} onChange={(e) => setD({ organizationId: e.target.value, unitIds: [] })}><option value="">{t("Select…")}</option>{live.orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select></Field>
            <Field label={t("Start (Kuala Lumpur)")} error={tried ? errors.period : undefined}><Input type="datetime-local" value={d.from} onChange={(e) => setD({ from: e.target.value })} /></Field>
            <Field label={t("End")}><Input type="datetime-local" value={d.to} onChange={(e) => setD({ to: e.target.value })} /></Field>
            <Field label={t("Baseline version")} error={tried ? errors.baseline : undefined}><Select value={d.baseline} onChange={(e) => setD({ baseline: e.target.value })}><option value="">{t("Select…")}</option>{live.baselines.map((b) => <option key={b.id} value={versionKey(b.id, b.version)}>{b.id.slice(0, 8)} v{b.version} · {b.value} · {b.units}</option>)}</Select></Field>
            <Field label={t("Emission factor version")} error={tried ? errors.factor : undefined}><Select value={d.factor} onChange={(e) => setD({ factor: e.target.value })}><option value="">{t("Select…")}</option>{live.factors.map((f) => <option key={f.id} value={versionKey(f.id, f.version)}>{f.region} {f.year} v{f.version} · {f.kgCO2ePerKWh}</option>)}</Select></Field>
            <Field label={t("Boundary")}><Select value={d.boundaryId} onChange={(e) => setD({ boundaryId: e.target.value as MRVDraft["boundaryId"] })}><option value="ac_input_electricity">ac_input_electricity</option><option value="whole_building_electricity">whole_building_electricity</option></Select></Field>
          </div>
          <Field label={t("Boundary description")} error={tried ? errors.boundary : undefined}><Input value={d.boundary} maxLength={500} onChange={(e) => setD({ boundary: e.target.value })} placeholder={t("AC unit input electricity only")} /></Field>
          <div><p className="mb-1 text-[13px] font-semibold">{t("Units")}</p>{!d.organizationId ? <p className="text-xs text-muted">{t("Choose the organization first.")}</p> : <div className="flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{orgUnits.map((u) => <Check key={u.id} label={u.label} checked={d.unitIds.includes(u.id)} onChange={(on) => setD({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) })} />)}</div>}{tried && errors.unitIds && <p className="mt-1 text-xs text-crit">{errors.unitIds}</p>}</div>
          <p className="text-[11px] text-muted">{evidence === 0 ? t("Evidence: none — no operation lists attachments outside a work report yet.") : t(evidence === 1 ? "Evidence: 1 attachment carried over from the previous version." : "Evidence: {n} attachments carried over from the previous version.", { n: evidence })}</p>
          {preview && <div className="flex flex-col gap-2 rounded-xl border border-line p-3"><div className="flex items-center gap-2"><b className="text-[13px]">{t("Preview · not saved")}</b><DemoBadge /><span className="text-[11px] text-muted">{t("Demo — unverified")}</span></div><FiguresGrid v={preview} /></div>}
          {!preview && <p className="text-[11px] text-muted">{t("Preview first: saving stores the conditions and the result computed at save time.")}</p>}
        </>}
      </Modal>
    </>
  );
}

function Factors({ live }: { live: MRVLive }) {
  const t = useT();
  const [pending, run] = useAction();
  const [selId, setSelId] = useState<string | "new">(live.factors[0]?.id ?? "new");
  const sel = selId === "new" ? null : live.factors.find((f) => f.id === selId) ?? live.factors[0] ?? null;
  const blank: FactorDraft = { region: "", year: "2026", kgCO2ePerKWh: "", source: t("Demo (fictional)") };
  const key = sel ? `${sel.id}:${sel.version}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<FactorDraft>(sel ? { region: sel.region, year: String(sel.year), kgCO2ePerKWh: String(sel.kgCO2ePerKWh), source: sel.source } : blank);
  const [tried, setTried] = useState(false);
  if (source !== key) {
    setSource(key);
    setD(sel ? { region: sel.region, year: String(sel.year), kgCO2ePerKWh: String(sel.kgCO2ePerKWh), source: sel.source } : blank);
    setTried(false);
  }
  const errors = factorErrors(d, t);
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveFactor(factorInput(d, sel?.id), sel?.version), (f) => (sel ? t("Saved as factor version {v}", { v: f.version }) : t("Factor created")), (f) => setSelId(f.id));
  };
  const err = (k: string) => (tried ? errors[k] : undefined);
  return (
    <div className="split-rev">
      <Card title={t("Emission factors")} action={live.canFactors && <Btn size="sm" onClick={() => setSelId("new")}>{t("+ New")}</Btn>} className="self-start">
        {live.factors.length === 0 ? <EmptyState title={t("No factors")}>{t("Add a demo factor.")}</EmptyState> : <div className="flex flex-col gap-2">{live.factors.map((f) => <ListRow key={f.id} selected={sel?.id === f.id} onClick={() => setSelId(f.id)}><div className="min-w-0"><b className="text-[13px]">{f.region} · {f.year}</b><div className="text-[11px] text-muted">v{f.version} · {f.kgCO2ePerKWh} kgCO₂e/kWh</div></div></ListRow>)}</div>}
        <p className="mt-3 text-[11px] text-muted">{t("Demo factors only — not real regional factors. One current factor per region and year.")}</p>
      </Card>
      <Card title={sel ? `${sel.region} · ${sel.year}` : t("New factor")} sub={sel ? t("Version {v} · saving creates version {next}", { v: sel.version, next: sel.version + 1 }) : t("Saving creates version 1")}>
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
          <Field label={t("Region")} error={err("region")}><Input value={d.region} maxLength={120} onChange={(e) => setD({ ...d, region: e.target.value })} /></Field>
          <Field label={t("Year")} error={err("year")}><Input type="number" value={d.year} onChange={(e) => setD({ ...d, year: e.target.value })} /></Field>
          <Field label={t("kgCO₂e/kWh (fixed unit)")} error={err("kgCO2ePerKWh")}><Input type="number" step="0.001" value={d.kgCO2ePerKWh} onChange={(e) => setD({ ...d, kgCO2ePerKWh: e.target.value })} /></Field>
        </div>
        <div className="mt-3"><Field label={t("Source (state that it is a demo)")} error={err("source")}><Input value={d.source} maxLength={500} onChange={(e) => setD({ ...d, source: e.target.value })} /></Field></div>
        {sel && <p className="mt-3 rounded-xl bg-surface2 p-3 text-xs">{t("Existing reports keep the factor version they reference.")}</p>}
        {live.canFactors && <div className="mt-3 flex justify-end gap-2">{!sel && live.factors[0] && <Btn onClick={() => setSelId(live.factors[0].id)}>{t("Cancel")}</Btn>}<Btn variant="primary" disabled={pending} onClick={save}>{sel ? t("Save as v{n}", { n: sel.version + 1 }) : t("Create factor")}</Btn></div>}
      </Card>
    </div>
  );
}
