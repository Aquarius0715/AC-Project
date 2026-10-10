// The contractor's schedule & assignments (FR-P03, DD-P03, Figma Contractor 03-1…03-10) from the Core API: the jobs
// the company can (re)assign with their delegation windows, the assign form of the selected job (assign at the agreed
// visit time, reassign at the same slot, extend while the work is under way — IR113, IR123 item 4), the technician
// candidates (members.eligible plus the reason a technician is not), the team's week and the refusals of jobs.assign.
// Pure code shared by the server loader and the client view; Vitest covers it.
import { klTime } from "@ac/web/lib/devices";
import { hhmm, until, type ApiCapacity, type ApiMember, type ApiPartnerJob, type Slot } from "@ac/web/lib/partnerOverview";
import { qualificationLabel, qualified, type ApiDetail, type ApiOffer } from "@ac/web/lib/partnerJobDetail";
import { slotText } from "@ac/web/lib/partnerJobs";

/** The states a contractor job can be (re)assigned in (IR123 item 4); an accepted offer before its window is listed too. */
export const SCHEDULE_STATUSES = ["accepted", "assigned", "in_progress", "on_hold", "rework_requested"] as const;
export const SORTS = [{ id: "status", text: "status ↑" }, { id: "dueAt", text: "deadline ↑" }] as const;
export type ScheduleSort = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): ScheduleSort => (SORTS.some((s) => s.id === v) ? (v as ScheduleSort) : "status");

type Tone = "crit" | "warn" | "ok" | "primary" | "muted";
const mmddhm = (iso: string) => klTime(iso).slice(5);
const ended = (s: Slot | null, now: number) => !!s && Date.parse(s.endAt) <= now;

export type ScheduleRow = {
  id: string; short: string; title: string; line: string; tone?: Tone;
  window: { from: string; until: string } | null; windowText: string; left: string; leftTone: Tone; elapsedPct: number;
};
/** One job of the list and its delegation window (“14 h left”, “Ended”; the bar is the elapsed share). */
export function scheduleRow(j: ApiPartnerJob, now: number, units: Map<string, string>, names: Map<string, string>): ScheduleRow | null {
  if (j.projection === "history") return null;
  const id = j.projection === "offer" ? j.jobId : j.id;
  const from = j.accessValidFrom ?? null, to = j.accessValidUntil ?? null;
  const tech = j.projection === "summary" && j.technicianMembershipId ? names.get(j.technicianMembershipId) ?? "technician" : null;
  let line: string, tone: Tone | undefined;
  if (j.projection === "offer") [line, tone] = [`Accepted · delegation starts ${from ? mmddhm(from) : "later"}`, "muted"];
  else if ((j.status === "assigned" || j.status === "in_progress") && ended(j.scheduledSlot, now)) [line, tone] = ["Work window ended · reassignment required", "crit"];
  else if (j.status === "accepted") line = "Accepted · awaiting assignment";
  else if (j.status === "assigned") {
    const ack = j.assignmentAcknowledgement === "accepted" ? "accepted ✓" : j.assignmentAcknowledgement === "cant_make" ? "can’t make it ⚠" : "awaiting acceptance";
    [line, tone] = [`Assigned · ${tech} · ${ack}`, j.assignmentAcknowledgement === "cant_make" ? "warn" : undefined];
  } else if (j.status === "in_progress") line = `In progress · ${tech ?? "technician"}`;
  else if (j.status === "on_hold") [line, tone] = ["On hold by HQ", "warn"];
  else line = `Rework · ${tech ?? "technician"}`;
  const span = from && to ? Date.parse(to) - Date.parse(from) : 0;
  const elapsedPct = span > 0 ? Math.min(100, Math.max(0, Math.round(((now - Date.parse(from!)) / span) * 100))) : 0;
  const left = to ? (Date.parse(to) <= now ? "Ended" : `${until(to, now)} left`) : "—";
  const soon = !!to && Date.parse(to) - now < 24 * 3600_000;
  return {
    id, short: id.slice(0, 8), title: j.projection === "offer" ? j.siteAddress ?? "Accepted job" : units.get(j.unitId) ?? "Unit",
    line, tone, window: from && to ? { from, until: to } : null, windowText: from && to ? `${mmddhm(from)} → ${mmddhm(to)}` : "—",
    left, leftTone: left === "Ended" ? "crit" : soon ? "warn" : "primary", elapsedPct,
  };
}

export type FormMode =
  | { kind: "assign" | "reassign" | "extend"; title: string; slot: Slot; endEditable: boolean; reasonRequired: boolean; button: string; banner?: { tone: Tone; text: string }; currentTech: string | null }
  | { kind: "blocked"; title: string; slot: Slot | null; banner: { tone: Tone; text: string } };
/** What the form of the selected job allows (IR123 item 4): the agreed visit time from accepted, the same slot from
 * assigned, an extension (same start, later end, reason) while in progress, on hold or in rework; an accepted offer
 * before its delegation period and an assigned job whose agreed time has passed cannot be booked here. */
export function formMode(j: ApiDetail | ApiOffer, label: string, now: number, names: Map<string, string>): FormMode {
  if (j.projection === "offer") {
    return { kind: "blocked", title: `Assign — ${label}`, slot: j.visitSlot, banner: { tone: "primary", text: `Accepted — the delegation period starts ${klTime(j.accessValidFrom)}; the technician can be assigned from then, for exactly ${slotText(j.visitSlot)}.` } };
  }
  const current = j.assignment ? names.get(j.assignment.technicianMembershipId) ?? "technician" : null;
  const slot = j.assignment ? { startAt: j.assignment.scheduledStart, endAt: j.assignment.scheduledEnd } : j.scheduledSlot ?? j.offer?.visitSlot ?? null;
  if (!slot) return { kind: "blocked", title: `Assign — ${label}`, slot: null, banner: { tone: "warn", text: "No agreed visit time — HQ agrees one with the client first." } };
  switch (j.status) {
    case "accepted":
      return { kind: "assign", title: `Assign — ${label}`, slot, endEditable: false, reasonRequired: false, button: "Confirm assignment", currentTech: null };
    case "assigned":
      if (ended(slot, now)) {
        return { kind: "blocked", title: `Reschedule — ${label}`, slot, banner: { tone: "crit", text: `Work window ended; reassignment required. The agreed time ${slotText(slot)} passed while the job is still “assigned”. A new time needs the client’s agreement — HQ proposes it (IR113); tell HQ in the job history.` } };
      }
      return {
        kind: "reassign", title: `Reassign — ${label}`, slot, endEditable: false, reasonRequired: false, button: "Confirm reassignment", currentTech: current,
        banner: j.assignment?.acknowledgement === "cant_make" ? { tone: "warn", text: `${current} can’t make this time${j.assignment.cantMakeReason ? `: “${j.assignment.cantMakeReason}”` : ""}. Reassign another technician for the same agreed time, or ask HQ to propose a new time to the client.` } : undefined,
      };
    case "in_progress":
    case "on_hold":
    case "rework_requested":
      return {
        kind: "extend", title: ended(slot, now) ? `Reschedule — ${label}` : `Reassign — ${label} (${j.status === "in_progress" ? "in progress" : j.status === "on_hold" ? "on hold" : "rework"})`, slot, endEditable: true, reasonRequired: true,
        button: "Confirm reassignment", currentTech: current,
        banner: ended(slot, now) ? { tone: "crit", text: `Work window ended; reassignment required. The work window ${slotText(slot)} ended while the job is “${j.status.replace(/_/g, " ")}” — keep the start and set a later end (same or another technician, IR89).` } : undefined,
      };
  }
  return { kind: "blocked", title: `Assign — ${label}`, slot, banner: { tone: "muted", text: `The job is ${j.status.replace(/_/g, " ")} — nothing to schedule.` } };
}

/** The free time of a capacity day: the available slots minus the assigned ones (“10:00–17:00”, “—”). */
export function freeText(c: ApiCapacity | undefined): string {
  if (!c || !c.availableSlots.length) return "no hours";
  const parts: string[] = [];
  for (const a of c.availableSlots) {
    let start = Date.parse(a.startAt);
    const end = Date.parse(a.endAt);
    for (const b of [...c.assignedSlots].sort((x, y) => Date.parse(x.startAt) - Date.parse(y.startAt))) {
      const bs = Date.parse(b.startAt), be = Date.parse(b.endAt);
      if (be <= start || bs >= end) continue;
      if (bs > start) parts.push(`${hhmm(new Date(start).toISOString())}–${hhmm(new Date(bs).toISOString())}`);
      start = Math.max(start, be);
    }
    if (start < end) parts.push(`${hhmm(new Date(start).toISOString())}–${hhmm(new Date(end).toISOString())}`);
  }
  return parts.length ? parts.join(", ") : "fully booked";
}

export type Candidate = { id: string; name: string; sub: string; badge: { text: string; tone: Tone }; eligible: boolean };
/** The company's technicians for the slot: eligible ones (members.eligible) first, the others with the reason. */
export function candidates(members: ApiMember[], eligible: Set<string>, required: string[], slot: Slot, day: ApiCapacity[], currentId: string | null): Candidate[] {
  const cap = new Map(day.map((c) => [c.membershipId, c]));
  const overlaps = (a: Slot, b: Slot) => Date.parse(a.startAt) < Date.parse(b.endAt) && Date.parse(b.startAt) < Date.parse(a.endAt);
  const rows = members.filter((m) => m.role === "technician").map((m): Candidate => {
    const c = cap.get(m.id);
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => qualificationLabel(q.code)).join(" · ") || "no qualifications";
    const sub = `${quals} · free ${freeText(c)}`;
    if (eligible.has(m.id)) return { id: m.id, name: m.displayName, sub, badge: m.id === currentId ? { text: "Current", tone: "primary" } : { text: "Qualified", tone: "ok" }, eligible: true };
    const missing = required.filter((r) => !qualified(m, [r], slot.startAt)).map((c) => qualificationLabel(c));
    const badge = missing.length ? { text: `Missing: ${missing.join(", ")}`, tone: "warn" as Tone }
      : c?.unavailability ? { text: `Unavailable (${c.unavailability.replace(/_/g, " ")})`, tone: "warn" as Tone }
      : c?.assignedSlots.some((s) => overlaps(s, slot)) ? { text: m.id === currentId ? "Current · booked here" : "Busy at this time", tone: "muted" as Tone }
      : { text: "Not eligible", tone: "muted" as Tone };
    return { id: m.id, name: m.displayName, sub, badge, eligible: false };
  });
  return rows.sort((a, b) => Number(b.eligible) - Number(a.eligible) || a.name.localeCompare(b.name));
}

export type WeekCell = { kind: "booked" | "free" | "leave" | "none" | "proposal"; text: string; sub?: string };
export type WeekRow = { id: string; name: string; sub: string; cells: WeekCell[] };
const KL = 8 * 3600_000;
/** The team's week (Mon–Sun, KL): per technician and day the booked blocks with their job, leave, no hours or free;
 * the selected technician's cell of the proposed slot is “This proposal”. */
export function weekGrid(dates: string[], members: ApiMember[], capacity: ApiCapacity[][], jobsOf: Map<string, { short: string; slot: Slot }[]>, proposal: { technicianId: string; slot: Slot; short: string } | null): WeekRow[] {
  const dayOf = (iso: string) => new Date(Date.parse(iso) + KL).toISOString().slice(0, 10);
  return members.filter((m) => m.role === "technician").map((m) => {
    let avail = 0;
    const cells = dates.map((date, i): WeekCell => {
      const c = capacity[i]?.find((x) => x.membershipId === m.id);
      avail += c?.availableMinutes ?? 0;
      if (proposal && proposal.technicianId === m.id && dayOf(proposal.slot.startAt) === date) return { kind: "proposal", text: `${hhmm(proposal.slot.startAt)}–${hhmm(proposal.slot.endAt)}`, sub: proposal.short };
      if (c?.unavailability) return { kind: "leave", text: "Leave", sub: c.unavailability.replace(/_/g, " ") };
      if (!c || !c.availableMinutes) return { kind: "none", text: "no hours" };
      if (c.assignedSlots.length) {
        const s = c.assignedSlots[0];
        const job = (jobsOf.get(m.id) ?? []).find((j) => Date.parse(j.slot.startAt) < Date.parse(s.endAt) && Date.parse(s.startAt) < Date.parse(j.slot.endAt));
        return { kind: "booked", text: `${hhmm(s.startAt)}–${hhmm(c.assignedSlots[c.assignedSlots.length - 1].endAt)}`, sub: [job?.short ?? "booked", c.assignedSlots.length > 1 ? `+${c.assignedSlots.length - 1}` : ""].filter(Boolean).join(" ") };
      }
      return { kind: "free", text: "free" };
    });
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => qualificationLabel(q.code).replace(/ (unit )?work$/, "")).join(", ");
    return { id: m.id, name: m.displayName, sub: `${quals || "—"} · ${Math.round(avail / 60)} h avail.`, cells };
  });
}

/** Readable refusals of jobs.assign (DD-P03 boundary cases). */
export function assignRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, tech: string): string {
  switch (f.messageKey) {
    case "errors.assignment_overlap": return `Overlaps a confirmed schedule for ${tech} (CONFLICT) — choose another technician.`;
    case "errors.access_window": return "Outside the delegation period — assignments are possible only inside it.";
    case "errors.qualification_missing": return `${tech}’s qualification is not valid for this slot (revoked or expiring, FORBIDDEN) — choose another technician.`;
    case "errors.technician_out_of_scope": case "errors.technician_not_allowed": return `${tech} cannot be assigned to this unit (FORBIDDEN).`;
    case "error.invalidState": return "The job can no longer be scheduled in its current state — the latest state is shown.";
    case "error.versionConflict": return "The job changed elsewhere — the latest version is shown; check and confirm again.";
  }
  if (f.code === "NOT_FOUND") return "The job is no longer delegated to your company.";
  if (f.code === "VALIDATION") {
    const text: Record<string, string> = {
      "errors.slot_not_agreed": "the slot must be the agreed visit time — a different time needs the client’s approval via HQ",
      "errors.extension_only": "keep the start and set an end at or after the current end",
      "error.required": "required (1–1000 characters)", "error.length": "at most 1000 characters", "error.range": "the end must be after the start",
    };
    return Object.entries(f.fieldErrors).map(([k, v]) => `${k}: ${text[v] ?? v}`).join(" · ") || "Check the input.";
  }
  return `${f.code} — ${f.messageKey}`;
}
