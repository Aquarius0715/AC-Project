"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, Input, ListRow, Page, Select, Textarea, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { zonedInstant } from "@ac/web/lib/i18n";
import { assignRefusal, weekGrid, type WeekCell } from "@ac/web/lib/partnerSchedule";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { assignTechnician } from "../actions";
import type { ScheduleLive } from "../_lib/load";

const toneText: Record<string, string> = { crit: "text-crit", warn: "text-warn", muted: "text-muted", primary: "text-primary", ok: "text-ok" };
const barTone: Record<string, string> = { crit: "bg-crit", warn: "bg-warn", primary: "bg-primary", muted: "bg-muted", ok: "bg-ok" };
const cellTone: Record<WeekCell["kind"], string> = {
  booked: "bg-primary text-white", free: "border border-dashed border-line text-muted", leave: "bg-warn-soft text-warn", none: "bg-surface2 text-muted", proposal: "border-2 border-dashed border-primary bg-primary-soft/60 text-primary",
};

/** The contractor's schedule & assignments (FR-P03, Figma Contractor 03-1…03-10) from the Core API. Texts in the user's
 * display language; the job's times come formatted from the loader, the team's week is in Kuala Lumpur time (IR275). */
export function ScheduleView({ live }: { live: ScheduleLive }) {
  const t = useT();
  const patch = useUrlPatch();
  const sel = live.selected;
  if (!live.rows.length) {
    return (
      <Page>
        <EmptyState title={t("No accepted jobs")} action={<Link className="text-sm font-semibold text-primary" href="/partner/jobs?tab=offered">{t("Open offers →")}</Link>}>
          {t("Jobs your company has accepted appear here to assign a technician for the agreed visit time.")}
        </EmptyState>
      </Page>
    );
  }
  return (
    <Page>
      <div className="split-rev">
        <div className="flex min-w-0 flex-col gap-4 self-start">
          <Card title={t("Accepted jobs")} action={
            <label className="flex items-center gap-1 text-[11px] text-muted">{t("Sort")}
              <Select aria-label={t("Sort")} className="w-auto py-1 text-[11px]" value={live.sort} onChange={(e) => patch({ sort: e.target.value === "status" ? null : e.target.value })}>
                {live.sorts.map((s) => <option key={s.id} value={s.id}>{s.text}</option>)}
              </Select>
            </label>
          }>
            <div className="flex flex-col gap-2">
              {live.rows.map((r) => (
                <ListRow key={r.id} selected={sel?.id === r.id} href={`/partner/schedule?jobId=${r.id}${live.sort === "status" ? "" : `&sort=${live.sort}`}`}>
                  <div className="min-w-0">
                    <b className={cx("block truncate text-[13px]", sel?.id === r.id && "text-primary")}>{r.short} · {r.title}</b>
                    <span className={cx("block text-xs", r.tone ? toneText[r.tone] : "text-muted")}>{r.line}</span>
                  </div>
                </ListRow>
              ))}
            </div>
            {live.total > live.rows.length && <p className="mt-2 text-[11px] text-muted">{t("Showing {shown} of {total}.", { shown: live.rows.length, total: live.total })}</p>}
          </Card>
          <Card title={t("Delegation windows")} sub={t("Work must fit inside each window. Access ends automatically at the end time.")}>
            {live.rows.map((r) => (
              <div key={r.id} className="border-t border-line py-2 first:border-0">
                <div className="flex items-center justify-between gap-2 text-xs"><b>{r.short}</b><span className={cx("font-semibold", toneText[r.leftTone])}>{r.left}</span></div>
                <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-surface2"><div className={cx("h-full rounded-full", barTone[r.leftTone])} style={{ width: `${r.ended ? 100 : r.elapsedPct}%` }} /></div>
                <div className="mt-1 text-[11px] text-muted">{r.windowText}</div>
              </div>
            ))}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          {sel ? <Workspace key={sel.id} sel={sel} week={live.week} members={live.members} zone={live.zone} otherZone={live.otherZone} /> : <Card title={t("Assign")}><p className="text-[13px] text-muted">{t("The job is no longer delegated to your company — pick another one.")}</p></Card>}
        </div>
      </div>
    </Page>
  );
}

type Selected = NonNullable<ScheduleLive["selected"]>;
type Choice = { tech: string; setTech: (v: string) => void; end: string; setEnd: (v: string) => void };

/** The form and the week of the selected job share the chosen technician and end (the week's “This proposal”). */
function Workspace({ sel, week, members, zone, otherZone }: { sel: Selected; week: ScheduleLive["week"]; members: ScheduleLive["members"]; zone: string; otherZone: boolean }) {
  const firstEligible = sel.candidates.find((c) => c.eligible && c.id !== sel.currentId) ?? sel.candidates.find((c) => c.eligible);
  const [tech, setTech] = useState(firstEligible?.id ?? "");
  const [end, setEnd] = useState(sel.text.end);
  const choice = { tech, setTech, end, setEnd };
  return (
    <>
      <AssignCard sel={sel} choice={choice} zone={zone} otherZone={otherZone} />
      {week && <WeekCard week={week} members={members} sel={sel} choice={choice} zone={zone} />}
    </>
  );
}

/** The slot the form proposes: the agreed or current one, or the same start with the chosen end — typed in the user's
 * display time zone (NFR-08) — while extending. */
function proposed(sel: Selected, end: string, zone: string) {
  const endAt = sel.mode.kind === "extend" && end ? zonedInstant(end.slice(0, 10), end.slice(11, 16), zone) : "";
  return sel.mode.kind === "extend" && endAt ? { startAt: sel.mode.slot.startAt, endAt } : sel.mode.slot;
}

function AssignCard({ sel, choice, zone, otherZone }: { sel: Selected; choice: Choice; zone: string; otherZone: boolean }) {
  const t = useT();
  const m = sel.mode;
  const [pending, run] = useAction();
  const { tech, setTech, end, setEnd } = choice;
  const [reason, setReason] = useState("");
  const [tried, setTried] = useState(false);
  const [result, setResult] = useState<{ tone: "ok" | "crit" | "warn"; text: string } | null>(null);
  const chosen = sel.candidates.find((c) => c.id === tech);
  const currentName = sel.candidates.find((c) => c.id === sel.currentId)?.name;
  const window = sel.text.window ? t("Delegation period: {span} — work must fit within this window.", { span: sel.text.window }) : null;
  if (m.kind === "blocked") {
    return (
      <Card title={m.title}>
        <div className="flex flex-col gap-3">
          <Banner tone={m.banner.tone === "muted" ? undefined : m.banner.tone}>{m.banner.text}</Banner>
          {window && <Banner>{window}</Banner>}
          <div className="flex flex-wrap gap-2"><Link className="text-xs font-semibold text-primary" href={`/partner/jobs/${sel.id}`}>{t("Open the job →")}</Link><Link className="text-xs font-semibold text-primary" href={`/partner/history?jobId=${sel.id}`}>{t("Job history →")}</Link></div>
        </div>
      </Card>
    );
  }
  const slot = proposed(sel, end, zone)!;
  const endOk = !m.endEditable || (end && Date.parse(slot.endAt) >= Date.parse(m.slot.endAt));
  const reasonMissing = m.reasonRequired && !reason.trim();
  const same = tech === sel.currentId && (!m.endEditable || slot.endAt === m.slot.endAt);
  const save = () => {
    setTried(true);
    if (!tech || reasonMissing || !endOk || same) return;
    const name = chosen?.name ?? t("technician");
    run(() => assignTechnician(sel.id, sel.version, tech, slot, reason), t(m.kind === "assign" ? "Technician assigned — waiting for acceptance" : "Assignment changed"),
      (v) => setResult({ tone: "ok", text: m.kind === "assign" ? t("Assigned {name} for {slot}. The technician must accept (受領); HQ and the technician are notified.", { name, slot: v.slot }) : t("Changed to {name} for {slot}. The technician must accept (受領); HQ and the technician are notified.", { name, slot: v.slot }) }),
      (f: ActionFailure) => setResult({ tone: f.code === "CONFLICT" ? "warn" : "crit", text: assignRefusal(f, name, t) }));
  };
  return (
    <Card title={m.title}>
      <div className="flex flex-col gap-3">
        {m.banner && <Banner tone={m.banner.tone === "muted" ? undefined : m.banner.tone}>{m.banner.text}</Banner>}
        {window && <Banner>{window}</Banner>}
        <Field label={sel.currentId ? t("New technician (current: {name})", { name: currentName ?? "—" }) : t("Technician (own company, active, qualified)")} error={tried && !tech ? t("Choose a technician") : tried && same ? t("Choose another technician or a later end") : undefined}>
          <div role="radiogroup" aria-label={t("Technician")} className="flex flex-col gap-1.5">
            {sel.candidates.length === 0 && <p className="text-[13px] text-muted">{t("No technicians in your company yet — add them in Team & capacity.")}</p>}
            {sel.candidates.map((c) => (
              <button key={c.id} type="button" role="radio" aria-checked={tech === c.id} disabled={!c.eligible} onClick={() => setTech(c.id)}
                className={cx("flex items-center justify-between gap-2 rounded-xl border px-3 py-2 text-left", tech === c.id ? "border-primary bg-primary-soft/60" : "border-line bg-surface", !c.eligible && "cursor-not-allowed opacity-70")}>
                <span className="min-w-0"><b className={cx("block text-[13px]", tech === c.id && "text-primary")}>{c.name}</b><span className="block text-[11px] text-muted">{c.sub}</span></span>
                <Badge tone={c.badge.tone === "muted" ? "muted" : c.badge.tone}>{c.badge.text}</Badge>
              </button>
            ))}
          </div>
          <p className="mt-1 text-[11px] text-muted">
            {t("Only active technicians of your company are listed; eligibility is checked again on save.")}
            {otherZone && ` ${t("Free hours are in Kuala Lumpur time (Asia/Kuala_Lumpur).")}`}
          </p>
        </Field>
        {m.endEditable ? (
          <div className="grid-fluid" style={{ ["--min" as string]: "220px" }}>
            <Field label={t("Work start (kept)")}><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 {sel.text.start}</div></Field>
            <Field label={t("Work end (same or later)")} hint={t("Times in {zone}.", { zone })} error={tried && !endOk ? t("The end must be at or after the current end") : undefined}><Input type="datetime-local" value={end} min={sel.text.end} onChange={(e) => setEnd(e.target.value)} /></Field>
          </div>
        ) : (
          <Field label={t("Visit time (agreed with the client)")} hint={t("Fixed. A different time needs the client’s approval via HQ.")}>
            <div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold">🔒 {sel.text.slot}</div>
          </Field>
        )}
        {m.kind !== "assign" && (
          <Field label={t(m.reasonRequired ? "Reason for reassignment (required, 1–1000)" : "Reason (optional, kept with the change)")} error={tried && reasonMissing ? t("A reason is required while the work is under way") : undefined}>
            <Textarea value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} placeholder={m.reasonRequired ? t("tech-external-a is on sick leave from today.") : ""} />
          </Field>
        )}
        {m.kind === "extend" && chosen && chosen.id !== sel.currentId && <Banner tone="warn">{t("On save, {current} loses action access immediately. The job keeps its state; {name} continues in a new draft version and the original report authorship is kept.", { current: currentName ?? t("the current technician"), name: chosen.name })}</Banner>}
        {sel.ack && m.kind === "reassign" && sel.ack.status !== "cant_make" && <Banner tone={sel.ack.status === "accepted" ? "ok" : "primary"}>{t(sel.ack.status === "accepted" ? "The current technician accepted the assignment ✓" : "Waiting for the current technician to accept (受領).")}</Banner>}
        {sel.text.alternative && <p className="text-[11px] text-muted">{t("The technician could do {slot} instead — a different time needs the client’s approval via HQ.", { slot: sel.text.alternative })}</p>}
        {sel.candidates.length > 0 && !sel.candidates.some((c) => c.eligible) && <Banner tone="warn">{t("No technician of your company can take this time (busy, unavailable or not qualified). Free a technician in Team & capacity, or ask HQ to agree another time with the client.")}</Banner>}
        {chosen && (chosen.eligible
          ? <Banner tone="ok">{t("No overlapping confirmed schedule for {name} in this window.", { name: chosen.name })}</Banner>
          : <Banner tone="warn">{t("{name}: {reason} — saving is refused.", { name: chosen.name, reason: chosen.badge.text })}</Banner>)}
        {result && <Banner tone={result.tone}>{result.text}</Banner>}
        <Btn variant="primary" className="w-full justify-center" disabled={pending || !sel.candidates.some((c) => c.eligible)} onClick={save}>{m.kind === "extend" && tech === sel.currentId ? t("Save new slot") : m.button}</Btn>
        <p className="text-[11px] text-muted">{t("Reassigning a job already in progress requires a reason and immediately revokes the previous technician’s access.")}</p>
      </div>
    </Card>
  );
}

function WeekCard({ week, members, sel, choice, zone }: { week: NonNullable<ScheduleLive["week"]>; members: ScheduleLive["members"]; sel: Selected; choice: Choice; zone: string }) {
  const t = useT();
  const slot = proposed(sel, choice.end, zone);
  const unchanged = choice.tech === sel.currentId && slot?.endAt === sel.mode.slot?.endAt; // the current booking, not a proposal
  const proposal = slot && sel.mode.kind !== "blocked" && choice.tech && !unchanged ? { technicianId: choice.tech, slot, short: sel.short } : null;
  const rows = weekGrid(week.dates, members, week.capacity, new Map(Object.entries(week.jobsOf)), proposal, t);
  return (
    <Card title={week.title} action={<Link href="/partner/team" className="text-xs font-semibold text-primary">{t("Team & capacity →")}</Link>}>
      {rows.length === 0 ? <p className="text-[13px] text-muted">{t("No technicians in your company yet.")}</p> : (
        <div className="scroll-x">
          <table className="w-full min-w-[720px] text-[11px]">
            <thead><tr><th className="p-1 text-left" />{week.dates.map((d, k) => <th key={d} className={cx("p-1 text-center font-semibold", d === week.today ? "text-primary" : "text-muted")}>{week.days[k]}</th>)}</tr></thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className="border-t border-line">
                  <td className="p-1.5 align-top"><b className="text-xs">{r.name}</b><div className="text-muted">{r.sub}</div></td>
                  {r.cells.map((c, k) => <td key={k} className="p-1"><div className={cx("min-h-9 rounded-lg px-1.5 py-1", cellTone[c.kind])}><b className="block">{c.text}</b>{c.sub && <span className="block truncate opacity-80">{c.sub}</span>}</div></td>)}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="mt-2 flex flex-wrap gap-3 text-[11px] text-muted">
        <span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-primary" />{t("Confirmed assignment")}</span>
        <span><i className="mr-1 inline-block h-2 w-2 rounded-sm border border-dashed border-primary" />{t("This proposal")}</span>
        <span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-warn-soft" />{t("Leave / unavailable")}</span>
        <span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-surface2" />{t("No available hours")}</span>
      </p>
      <p className="text-[11px] text-muted">{t("Only own-company technicians are shown. Days and times in Kuala Lumpur time (Asia/Kuala_Lumpur). Overlap with a confirmed block is rejected on save (CONFLICT).")}</p>
    </Card>
  );
}
