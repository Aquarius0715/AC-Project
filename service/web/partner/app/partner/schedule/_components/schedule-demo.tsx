"use client";
const ACCEPTED_AWAITING_ASSIGNMENT = "Accepted · awaiting assignment";
const TECH_EXTERNAL_A = "tech-external-a";

import Link from "next/link";
import { useState } from "react";
import { Badge, Banner, Btn, Card, EmptyState, Field, ListRow, Page, Select, Textarea, cx, useToast, type Tone } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { fmt, jobActions, longDate, useJobs } from "@ac/web/lib/jobs";
import { Icon } from "@ac/web/components/Icon";
import { orDash } from "@ac/web/lib/text";
import { nonEmpty } from "@ac/web/lib/lists";
import { DateTimeSelect } from "@ac/web/components/TimeSelect";

const staticJobs = nonEmpty([
  { id: "job-p02", unit: "Rooftop unit", st: ACCEPTED_AWAITING_ASSIGNMENT, win: "09-14 01:00 → 09-22 00:00", left: "14 h left" },
  { id: "job-p12", unit: "Lobby AC", st: ACCEPTED_AWAITING_ASSIGNMENT, win: "09-19 09:00 → 09-24 18:00", left: "3 d 8 h left" },
  { id: "job-contractor-a", unit: "Bedroom AC", st: "In progress · tech-external-a", win: "09-18 08:00 → 09-23 00:00", left: "1 d 14 h left" },
  { id: "job-p07", unit: "Server room AC", st: "Work window ended · reassignment required", win: "09-15 08:00 → 09-20 17:00", left: "Ended" },
]);
const grid: [string, string, string[]][] = [[TECH_EXTERNAL_A, "Refrigerant · 32 h avail.", ["10–12 job-p02", "09–13 job-contractor-a", "free", "13–17 job-c11", "free", "no hours", "no hours"]], ["tech-external-a2", "General · 40 h avail.", ["free", "free", "10–15 job-c09", "09–17 Leave", "09–12 job-c12", "no hours", "no hours"]]];
// the technician's answer and its banner tone; no answer yet is awaiting acceptance in the primary tone
const ACK_WORD = new Map([["accepted", "accepted"], ["cant_make", "can’t make it"]]);
const ACK_TONE = new Map<string, Tone>([["accepted", "ok"], ["cant_make", "warn"]]);
/** A schedule cell's colours: free, no working hours, leave, or booked. */
function cellTone(c: string): string {
  if (c === "free") return "bg-ok-soft text-ok";
  if (c === "no hours") return "bg-surface2 text-muted-foreground";
  return c.includes("Leave") ? "bg-warn-soft text-warn" : "bg-primary-soft text-primary";
}
const days = ["Mon 21", "Tue 22", "Wed 23", "Thu 24", "Fri 25", "Sat 26", "Sun 27"];

/** The Phase 1A demo of the schedule (fixture jobs and the in-browser job store). */
export function ScheduleDemo({ jobId }: Readonly<{ jobId?: string }>) {
  const toast = useToast();
  const store = useJobs();
  const live = store.filter((j) => j.contractor === "contractor-a" && (j.status === "accepted" || j.status === "assigned") && staticJobs.every((s) => s.id !== j.id));
  const jobs = [...live.map((j) => ({ id: j.id, unit: j.unit, st: j.status === "accepted" ? ACCEPTED_AWAITING_ASSIGNMENT : `Assigned · ${orDash(j.technician)} · ${ACK_WORD.get(j.techAck?.status ?? "") ?? "awaiting acceptance"}`, win: `${orDash(j.scheduled?.date.slice(5))} 00:00 → ${orDash(j.scheduled?.date.slice(5))} 23:59`, left: "fixed time", live: true })), ...staticJobs.map((j) => ({ ...j, live: false }))];
  const fallback = { ...staticJobs[0], live: false }; // the first fixture job when nothing is picked
  const [selId, setSelId] = useState(jobId ?? fallback.id);
  const sel = jobs.find((j) => j.id === selId) ?? fallback;
  const setSel = (j: (typeof jobs)[number]) => { setSelId(j.id); };
  const lj = store.find((j) => j.id === sel.id && sel.live);
  const [tech, setTech] = useState(TECH_EXTERNAL_A);
  const [start, setStart] = useState("2026-09-21T10:00");
  const [end, setEnd] = useState("2026-09-21T12:00");
  const [reason, setReason] = useState("");
  const [assigned, setAssigned] = useState<string[]>([]);
  const [message, setMessage] = useState<{ tone: "crit" | "ok" | "warn"; t: string } | null>(null);
  const isInProg = sel.id === "job-contractor-a" || sel.id === "job-p07";
  const save = () => {
    if (lj) { jobActions.partnerAssign(lj.id, tech); setMessage({ tone: "ok", t: `Assigned ${tech} to ${lj.id} for ${fmt(lj.scheduled)}. The technician must accept.` }); toast("Technician assigned — waiting for acceptance"); return; }
    if (isInProg && !reason.trim()) { setMessage({ tone: "crit", t: "Reason required when reassigning a job already in progress (VALIDATION)." }); return; }
    const we = new Date(sel.id === "job-p02" ? "2026-09-22T00:00" : "2026-09-24T18:00");
    if (new Date(start) >= new Date(end)) { setMessage({ tone: "crit", t: "Start must be before end." }); return; }
    if (new Date(end) > we) { setMessage({ tone: "crit", t: "Slot is outside the delegation period (VALIDATION)." }); return; }
    if (tech === TECH_EXTERNAL_A && start.startsWith("2026-09-21T09")) { setMessage({ tone: "warn", t: "Overlaps a confirmed schedule for tech-external-a (CONFLICT)." }); return; }
    setAssigned((a) => [...a, sel.id]); setMessage({ tone: "ok", t: `Assigned ${tech} to ${sel.id}. Notifications sent (preview).` }); toast("Technician assigned");
  };
  return (
    <Page>
      {jobs.length === 0 && <EmptyState title="No accepted jobs" />}
      <div className="split-rev">
        <div className="flex min-w-0 flex-col gap-4 self-start">
          <Card title="Accepted jobs" sub="status, earliest stage first">
            <div className="flex flex-col gap-2">{jobs.map((j) => <ListRow key={j.id} selected={sel.id === j.id} onClick={() => { setSel(j); setMessage(null); }}><div className="min-w-0"><div className="flex flex-wrap items-center gap-1.5"><b className="text-[13px]">{j.id} · {j.unit}</b>{j.live && <OriginBadge origin={store.find((x) => x.id === j.id)?.origin ?? "request"} />}</div><div className="text-xs text-muted-foreground">{assigned.includes(j.id) ? "Assigned · awaiting technician acceptance" : j.st}</div></div></ListRow>)}</div>
          </Card>
          <Card title="Delegation windows" sub="Work must fit inside each window. Access ends automatically at the end time.">
            {jobs.map((j) => <div key={j.id} className="flex flex-wrap justify-between gap-1 border-t border-line py-2 text-xs first:border-0"><span><b>{j.id}</b><span className="block text-muted-foreground">{j.win}</span></span><Badge tone={j.left === "Ended" ? "crit" : "muted"}>{j.left}</Badge></div>)}
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`Assign — ${sel.id} · ${sel.unit}`} sub={`Delegation period: ${sel.win.replace(" → ", " – ")} — work must fit within this window.`}>
            <div className="grid-fluid" style={{ ["--min"as string]: "200px" }}>
              <Field label="Technician (own company, active, qualified)"><Select value={tech} onChange={(e) => { setTech(e.target.value); }}><option value="tech-external-a">tech-external-a — refrigerant handling, active</option><option value="tech-external-a2">tech-external-a2 — general maintenance, active</option></Select></Field>
              {lj ? <Field label="Visit time (agreed with the client)" hint="Fixed. A different time needs the client’s approval via HQ."><div className="rounded-control border border-line bg-surface2 px-3 py-2 text-[13px] font-semibold"><Icon name="locked" /> {lj.scheduled ? longDate(lj.scheduled) : "—"}</div></Field> : <>
              <Field label="Work start"><DateTimeSelect value={start} onChange={(v) => { setStart(v); }} /></Field>
              <Field label="Work end"><DateTimeSelect value={end} onChange={(v) => { setEnd(v); }} /></Field></>}
            </div>
            {isInProg && <div className="mt-3"><Field label="Reassign reason (required)" hint="Reassigning a job already in progress requires a reason and immediately revokes the previous technician's access."><Textarea value={reason} onChange={(e) => { setReason(e.target.value); }} /></Field></div>}
            <p className="mt-2 flex items-center gap-1 text-xs text-ok"><Icon name="check" />No overlapping confirmed schedule for {tech} in this window.</p>
            {lj?.techAck && <div className="mt-3"><Banner tone={ACK_TONE.get(lj.techAck.status) ?? "primary"}>{lj.techAck.status === "accepted" ? `${orDash(lj.technician)} accepted the assignment (${orDash(lj.techAck.at)})` : null}{lj.techAck.status !== "accepted" && lj.techAck.status === "cant_make" && `${orDash(lj.technician)} can’t make this time: “${orDash(lj.techAck.reason)}”${lj.techAck.alt ? ` · could do ${lj.techAck.alt}` : ""}. Reassign another technician below, or ask HQ to propose a new time to the client.`}{lj.techAck.status !== "accepted" && lj.techAck.status !== "cant_make" && `Waiting for ${orDash(lj.technician)} to accept the assignment.`}</Banner></div>}
            {message && <div className="mt-3"><Banner tone={message.tone}>{message.t}</Banner></div>}
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><span className="text-[11px] text-muted-foreground">Hint: start 09:00 shows the overlap conflict · end after window shows validation.</span><Btn variant="primary" onClick={save}>{isInProg || lj?.status === "assigned" ? "Reassign" : "Assign technician"}</Btn></div>
          </Card>
          <Card title="Team schedule — week of 09-21" action={<Link href="/partner/team" className="text-xs font-semibold text-primary">Team & capacity <Icon name="arrowRight" /></Link>}>
            <div className="scroll-x"><table className="w-full min-w-[640px] text-[11px]"><thead><tr><td className="p-1 text-left" />{days.map((d) => <th key={d} className="p-1 text-left text-muted-foreground">{d}</th>)}</tr></thead><tbody>{grid.map(([n, q, cells]) => <tr key={n} className="border-t border-line"><td className="p-1.5"><b className="text-xs">{n}</b><div className="text-muted-foreground">{q}</div></td>{cells.map((c, i) => <td key={i} className="p-1"><div className={cx("rounded-lg px-1.5 py-1", cellTone(c))}>{c}</div></td>)}</tr>)}</tbody></table></div>
            <p className="mt-2 flex flex-wrap gap-3 text-[11px] text-muted-foreground"><span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-primary-soft" />Confirmed assignment</span><span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-warn-soft" />Leave / unavailable</span><span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-surface2" />No available hours</span></p>
            <p className="text-[11px] text-muted-foreground">Only own-company technicians are shown. Times in Asia/Kuala_Lumpur. Overlap with a confirmed block is rejected on save (CONFLICT).</p>
          </Card>
        </div>
      </div>
    </Page>
  );
}
