"use client";

import { useState } from "react";
import { Badge, Banner, Btn, Card, Check, DemoBadge, EmptyState, Field, Input, ListRow, Modal, Page, Select, SummaryList, Tabs, Textarea, Timeline } from "@ac/web/components/ui";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klStamp, type BaselineRow } from "@ac/web/lib/energy";
import {
  factorErrors, factorInput, mrvConditions, mrvDraftErrors, mrvDraftOf, mrvView, versionKey,
  type ApiFactor, type ApiMRVReport, type FactorDraft, type MRVDraft, type MRVView as Figures, type ReportRow,
} from "@ac/web/lib/mrv";
import { previewReport, recordReview, saveFactor, saveReportDraft } from "../actions";

type UnitOption = { id: string; label: string; organizationId: string };
export type MRVLive = {
  tab: "reports" | "factors"; canWrite: boolean; canReview: boolean; canFactors: boolean;
  orgs: { id: string; name: string }[]; units: UnitOption[]; baselines: BaselineRow[]; factors: ApiFactor[];
  reports?: {
    rows: ReportRow[]; scope: { organizationId?: string; unitId?: string; status?: string; from: string; to: string };
    selected?: { report: ApiMRVReport; latestVersion: number; versions: { version: number; status: string }[]; view: Figures };
  };
};

/** The digital MRV demo workspace (FR-A14) in API mode: filters, the selected report and version live in the URL;
 * previews, drafts, demo reviews and factor versions are Server Actions. Every figure stays "Demo — unverified". */
export function MRVView({ live }: { live: MRVLive }) {
  const nav = useUrlPatch();
  return (
    <Page>
      <Tabs value={live.tab} onChange={(t) => nav({ tab: t === "reports" ? null : t, reportId: null, reportVersion: null })} tabs={[{ id: "reports", label: "Reports", count: live.reports?.rows.length }, { id: "factors", label: "Emission factors", count: live.factors.length }]} />
      {live.tab === "reports" && live.reports ? <Reports live={live} r={live.reports} /> : <Factors live={live} />}
    </Page>
  );
}

function FiguresGrid({ v }: { v: Figures }) {
  if (v.incomplete) return <Banner tone="warn">Calculation incomplete — a factor, the boundary or full coverage is missing, so nothing is shown as zero.</Banner>;
  return (
    <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
      {[["Electricity used", v.electricity, `coverage ${v.summary.coverage}`], ["Scope 2 emissions", v.emissions, "electricity × the factor version"], ["Energy vs baseline", v.energyVsBaseline, `${v.energyPercent} · not adjusted`], ["Emissions vs baseline", v.emissionsVsBaseline, "shown separately from energy"]].map(([a, b, c]) => (
        <div key={a} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{a}</div><b>{b}</b><div className="text-[10px] text-muted">{c}</div></div>
      ))}
    </div>
  );
}

function Reports({ live, r }: { live: MRVLive; r: NonNullable<MRVLive["reports"]> }) {
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
  const errors = d ? mrvDraftErrors(d) : {};
  const setD = (patch: Partial<MRVDraft>) => { setPreview(null); setDraft((x) => (x ? { ...x, d: { ...x.d, ...patch } } : x)); };
  const doPreview = () => {
    setTried(true);
    if (!d || Object.keys(errors).length > 0) return;
    run(() => previewReport(mrvConditions(d)), "Preview ready — nothing is saved", (p) => setPreview(mrvView(p, live.orgs, live.units)));
  };
  const doSave = () => {
    if (!draft || !d || Object.keys(errors).length > 0) return;
    run(() => saveReportDraft(mrvConditions(d), draft.evidenceIds, draft.id, draft.version), (v) => (draft.id ? `Saved as version ${v.version} (draft)` : "Report draft created"), (v) => { setDraft(null); nav({ reportId: v.id, reportVersion: null }); });
  };
  const doReview = () => {
    if (!sel || comment.trim().length < 1) return;
    run(() => recordReview(sel.report.id, sel.report.version, comment.trim()), (v) => `Demo review recorded — version ${v}`, () => { setComment(""); nav({ reportVersion: null }); });
  };
  const orgUnits = d ? live.units.filter((u) => u.organizationId === d.organizationId) : [];
  return (
    <>
      <div className="flex flex-wrap items-end gap-3">
        <Field label="Customer"><Select value={r.scope.organizationId ?? ""} onChange={(e) => nav({ organizationId: e.target.value || null, unitId: null, reportId: null, reportVersion: null })}><option value="">All customers</option>{live.orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select></Field>
        <Field label="Unit"><Select value={r.scope.unitId ?? ""} onChange={(e) => nav({ unitId: e.target.value || null, reportId: null, reportVersion: null })}><option value="">All units</option>{scopeUnits.map((u) => <option key={u.id} value={u.id}>{u.label}</option>)}</Select></Field>
        <Field label="Status"><Select value={r.scope.status ?? ""} onChange={(e) => nav({ status: e.target.value || null, reportId: null, reportVersion: null })}><option value="">All</option><option value="draft">Draft</option><option value="demo_reviewed">Demo reviewed</option></Select></Field>
        <Field label="Period start from"><Input type="datetime-local" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label="to"><Input type="datetime-local" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
        <Btn size="sm" disabled={from === r.scope.from && to === r.scope.to} onClick={() => nav({ from: from || null, to: to || null, reportId: null, reportVersion: null })}>Apply</Btn>
      </div>
      <div className="split-rev">
        <Card title="Scope 2 reports" action={live.canWrite && <Btn size="sm" variant="primary" onClick={() => openDraft()}>+ New</Btn>} className="self-start">
          {r.rows.length === 0 ? <EmptyState title="No reports">No report matches these filters.</EmptyState> : <div className="flex flex-col gap-2">{r.rows.map((x) => <ListRow key={x.id} selected={sel?.report.id === x.id} onClick={() => nav({ reportId: x.id, reportVersion: null })}><div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-2"><b className="text-[13px]">{x.id.slice(0, 8)} · v{x.version}</b><Badge tone={x.status === "demo_reviewed" ? "ok" : "muted"}>{x.status === "demo_reviewed" ? "Demo reviewed" : "Draft"}</Badge></div><div className="text-[11px] text-muted">{x.org} · {x.units} · {x.period}</div><div className={`text-[11px] ${x.incomplete ? "text-warn" : "text-muted"}`}>{x.result}</div></div></ListRow>)}</div>}
          <p className="mt-3 text-[11px] text-muted">Every report is a demo — “Demo — unverified”. A demo review is not external certification.</p>
        </Card>
        {!sel ? <Card title="Report"><EmptyState title="Nothing selected">Choose a report or create one.</EmptyState></Card> : (
          <div className="flex min-w-0 flex-col gap-4">
            <Card title="Scope 2 electricity — demo summary" sub={`${sel.report.id.slice(0, 8)} · ${sel.report.status === "demo_reviewed" ? "demo reviewed" : "draft"}`} action={<div className="flex items-center gap-2"><DemoBadge /><Select aria-label="Version" className="w-auto" value={sel.report.version} onChange={(e) => nav({ reportVersion: Number(e.target.value) === sel.latestVersion ? null : e.target.value })}>{sel.versions.map((v) => <option key={v.version} value={v.version}>Version {v.version}{v.version === sel.latestVersion ? " (latest)" : ""} · {v.status}</option>)}</Select>{live.canWrite && isLatest && <Btn size="sm" onClick={() => openDraft(sel.report)}>New version…</Btn>}</div>}>
              <FiguresGrid v={sel.view} />
              {!isLatest && <p className="mt-2 text-[11px] text-muted">An earlier version — read only. Its conditions and results never change.</p>}
            </Card>
            <Card title="Calculation conditions (snapshot)"><SummaryList items={sel.view.conditions} />{sel.view.summary.warnings.length > 0 && <div className="mt-2 flex flex-col gap-1">{sel.view.summary.warnings.map((w) => <Banner key={w} tone="warn">{w}</Banner>)}</div>}<p className="mt-2 text-[11px] text-muted">Snapshots are stored with the version; later baseline or factor versions do not change it.</p></Card>
            <Card title="Evidence">{sel.report.evidenceIds.length === 0 ? <p className="text-xs text-muted">No evidence attached.</p> : <ul className="list-disc pl-5 text-[13px]">{sel.report.evidenceIds.map((e) => <li key={e} className="font-mono text-xs">{e}</li>)}</ul>}</Card>
            <Card title="Demo review">
              {sel.report.reviewHistory.length > 0 && <Timeline items={sel.report.reviewHistory.map((h) => ({ time: klStamp(h.at), title: `“${h.comment}”`, detail: `version ${h.reportVersion} · ${h.userId.slice(0, 8)}` }))} />}
              {sel.report.status === "demo_reviewed" ? <p className="text-xs text-muted">This version is demo reviewed. Sending the same review again returns the existing result.</p>
                : !isLatest ? <p className="text-xs text-muted">Only the latest version can be reviewed.</p>
                : sel.view.incomplete ? <p className="text-xs text-muted">An incomplete calculation cannot be reviewed.</p>
                : live.canReview && <><Field label="Review comment"><Textarea value={comment} maxLength={1000} onChange={(e) => setComment(e.target.value)} /></Field><div className="mt-2"><Btn variant="primary" disabled={pending || !comment.trim()} onClick={doReview}>Submit demo review</Btn></div></>}
            </Card>
          </div>
        )}
      </div>
      <Modal open={!!draft} onClose={() => setDraft(null)} title={draft?.id ? "New report version" : "New Scope 2 report"} wide footer={<><Btn onClick={() => setDraft(null)}>Cancel</Btn><Btn disabled={pending} onClick={doPreview}>Preview</Btn><Btn variant="primary" disabled={pending || !preview} onClick={doSave}>Save draft</Btn></>}>
        {d && <>
          <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
            <Field label="Customer organization" error={tried ? errors.organizationId : undefined}><Select value={d.organizationId} onChange={(e) => setD({ organizationId: e.target.value, unitIds: [] })}><option value="">Select…</option>{live.orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select></Field>
            <Field label="Start (Kuala Lumpur)" error={tried ? errors.period : undefined}><Input type="datetime-local" value={d.from} onChange={(e) => setD({ from: e.target.value })} /></Field>
            <Field label="End"><Input type="datetime-local" value={d.to} onChange={(e) => setD({ to: e.target.value })} /></Field>
            <Field label="Baseline version" error={tried ? errors.baseline : undefined}><Select value={d.baseline} onChange={(e) => setD({ baseline: e.target.value })}><option value="">Select…</option>{live.baselines.map((b) => <option key={b.id} value={versionKey(b.id, b.version)}>{b.id.slice(0, 8)} v{b.version} · {b.value} · {b.units}</option>)}</Select></Field>
            <Field label="Emission factor version" error={tried ? errors.factor : undefined}><Select value={d.factor} onChange={(e) => setD({ factor: e.target.value })}><option value="">Select…</option>{live.factors.map((f) => <option key={f.id} value={versionKey(f.id, f.version)}>{f.region} {f.year} v{f.version} · {f.kgCO2ePerKWh}</option>)}</Select></Field>
            <Field label="Boundary"><Select value={d.boundaryId} onChange={(e) => setD({ boundaryId: e.target.value as MRVDraft["boundaryId"] })}><option value="ac_input_electricity">ac_input_electricity</option><option value="whole_building_electricity">whole_building_electricity</option></Select></Field>
          </div>
          <Field label="Boundary description" error={tried ? errors.boundary : undefined}><Input value={d.boundary} maxLength={500} onChange={(e) => setD({ boundary: e.target.value })} placeholder="AC unit input electricity only" /></Field>
          <div><p className="mb-1 text-[13px] font-semibold">Units</p>{!d.organizationId ? <p className="text-xs text-muted">Choose the organization first.</p> : <div className="flex flex-wrap gap-x-5 gap-y-1.5 text-[13px]">{orgUnits.map((u) => <Check key={u.id} label={u.label} checked={d.unitIds.includes(u.id)} onChange={(on) => setD({ unitIds: on ? [...d.unitIds, u.id] : d.unitIds.filter((x) => x !== u.id) })} />)}</div>}{tried && errors.unitIds && <p className="mt-1 text-xs text-crit">{errors.unitIds}</p>}</div>
          <p className="text-[11px] text-muted">Evidence: {draft?.evidenceIds.length ? `${draft.evidenceIds.length} attachment(s) carried over from the previous version` : "none — no operation lists attachments outside a work report yet"}.</p>
          {preview && <div className="flex flex-col gap-2 rounded-xl border border-line p-3"><div className="flex items-center gap-2"><b className="text-[13px]">Preview · not saved</b><DemoBadge /><span className="text-[11px] text-muted">Demo — unverified</span></div><FiguresGrid v={preview} /></div>}
          {!preview && <p className="text-[11px] text-muted">Preview first: saving stores the conditions and the result computed at save time.</p>}
        </>}
      </Modal>
    </>
  );
}

function Factors({ live }: { live: MRVLive }) {
  const [pending, run] = useAction();
  const [selId, setSelId] = useState<string | "new">(live.factors[0]?.id ?? "new");
  const sel = selId === "new" ? null : live.factors.find((f) => f.id === selId) ?? live.factors[0] ?? null;
  const blank: FactorDraft = { region: "", year: "2026", kgCO2ePerKWh: "", source: "Demo (fictional)" };
  const key = sel ? `${sel.id}:${sel.version}` : "new";
  const [source, setSource] = useState(key);
  const [d, setD] = useState<FactorDraft>(sel ? { region: sel.region, year: String(sel.year), kgCO2ePerKWh: String(sel.kgCO2ePerKWh), source: sel.source } : blank);
  const [tried, setTried] = useState(false);
  if (source !== key) {
    setSource(key);
    setD(sel ? { region: sel.region, year: String(sel.year), kgCO2ePerKWh: String(sel.kgCO2ePerKWh), source: sel.source } : blank);
    setTried(false);
  }
  const errors = factorErrors(d);
  const save = () => {
    setTried(true);
    if (Object.keys(errors).length > 0) return;
    run(() => saveFactor(factorInput(d, sel?.id), sel?.version), (f) => (sel ? `Saved as factor version ${f.version}` : "Factor created"), (f) => setSelId(f.id));
  };
  const err = (k: string) => (tried ? errors[k] : undefined);
  return (
    <div className="split-rev">
      <Card title="Emission factors" action={live.canFactors && <Btn size="sm" onClick={() => setSelId("new")}>+ New</Btn>} className="self-start">
        {live.factors.length === 0 ? <EmptyState title="No factors">Add a demo factor.</EmptyState> : <div className="flex flex-col gap-2">{live.factors.map((f) => <ListRow key={f.id} selected={sel?.id === f.id} onClick={() => setSelId(f.id)}><div className="min-w-0"><b className="text-[13px]">{f.region} · {f.year}</b><div className="text-[11px] text-muted">v{f.version} · {f.kgCO2ePerKWh} kgCO₂e/kWh</div></div></ListRow>)}</div>}
        <p className="mt-3 text-[11px] text-muted">Demo factors only — not real regional factors. One current factor per region and year.</p>
      </Card>
      <Card title={sel ? `${sel.region} · ${sel.year}` : "New factor"} sub={sel ? `Version ${sel.version} · saving creates version ${sel.version + 1}` : "Saving creates version 1"}>
        <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
          <Field label="Region" error={err("region")}><Input value={d.region} maxLength={120} onChange={(e) => setD({ ...d, region: e.target.value })} /></Field>
          <Field label="Year" error={err("year")}><Input type="number" value={d.year} onChange={(e) => setD({ ...d, year: e.target.value })} /></Field>
          <Field label="kgCO₂e/kWh (fixed unit)" error={err("kgCO2ePerKWh")}><Input type="number" step="0.001" value={d.kgCO2ePerKWh} onChange={(e) => setD({ ...d, kgCO2ePerKWh: e.target.value })} /></Field>
        </div>
        <div className="mt-3"><Field label="Source (state that it is a demo)" error={err("source")}><Input value={d.source} maxLength={500} onChange={(e) => setD({ ...d, source: e.target.value })} /></Field></div>
        {sel && <p className="mt-3 rounded-xl bg-surface2 p-3 text-xs">Existing reports keep the factor version they reference.</p>}
        {live.canFactors && <div className="mt-3 flex justify-end gap-2">{!sel && live.factors[0] && <Btn onClick={() => setSelId(live.factors[0].id)}>Cancel</Btn>}<Btn variant="primary" disabled={pending} onClick={save}>{sel ? `Save as v${sel.version + 1}` : "Create factor"}</Btn></div>}
      </Card>
    </div>
  );
}
