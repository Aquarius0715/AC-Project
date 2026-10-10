"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Banner, Btn, Card, EmptyState, Field, Input, ListRow, Modal, Page, Select } from "@ac/web/components/ui";
import { JobStatusBadge } from "@ac/web/components/JobBits";
import { useI18n, useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { zonedParts } from "@ac/web/lib/i18n";
import { fromZonedInput, zonedInput } from "@ac/web/lib/adminJobs";
import { anchorOf, cadence, everyText, generateConflict, nextBox, nextOccurrence, planDate, planErrors, planRefusal } from "@ac/web/lib/adminPlans";
import type { JobStatus } from "@ac/web/lib/jobs";
import { generateJob, savePlan } from "../actions";
import type { PlansLive } from "../_lib/load";
import { JobsTabs, ScopeBar } from "./jobs-header";

type Detail = NonNullable<PlansLive["detail"]>;
/** What the last write on a plan left to show: saved, generated (the box names the job), a refused Generate job (the
 * CONFLICT banner reads the refreshed plan) or another refusal. Kept here because a write re-keys the detail. */
type Notice = { planId: string } & ({ kind: "saved"; text: string } | { kind: "generated"; occurrenceAt: string; jobId: string } | { kind: "conflict"; attempted: string } | { kind: "error"; text: string });
const MONTHS = Array.from({ length: 12 }, (_, i) => i + 1);

/** HQ maintenance plans (FR-A06 Plans tab, DD-A06 item 9, D16, Figma Admin 06 283:2 / 360:414) from the Core API. Texts
 * in the display language; the next date is typed and shown in the display time zone, the first render's dates come
 * from the loader (IR291). */
export function PlansView({ live }: { live: PlansLive }) {
  const t = useT();
  const [creating, setCreating] = useState(false);
  const [notice, setNotice] = useState<Notice | null>(null);
  const sel = live.detail;
  const href = (id: string) => `/admin/jobs?${new URLSearchParams({ ...Object.fromEntries(Object.entries(live.q).filter(([, v]) => v) as [string, string][]), tab: "plans", planId: id })}`;
  return (
    <Page className="max-w-[1440px]">
      <JobsTabs tab="plans" counts={live.counts} q={live.q} action={<Btn variant="primary" size="sm" onClick={() => setCreating(true)}>{t("+ New plan")}</Btn>} />
      <ScopeBar scope={live.scope} q={live.q} text={t(live.plansTotal === 1 ? "1 plan in scope" : "{n} plans in scope", { n: live.plansTotal })} clear="planId" />
      <div className="split-rev">
        <Card title={t("Recurring plans")} sub={t("next due ↑")} className="self-start">
          {live.rows.length === 0 ? <EmptyState title={t("No plans in this scope")}>{t("“+ New plan” sets up a recurring visit for a unit.")}</EmptyState> : (
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
          : <Card title={t("Plan")}><p className="text-[13px] text-muted">{t("missing" in live ? "That plan no longer exists in your scope." : live.rows.length ? "Pick a plan." : "No plan yet.")}</p></Card>}
      </div>
      {creating && <NewPlanModal live={live} onClose={() => setCreating(false)} onCreated={(id) => { setCreating(false); setNotice({ planId: id, kind: "saved", text: t("Plan created — Generate job creates the job of its first occurrence.") }); }} href={href} />}
    </Page>
  );
}

function PlanDetail({ d, now, notice, setNotice }: { d: Detail; now: number; notice: Notice | null; setNotice: (n: Notice | null) => void }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const p = d.plan;
  const [pending, run] = useAction();
  const [every, setEvery] = useState(p.recurrence.intervalMonths);
  const [next, setNext] = useState(zonedInput(p.nextDueAt, zone));
  const nextIso = next ? fromZonedInput(next, zone) || null : null;
  const nextChanged = !!nextIso && Date.parse(nextIso) !== Date.parse(p.nextDueAt);
  const dirty = every !== p.recurrence.intervalMonths || nextChanged;
  const errors = planErrors({ unitId: p.unitId, every, nextDueAt: nextChanged ? nextIso : p.nextDueAt }, now, t);
  const anchor = nextIso ? anchorOf(nextChanged ? null : p, nextChanged ? nextIso : p.nextDueAt) : p.anchorDay;
  // the saved plan's following date comes from the loader; an edit computes it here
  const then = !dirty ? d.texts.then : nextIso ? planDate(nextOccurrence(nextChanged ? nextIso : p.nextDueAt, every, anchor), i.display) : null;
  const box = nextBox(p, now, dirty, notice?.kind === "generated" ? notice : null, i, d.texts.next);
  const conflict = notice?.kind === "conflict" ? generateConflict(p, notice.attempted, i) : null;
  const save = () => {
    if (Object.keys(errors).length) return;
    const nextDueAt = nextChanged ? nextIso! : p.nextDueAt;
    run(() => savePlan({ id: p.id, version: p.version, unitId: p.unitId, intervalMonths: every, nextDueAt }), t("Plan saved"),
      () => setNotice({ planId: p.id, kind: "saved", text: t("Plan saved — {every}, next due {date} (anchor day {n}).", { every: everyText(every, t).toLowerCase(), date: planDate(nextDueAt, i.display), n: anchor }) }), (f) => setNotice({ planId: p.id, kind: "error", text: planRefusal(f, t) }));
  };
  const generate = () => run(() => generateJob(p.id, p.version, p.nextDueAt), t("Job generated"),
    (v) => setNotice({ planId: p.id, kind: "generated", occurrenceAt: p.nextDueAt, jobId: v.id }),
    (f) => setNotice(f.code === "CONFLICT" ? { planId: p.id, kind: "conflict", attempted: p.nextDueAt } : { planId: p.id, kind: "error", text: planRefusal(f, t) }));
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {notice?.kind === "saved" && <Banner tone="ok">{notice.text}</Banner>}
      {notice?.kind === "error" && <Banner tone="crit">{notice.text}</Banner>}
      <Card title={t("{unit} · {cadence} maintenance", { unit: d.unit, cadence: cadence(p.recurrence.intervalMonths, t) })} sub={`${p.id.slice(0, 8)} · ${d.customer}${d.location ? ` · ${d.location}` : ""}`}>
        {conflict && <div className="mb-3"><Banner tone="warn"><b>{conflict.title}</b><br />{conflict.text}</Banner></div>}
        <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
          <Field label={t("Repeat")} hint={t("Only monthly recurrence is supported")}><Input value={t("Monthly")} disabled /></Field>
          <Field label={t("Every")} hint="1–12" error={errors.every}><Select value={String(every)} onChange={(e) => setEvery(Number(e.target.value))}>{MONTHS.map((m) => <option key={m} value={m}>{t(m === 1 ? "1 month" : "{n} months", { n: m })}</option>)}</Select></Field>
          <Field label={t("Next due")} hint={then ? t("Anchor day {n} · Recurrence uses UTC · then {date} · times in {zone}", { n: anchor, date: then, zone }) : t("Anchor day {n} · Recurrence uses UTC · times in {zone}", { n: anchor, zone })} error={errors.nextDueAt}><Input type="datetime-local" value={next} onChange={(e) => setNext(e.target.value)} /></Field>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-3 rounded-2xl border border-[#b9cdf5] bg-primary-soft p-4">
          <div className="min-w-0 flex-1"><b className="text-[13px]">{box.title}</b><p className="text-xs text-muted">{box.text}</p></div>
          <Btn variant="primary" disabled={pending || !box.generate} title={box.why ?? undefined} onClick={generate}>{t("Generate job")}</Btn>
        </div>
        {box.why && <p className="mt-1 text-right text-[11px] text-muted">{box.why}</p>}
        <h3 className="mt-4 mb-1.5 text-[13px] font-bold">{t("Generated occurrences")}</h3>
        {d.occurrences.length === 0 ? <p className="text-[13px] text-muted">{t("None yet.")}</p> : (
          <div className="scroll-x"><table className="w-full min-w-[520px] text-[13px]">
            <thead><tr className="text-left text-[11px] uppercase text-muted"><th className="py-1.5">{t("Occurrence")}</th><th>{t("Job")}</th><th>{t("Status")}</th><th className="text-right"><span className="sr-only">{t("Open")}</span></th></tr></thead>
            <tbody>{d.occurrences.map((o) => (
              <tr key={o.jobId} className="border-t border-line">
                <td className="py-2 font-semibold">{o.date}</td><td className="text-xs">{o.jobId.slice(0, 8)}</td>
                <td>{o.status ? <JobStatusBadge s={o.status as JobStatus} /> : <span className="text-xs text-muted">—</span>}</td>
                <td className="text-right"><Link className="text-xs font-semibold text-primary hover:underline" href={`/admin/jobs?jobId=${o.jobId}`}>{t("Open job →")}</Link></td>
              </tr>
            ))}</tbody>
          </table></div>
        )}
        <div className="mt-4 flex items-center justify-end gap-2">
          {dirty && <Btn disabled={pending} onClick={() => { setEvery(p.recurrence.intervalMonths); setNext(zonedInput(p.nextDueAt, zone)); }}>{t("Discard")}</Btn>}
          <Btn variant="primary" disabled={pending || !dirty || Object.keys(errors).length > 0} onClick={save}>{t("Save plan")}</Btn>
        </div>
        <p className="mt-2 text-[11px] text-muted">{t("Generating creates one job for the saved next date only — no automatic generation or catch-up (D16). Saving does not change jobs already generated.")}</p>
      </Card>
    </div>
  );
}

/** + New plan (plans.save without an id): the unit, every 1–12 months and the first date (display zone → UTC). */
function NewPlanModal({ live, onClose, onCreated, href }: { live: PlansLive; onClose: () => void; onCreated: (id: string) => void; href: (id: string) => string }) {
  const i = useI18n(), { t } = i, zone = i.display.timeZone;
  const router = useRouter();
  const now = Date.parse(live.now);
  const [pending, run] = useAction();
  const [unitId, setUnitId] = useState(live.q.unitId ?? live.units[0]?.id ?? "");
  const [every, setEvery] = useState(3);
  const [next, setNext] = useState(`${zonedParts(new Date(now + 30 * 86_400_000).toISOString(), zone).date}T10:00`);
  const [tried, setTried] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);
  const nextIso = next ? fromZonedInput(next, zone) || null : null;
  const errors = planErrors({ unitId, every, nextDueAt: nextIso }, now, t);
  const existing = live.rows.filter((r) => r.unitId === unitId);
  const save = () => {
    setTried(true);
    setRefusal(null);
    if (Object.keys(errors).length) return;
    run(() => savePlan({ id: null, version: null, unitId, intervalMonths: every, nextDueAt: nextIso! }), t("Plan created"), (v) => { onCreated(v.id); router.push(href(v.id)); }, (f) => setRefusal(planRefusal(f, t)));
  };
  const groups = [...new Set(live.units.map((u) => u.customer))];
  const anchor = nextIso ? new Date(nextIso).getUTCDate() : null;
  return (
    <Modal open onClose={onClose} title={t("New maintenance plan")} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={save}>{t("Create plan")}</Btn></>}>
      {refusal && <Banner tone="crit">{refusal}</Banner>}
      <Field label={t("Unit · required")} error={tried ? errors.unitId : undefined} hint={existing.length ? t(existing.length === 1 ? "This unit already has 1 plan ({plans})." : "This unit already has {n} plans ({plans}).", { n: existing.length, plans: existing.map((r) => r.line.toLowerCase()).join("; ") }) : undefined}>
        <Select value={unitId} onChange={(e) => setUnitId(e.target.value)}>{groups.map((g) => <optgroup key={g} label={g}>{live.units.filter((u) => u.customer === g).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</optgroup>)}</Select>
      </Field>
      <div className="grid-fluid" style={{ ["--min" as string]: "200px" }}>
        <Field label={t("Every")} hint={t("Monthly recurrence, 1–12 months")} error={tried ? errors.every : undefined}><Select value={String(every)} onChange={(e) => setEvery(Number(e.target.value))}>{MONTHS.map((m) => <option key={m} value={m}>{t(m === 1 ? "1 month" : "{n} months", { n: m })}</option>)}</Select></Field>
        <Field label={t("First visit · next due")} hint={nextIso && anchor ? t("Anchor day {n} · Recurrence uses UTC · then {date} · times in {zone}", { n: anchor, date: planDate(nextOccurrence(nextIso, every, anchor), i.display), zone }) : undefined} error={tried ? errors.nextDueAt : undefined}><Input type="datetime-local" value={next} onChange={(e) => setNext(e.target.value)} /></Field>
      </div>
      <Banner>{t("Each occurrence becomes a periodic job (requested) at this time of day when HQ presses Generate job; HQ then books it like any agreed time (IR113).")}</Banner>
    </Modal>
  );
}
