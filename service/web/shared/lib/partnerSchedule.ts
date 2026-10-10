// The contractor's schedule & assignments (FR-P03, DD-P03, Figma Contractor 03-1…03-10) from the Core API: the jobs
// the company can (re)assign with their delegation windows, the assign form of the selected job (assign at the agreed
// visit time, reassign at the same slot, extend while the work is under way — IR113, IR123 item 4), the technician
// candidates (members.eligible plus the reason a technician is not), the team's week and the refusals of jobs.assign.
// Pure code shared by the server loader and the client view; Vitest covers it. Texts in the display language (`t` / `i`,
// IR275): the loader formats the job's instants in the user's display time zone; the team's capacity — the free hours
// and the week — stays on Kuala Lumpur working days and clock times, and the screen says so.
import { EN, showSpan, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { hhmm, shortQualification, unavailabilityLabel, until, type ApiCapacity, type ApiMember, type ApiPartnerJob, type Slot } from "@ac/web/lib/partnerOverview";
import { qualificationLabel, qualified, statusWord, type ApiDetail, type ApiOffer } from "@ac/web/lib/partnerJobDetail";

const en = translator("en");

/** The states a contractor job can be (re)assigned in (IR123 item 4); an accepted offer before its window is listed too. */
export const SCHEDULE_STATUSES = ["accepted", "assigned", "in_progress", "on_hold", "rework_requested"] as const;
export const SORTS = [{ id: "status", text: "Status ↑" }, { id: "dueAt", text: "Deadline ↑" }] as const;
export type ScheduleSort = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): ScheduleSort => (SORTS.some((s) => s.id === v) ? (v as ScheduleSort) : "status");

type Tone = "crit" | "warn" | "ok" | "primary" | "muted";
const ended = (s: Slot | null, now: number) => !!s && Date.parse(s.endAt) <= now;
const ACKS: Record<string, string> = { accepted: "accepted ✓", cant_make: "can’t make it ⚠" };

export type ScheduleRow = {
  id: string; short: string; title: string; line: string; tone?: Tone;
  window: { from: string; until: string } | null; windowText: string; left: string; ended: boolean; leftTone: Tone; elapsedPct: number;
};
/** One job of the list and its delegation window (“14 h left”, “Ended”; the bar is the elapsed share). */
export function scheduleRow(j: ApiPartnerJob, now: number, units: Map<string, string>, names: Map<string, string>, i: I18n = EN): ScheduleRow | null {
  const { t, display } = i;
  if (j.projection === "history") return null;
  const id = j.projection === "offer" ? j.jobId : j.id;
  const from = j.accessValidFrom ?? null, to = j.accessValidUntil ?? null;
  const name = (j.projection === "summary" && j.technicianMembershipId ? names.get(j.technicianMembershipId) : undefined) ?? t("technician");
  let line: string, tone: Tone | undefined;
  if (j.projection === "offer") [line, tone] = [from ? t("Accepted · delegation starts {time}", { time: showTime(from, display) }) : t("Accepted · delegation starts later"), "muted"];
  else if ((j.status === "assigned" || j.status === "in_progress") && ended(j.scheduledSlot, now)) [line, tone] = [t("Work window ended · reassignment required"), "crit"];
  else if (j.status === "accepted") line = t("Accepted · awaiting assignment");
  else if (j.status === "assigned") {
    const ack = t(ACKS[j.assignmentAcknowledgement ?? ""] ?? "awaiting acceptance");
    [line, tone] = [t("Assigned · {name} · {ack}", { name, ack }), j.assignmentAcknowledgement === "cant_make" ? "warn" : undefined];
  } else if (j.status === "in_progress") line = t("In progress · {name}", { name });
  else if (j.status === "on_hold") [line, tone] = [t("On hold by HQ"), "warn"];
  else line = t("Rework · {name}", { name });
  const span = from && to ? Date.parse(to) - Date.parse(from) : 0;
  const elapsedPct = span > 0 ? Math.min(100, Math.max(0, Math.round(((now - Date.parse(from!)) / span) * 100))) : 0;
  const over = !!to && Date.parse(to) <= now;
  const soon = !!to && Date.parse(to) - now < 24 * 3600_000;
  return {
    id, short: id.slice(0, 8), title: j.projection === "offer" ? j.siteAddress ?? t("Accepted job") : units.get(j.unitId) ?? t("Unit"),
    line, tone, window: from && to ? { from, until: to } : null, windowText: from && to ? showSpan(from, to, display) : "—",
    left: !to ? "—" : over ? t("Ended") : t("{left} left", { left: until(to, now, t) }), ended: over, leftTone: over ? "crit" : soon ? "warn" : "primary", elapsedPct,
  };
}

export type FormMode =
  | { kind: "assign" | "reassign" | "extend"; title: string; slot: Slot; endEditable: boolean; reasonRequired: boolean; button: string; banner?: { tone: Tone; text: string }; currentTech: string | null }
  | { kind: "blocked"; title: string; slot: Slot | null; banner: { tone: Tone; text: string } };
/** What the form of the selected job allows (IR123 item 4): the agreed visit time from accepted, the same slot from
 * assigned, an extension (same start, later end, reason) while in progress, on hold or in rework; an accepted offer
 * before its delegation period and an assigned job whose agreed time has passed cannot be booked here. */
export function formMode(j: ApiDetail | ApiOffer, label: string, now: number, names: Map<string, string>, i: I18n = EN): FormMode {
  const { t, display } = i;
  const span = (s: Slot) => showSpan(s.startAt, s.endAt, display);
  const assign = t("Assign — {job}", { job: label }), reschedule = t("Reschedule — {job}", { job: label });
  if (j.projection === "offer") {
    return { kind: "blocked", title: assign, slot: j.visitSlot, banner: { tone: "primary", text: t("Accepted — the delegation period starts {time}; the technician can be assigned from then, for exactly {slot}.", { time: showTime(j.accessValidFrom, display), slot: span(j.visitSlot) }) } };
  }
  const current = j.assignment ? names.get(j.assignment.technicianMembershipId) ?? t("technician") : null;
  const slot = j.assignment ? { startAt: j.assignment.scheduledStart, endAt: j.assignment.scheduledEnd } : j.scheduledSlot ?? j.offer?.visitSlot ?? null;
  if (!slot) return { kind: "blocked", title: assign, slot: null, banner: { tone: "warn", text: t("No agreed visit time — HQ agrees one with the client first.") } };
  switch (j.status) {
    case "accepted":
      return { kind: "assign", title: assign, slot, endEditable: false, reasonRequired: false, button: t("Confirm assignment"), currentTech: null };
    case "assigned": {
      if (ended(slot, now)) {
        return { kind: "blocked", title: reschedule, slot, banner: { tone: "crit", text: t("Work window ended; reassignment required. The agreed time {slot} passed while the job is still “assigned”. A new time needs the client’s agreement — HQ proposes it (IR113); tell HQ in the job history.", { slot: span(slot) }) } };
      }
      const why = j.assignment?.cantMakeReason;
      const name = current ?? t("technician");
      return {
        kind: "reassign", title: t("Reassign — {job}", { job: label }), slot, endEditable: false, reasonRequired: false, button: t("Confirm reassignment"), currentTech: current,
        banner: j.assignment?.acknowledgement === "cant_make" ? {
          tone: "warn",
          text: why ? t("{name} can’t make this time: “{reason}”. Reassign another technician for the same agreed time, or ask HQ to propose a new time to the client.", { name, reason: why })
            : t("{name} can’t make this time. Reassign another technician for the same agreed time, or ask HQ to propose a new time to the client.", { name }),
        } : undefined,
      };
    }
    case "in_progress":
    case "on_hold":
    case "rework_requested":
      return {
        kind: "extend", title: ended(slot, now) ? reschedule : t("Reassign — {job} ({state})", { job: label, state: t(j.status === "in_progress" ? "in progress" : j.status === "on_hold" ? "on hold" : "rework") }), slot, endEditable: true, reasonRequired: true,
        button: t("Confirm reassignment"), currentTech: current,
        banner: ended(slot, now) ? { tone: "crit", text: t("Work window ended; reassignment required. The work window {slot} ended while the job is “{status}” — keep the start and set a later end (same or another technician, IR89).", { slot: span(slot), status: statusWord(j.status, t) }) } : undefined,
      };
  }
  return { kind: "blocked", title: assign, slot, banner: { tone: "muted", text: t("The job is {status} — nothing to schedule.", { status: statusWord(j.status, t) }) } };
}

/** The free time of a capacity day in Kuala Lumpur hours: the available slots minus the assigned ones (“free
 * 10:00–17:00”, “no hours”, “fully booked”). */
export function freeText(c: ApiCapacity | undefined, t: T = en): string {
  if (!c || !c.availableSlots.length) return t("no hours");
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
  return parts.length ? t("free {hours}", { hours: parts.join(", ") }) : t("fully booked");
}

export type Candidate = { id: string; name: string; sub: string; badge: { text: string; tone: Tone }; eligible: boolean };
/** The company's technicians for the slot: eligible ones (members.eligible) first, the others with the reason. */
export function candidates(members: ApiMember[], eligible: Set<string>, required: string[], slot: Slot, day: ApiCapacity[], currentId: string | null, t: T = en): Candidate[] {
  const cap = new Map(day.map((c) => [c.membershipId, c]));
  const overlaps = (a: Slot, b: Slot) => Date.parse(a.startAt) < Date.parse(b.endAt) && Date.parse(b.startAt) < Date.parse(a.endAt);
  const rows = members.filter((m) => m.role === "technician").map((m): Candidate => {
    const c = cap.get(m.id);
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => qualificationLabel(q.code, t)).join(" · ") || t("no qualifications");
    const sub = `${quals} · ${freeText(c, t)}`;
    if (eligible.has(m.id)) return { id: m.id, name: m.displayName, sub, badge: m.id === currentId ? { text: t("Current"), tone: "primary" } : { text: t("Qualified"), tone: "ok" }, eligible: true };
    const missing = required.filter((r) => !qualified(m, [r], slot.startAt)).map((code) => qualificationLabel(code, t));
    const badge = missing.length ? { text: t("Missing: {list}", { list: missing.join(", ") }), tone: "warn" as Tone }
      : c?.unavailability ? { text: t("Unavailable ({reason})", { reason: unavailabilityLabel(c.unavailability, t) }), tone: "warn" as Tone }
      : c?.assignedSlots.some((s) => overlaps(s, slot)) ? { text: t(m.id === currentId ? "Current · booked here" : "Busy at this time"), tone: "muted" as Tone }
      : { text: t("Not eligible"), tone: "muted" as Tone };
    return { id: m.id, name: m.displayName, sub, badge, eligible: false };
  });
  return rows.sort((a, b) => Number(b.eligible) - Number(a.eligible) || a.name.localeCompare(b.name));
}

export type WeekCell = { kind: "booked" | "free" | "leave" | "none" | "proposal"; text: string; sub?: string };
export type WeekRow = { id: string; name: string; sub: string; cells: WeekCell[] };
const KL = 8 * 3600_000;
const LEAVE = new Set(["annual_leave", "sick"]);
/** The team's week (Mon–Sun, Kuala Lumpur days and hours): per technician and day the booked blocks with their job,
 * leave or other unavailability, no hours or free; the selected technician's cell of the proposed slot is “This
 * proposal”. */
export function weekGrid(dates: string[], members: ApiMember[], capacity: ApiCapacity[][], jobsOf: Map<string, { short: string; slot: Slot }[]>, proposal: { technicianId: string; slot: Slot; short: string } | null, t: T = en): WeekRow[] {
  const dayOf = (iso: string) => new Date(Date.parse(iso) + KL).toISOString().slice(0, 10);
  return members.filter((m) => m.role === "technician").map((m) => {
    let avail = 0;
    const cells = dates.map((date, k): WeekCell => {
      const c = capacity[k]?.find((x) => x.membershipId === m.id);
      avail += c?.availableMinutes ?? 0;
      if (proposal && proposal.technicianId === m.id && dayOf(proposal.slot.startAt) === date) return { kind: "proposal", text: `${hhmm(proposal.slot.startAt)}–${hhmm(proposal.slot.endAt)}`, sub: proposal.short };
      if (c?.unavailability) return { kind: "leave", text: t(LEAVE.has(c.unavailability) ? "Leave" : "Unavailable"), sub: unavailabilityLabel(c.unavailability, t) };
      if (!c || !c.availableMinutes) return { kind: "none", text: t("no hours") };
      if (c.assignedSlots.length) {
        const s = c.assignedSlots[0];
        const job = (jobsOf.get(m.id) ?? []).find((j) => Date.parse(j.slot.startAt) < Date.parse(s.endAt) && Date.parse(s.startAt) < Date.parse(j.slot.endAt));
        return { kind: "booked", text: `${hhmm(s.startAt)}–${hhmm(c.assignedSlots[c.assignedSlots.length - 1].endAt)}`, sub: [job?.short ?? t("booked"), c.assignedSlots.length > 1 ? `+${c.assignedSlots.length - 1}` : ""].filter(Boolean).join(" ") };
      }
      return { kind: "free", text: t("free") };
    });
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => shortQualification(q.code, t)).join(", ");
    return { id: m.id, name: m.displayName, sub: `${quals || "—"} · ${t("{n} h avail.", { n: Math.round(avail / 60) })}`, cells };
  });
}

const FIELDS: Record<string, string> = { startAt: "Start", endAt: "End", reason: "Reason", technicianMembershipId: "Technician" };
const FIELD_ERRORS: Record<string, string> = {
  "errors.slot_not_agreed": "the slot must be the agreed visit time — a different time needs the client’s approval via HQ",
  "errors.extension_only": "keep the start and set an end at or after the current end",
  "error.required": "required (1–1000 characters)", "error.length": "at most 1000 characters", "error.range": "the end must be after the start",
};
/** Readable refusals of jobs.assign (DD-P03 boundary cases), in the display language. */
export function assignRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, tech: string, t: T = en): string {
  switch (f.messageKey) {
    case "errors.assignment_overlap": return t("Overlaps a confirmed schedule for {name} (CONFLICT) — choose another technician.", { name: tech });
    case "errors.access_window": return t("Outside the delegation period — assignments are possible only inside it.");
    case "errors.qualification_missing": return t("{name}’s qualification is not valid for this slot (revoked or expiring, FORBIDDEN) — choose another technician.", { name: tech });
    case "errors.technician_out_of_scope": case "errors.technician_not_allowed": return t("{name} cannot be assigned to this unit (FORBIDDEN).", { name: tech });
    case "error.invalidState": return t("The job can no longer be scheduled in its current state — the latest state is shown.");
    case "error.versionConflict": return t("The job changed elsewhere — the latest version is shown; check and confirm again.");
  }
  if (f.code === "NOT_FOUND") return t("The job is no longer delegated to your company.");
  if (f.code === "VALIDATION") {
    return Object.entries(f.fieldErrors).map(([k, v]) => `${FIELDS[k] ? t(FIELDS[k]) : k}: ${FIELD_ERRORS[v] ? t(FIELD_ERRORS[v]) : v}`).join(" · ") || t("Check the input.");
  }
  return `${f.code} — ${f.messageKey}`;
}
