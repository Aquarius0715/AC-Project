"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Banner, Btn, Card, EmptyState, Field, Input, ListRow, Modal, Page, Select } from "@ac/web/components/ui";
import { JobStatusBadge } from "@ac/web/components/JobBits";
import { useAction } from "@ac/web/lib/useAction";
import { anchorOf, cadence, everyText, fromKlInput, generateConflict, klDate, klInput, nextBox, nextOccurrence, planErrors, planRefusal } from "@ac/web/lib/adminPlans";
import type { JobStatus } from "@ac/web/lib/jobs";
import { generateJob, savePlan } from "../actions";
import type { PlansLive } from "../_lib/load";
import { JobsTabs, ScopeBar } from "./jobs-header";

type Detail = NonNullable<PlansLive["detail"]>;
/** What the last write on a plan left to show: saved, generated (the box names the job), a refused Generate job (the
 * CONFLICT banner reads the refreshed plan) or another refusal. Kept here because a write re-keys the detail. */
type Notice = { planId: string } & ({ kind: "saved"; text: string } | { kind: "generated"; occurrenceAt: string; jobId: string } | { kind: "conflict"; attempted: string } | { kind: "error"; text: string });
const MONTHS = Array.from({ length: 12 }, (_, i) => i + 1);

/** HQ maintenance plans (FR-A06 Plans tab, DD-A06 item 9, D16, Figma Admin 06 283:2 / 360:414) from the Core API. */
export function PlansView({ live }: { live: PlansLive }) {
  const [creating, setCreating] = useState(false);
  const [notice, setNotice] = useState<Notice | null>(null);
  const sel = live.detail;
  const href = (id: string) => `/admin/jobs?${new URLSearchParams({ ...Object.fromEntries(Object.entries(live.q).filter(([, v]) => v) as [string, string][]), tab: "plans", planId: id })}`;
  return (
    <Page className="max-w-[1440px]">
      <JobsTabs tab="plans" counts={live.counts} q={live.q} action={<Btn variant="primary" size="sm" onClick={() => setCreating(true)}>+ New plan</Btn>} />
      <ScopeBar scope={live.scope} q={live.q} text={`${live.plansTotal} plan${live.plansTotal === 1 ? "" : "s"} in scope`} clear="planId" />
      <div className="split-rev">
        <Card title="Recurring plans" sub="next due ↑" className="self-start">
          {live.rows.length === 0 ? <EmptyState title="No plans in this scope">“+ New plan” sets up a recurring visit for a unit.</EmptyState> : (
            <div className="flex flex-col gap-1.5">{live.rows.map((r) => (
              <ListRow key={r.id} selected={sel ? sel.plan.id === r.id : false} href={href(r.id)}>
                <div className="min-w-0 flex-1">
                  <b className="block truncate text-[13px]">{r.unit} · {r.customer}</b>
                  <div className="text-[11px] text-muted">{r.short}</div>
                  <div className="text-[11px]"><span className="text-muted">{r.line}</span> · <b>{r.next}</b></div>
                </div>
              </ListRow>
            ))}</div>
          )}
        </Card>
        {sel ? <PlanDetail key={`${sel.plan.id}:${sel.plan.version}`} d={sel} now={Date.parse(live.now)} notice={notice?.planId === sel.plan.id ? notice : null} setNotice={setNotice} />
          : <Card title="Plan"><p className="text-[13px] text-muted">{"missing" in live ? "That plan no longer exists in your scope." : live.rows.length ? "Pick a plan." : "No plan yet."}</p></Card>}
      </div>
      {creating && <NewPlanModal live={live} onClose={() => setCreating(false)} onCreated={(id) => { setCreating(false); setNotice({ planId: id, kind: "saved", text: "Plan created — Generate job creates the job of its first occurrence." }); }} href={href} />}
    </Page>
  );
}

function PlanDetail({ d, now, notice, setNotice }: { d: Detail; now: number; notice: Notice | null; setNotice: (n: Notice | null) => void }) {
  const p = d.plan;
  const [pending, run] = useAction();
  const [every, setEvery] = useState(p.recurrence.intervalMonths);
  const [next, setNext] = useState(klInput(p.nextDueAt));
  const nextIso = next ? fromKlInput(next) : null;
  const nextChanged = !!nextIso && Date.parse(nextIso) !== Date.parse(p.nextDueAt);
  const dirty = every !== p.recurrence.intervalMonths || nextChanged;
  const errors = planErrors({ unitId: p.unitId, every, nextDueAt: nextChanged ? nextIso : p.nextDueAt }, now);
  const anchor = nextIso ? anchorOf(nextChanged ? null : p, nextChanged ? nextIso : p.nextDueAt) : p.anchorDay;
  const then = nextIso ? nextOccurrence(nextChanged ? nextIso : p.nextDueAt, every, anchor) : null;
  const box = nextBox(p, now, dirty, notice?.kind === "generated" ? notice : null);
  const conflict = notice?.kind === "conflict" ? generateConflict(p, notice.attempted) : null;
  const save = () => {
    if (Object.keys(errors).length) return;
    const nextDueAt = nextChanged ? nextIso! : p.nextDueAt;
    run(() => savePlan({ id: p.id, version: p.version, unitId: p.unitId, intervalMonths: every, nextDueAt }), "Plan saved",
      () => setNotice({ planId: p.id, kind: "saved", text: `Plan saved — ${everyText(every).toLowerCase()}, next due ${klDate(nextDueAt)} (anchor day ${anchor}).` }), (f) => setNotice({ planId: p.id, kind: "error", text: planRefusal(f) }));
  };
  const generate = () => run(() => generateJob(p.id, p.version, p.nextDueAt), "Job generated",
    (v) => setNotice({ planId: p.id, kind: "generated", occurrenceAt: p.nextDueAt, jobId: v.id }),
    (f) => setNotice(f.code === "CONFLICT" ? { planId: p.id, kind: "conflict", attempted: p.nextDueAt } : { planId: p.id, kind: "error", text: planRefusal(f) }));
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {notice?.kind === "saved" && <Banner tone="ok">{notice.text}</Banner>}
      {notice?.kind === "error" && <Banner tone="crit">{notice.text}</Banner>}
      <Card title={`${d.unit} · ${cadence(p.recurrence.intervalMonths)} maintenance`} sub={`${p.id.slice(0, 8)} · ${d.customer}${d.location ? ` · ${d.location}` : ""}`}>
        {conflict && <div className="mb-3"><Banner tone="warn"><b>{conflict.title}</b><br />{conflict.text}</Banner></div>}
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <Field label="Repeat" hint="Only monthly recurrence is supported"><Input value="Monthly" disabled /></Field>
          <Field label="Every" hint="1–12" error={errors.every}><Select value={String(every)} onChange={(e) => setEvery(Number(e.target.value))}>{MONTHS.map((m) => <option key={m} value={m}>{m} month(s)</option>)}</Select></Field>
          <Field label="Next due" hint={`Anchor day ${anchor} · Recurrence uses UTC${then ? ` · then ${klDate(then)}` : ""}`} error={errors.nextDueAt}><Input type="datetime-local" value={next} onChange={(e) => setNext(e.target.value)} /></Field>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-3 rounded-2xl border border-[#b9cdf5] bg-primary-soft p-4">
          <div className="min-w-0 flex-1"><b className="text-[13px]">{box.title}</b><p className="text-xs text-muted">{box.text}</p></div>
          <Btn variant="primary" disabled={pending || !box.generate} title={box.why ?? undefined} onClick={generate}>Generate job</Btn>
        </div>
        {box.why && <p className="mt-1 text-right text-[11px] text-muted">{box.why}</p>}
        <h3 className="mt-4 mb-1.5 text-[13px] font-bold">Generated occurrences</h3>
        {d.occurrences.length === 0 ? <p className="text-[13px] text-muted">None yet.</p> : (
          <div className="scroll-x"><table className="w-full min-w-[520px] text-[13px]">
            <thead><tr className="text-left text-[11px] uppercase text-muted"><th className="py-1.5">Occurrence</th><th>Job</th><th>Status</th><th className="text-right"><span className="sr-only">Open</span></th></tr></thead>
            <tbody>{d.occurrences.map((o) => (
              <tr key={o.jobId} className="border-t border-line">
                <td className="py-2 font-semibold">{klDate(o.occurrenceAt)}</td><td className="text-xs">{o.jobId.slice(0, 8)}</td>
                <td>{o.status ? <JobStatusBadge s={o.status as JobStatus} /> : <span className="text-xs text-muted">—</span>}</td>
                <td className="text-right"><Link className="text-xs font-semibold text-primary hover:underline" href={`/admin/jobs?jobId=${o.jobId}`}>Open job →</Link></td>
              </tr>
            ))}</tbody>
          </table></div>
        )}
        <div className="mt-4 flex items-center justify-end gap-2">
          {dirty && <Btn disabled={pending} onClick={() => { setEvery(p.recurrence.intervalMonths); setNext(klInput(p.nextDueAt)); }}>Discard</Btn>}
          <Btn variant="primary" disabled={pending || !dirty || Object.keys(errors).length > 0} onClick={save}>Save plan</Btn>
        </div>
        <p className="mt-2 text-[11px] text-muted">Generating creates one job for the saved next date only — no automatic generation or catch-up (D16). Saving does not change jobs already generated.</p>
      </Card>
    </div>
  );
}

/** + New plan (plans.save without an id): the unit, every 1–12 months and the first date (display zone → UTC). */
function NewPlanModal({ live, onClose, onCreated, href }: { live: PlansLive; onClose: () => void; onCreated: (id: string) => void; href: (id: string) => string }) {
  const router = useRouter();
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [unitId, setUnitId] = useState(live.q.unitId ?? live.units[0]?.id ?? "");
  const [every, setEvery] = useState(3);
  const [next, setNext] = useState(`${klDate(new Date(now + 30 * 86_400_000).toISOString())}T10:00`);
  const [tried, setTried] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);
  const nextIso = next ? fromKlInput(next) : null;
  const errors = planErrors({ unitId, every, nextDueAt: nextIso }, now);
  const existing = live.rows.filter((r) => r.unitId === unitId);
  const save = () => {
    setTried(true);
    setRefusal(null);
    if (Object.keys(errors).length) return;
    run(() => savePlan({ id: null, version: null, unitId, intervalMonths: every, nextDueAt: nextIso! }), "Plan created", (v) => { onCreated(v.id); router.push(href(v.id)); }, (f) => setRefusal(planRefusal(f)));
  };
  const groups = [...new Set(live.units.map((u) => u.customer))];
  return (
    <Modal open onClose={onClose} title="New maintenance plan" footer={<><Btn onClick={onClose}>Cancel</Btn><Btn variant="primary" disabled={pending} onClick={save}>Create plan</Btn></>}>
      {refusal && <Banner tone="crit">{refusal}</Banner>}
      <Field label="Unit · required" error={tried ? errors.unitId : undefined} hint={existing.length ? `This unit already has ${existing.length} plan${existing.length === 1 ? "" : "s"} (${existing.map((r) => r.line.toLowerCase()).join("; ")}).` : undefined}>
        <Select value={unitId} onChange={(e) => setUnitId(e.target.value)}>{groups.map((g) => <optgroup key={g} label={g}>{live.units.filter((u) => u.customer === g).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</optgroup>)}</Select>
      </Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
        <Field label="Every" hint="Monthly recurrence, 1–12 months" error={tried ? errors.every : undefined}><Select value={String(every)} onChange={(e) => setEvery(Number(e.target.value))}>{MONTHS.map((m) => <option key={m} value={m}>{m} month(s)</option>)}</Select></Field>
        <Field label="First visit · next due" hint={nextIso ? `Anchor day ${new Date(nextIso).getUTCDate()} · Recurrence uses UTC · then ${klDate(nextOccurrence(nextIso, every, new Date(nextIso).getUTCDate()))}` : undefined} error={tried ? errors.nextDueAt : undefined}><Input type="datetime-local" value={next} onChange={(e) => setNext(e.target.value)} /></Field>
      </div>
      <Banner>Each occurrence becomes a periodic job (requested) at this time of day when HQ presses Generate job; HQ then books it like any agreed time (IR113).</Banner>
    </Modal>
  );
}
