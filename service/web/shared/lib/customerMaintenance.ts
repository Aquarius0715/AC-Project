// Customer maintenance (FR-C09, FR-C17, DD-C09, DD-C17, IR113, IR110, IR237, Figma Client 07a–07p) from the Core API:
// the status tabs and the Origin filter, one row per request with what is next, the detail (preferred times, the
// proposal to answer, the plan visit, the technician once accepted, the accepted report, the rating), the notes and
// history from the job events, and the form checks. Pure code shared by the server loader and the client view; Vitest
// covers it.
import { klTime } from "@ac/web/lib/devices";
import type { Slot } from "@ac/web/lib/partnerOverview";

export type ApiClientRow = {
  projection: "summary"; id: string; version: number; unitId: string; type: string; status: string; displayStatus: string; origin: string; dueAt: string;
  requestedSlot: Slot; scheduledSlot: Slot | null; preferredSlots: Slot[]; assignmentAcknowledgement: "pending" | "accepted" | "cant_make" | null;
};
export type ApiClientProposal = { id: string; source: "hq" | "contractor"; slot: Slot; hold: { kind: "internal" | "contractor" }; message: string; replyBy: string; status: string; decidedAt: string | null; declineReason: string | null; declineComment: string | null };
export type ApiRating = { stars: number; tags: string[]; comment: string | null; ratedAt: string; editableUntil: string };
export type ApiClientJob = {
  projection: "detail"; id: string; version: number; unitId: string; type: string; status: string; symptom: string; contactWindow: string | null; origin: string;
  planId: string | null; occurrenceAt: string | null; requestedSlot: Slot; preferredSlots: Slot[]; preferenceRound: number; slotProposal: ApiClientProposal | null;
  scheduledSlot: Slot | null; dueAt: string; completedAt: string | null; contractorOrgId: string | null; followUpOfJobId: string | null; followUpClass: "pending" | "rework" | "new_request" | null;
  rating: ApiRating | null; customerConfirmedAt: string | null; reportRefs: { reportId: string; reportVersion: number }[];
  assignment: { acknowledgement: "pending" | "accepted" | "cant_make"; technicianName: string | null; scheduledStart: string; scheduledEnd: string; status: string } | null;
};

const md = (iso: string) => klTime(iso).slice(5, 10);
const hm = (iso: string) => klTime(iso).slice(11);
const oneDay = (s: Slot) => klTime(s.startAt).slice(0, 10) === klTime(s.endAt).slice(0, 10);
const weekday = (iso: string) => new Date(Date.parse(iso) + 8 * 3600_000).toLocaleDateString("en-GB", { weekday: "short", timeZone: "UTC" });
/** “09-22 10:00–12:00”, or both ends across days (“09-14 08:00 → 09-20 08:00”). */
export const slotShort = (s: Slot) => (oneDay(s) ? `${md(s.startAt)} ${hm(s.startAt)}–${hm(s.endAt)}` : `${md(s.startAt)} ${hm(s.startAt)} → ${md(s.endAt)} ${hm(s.endAt)}`);
/** “Tue 09-22 · 10:00–12:00”, or both ends across days. */
export const slotLong = (s: Slot) => (oneDay(s) ? `${weekday(s.startAt)} ${md(s.startAt)} · ${hm(s.startAt)}–${hm(s.endAt)}` : `${weekday(s.startAt)} ${md(s.startAt)} ${hm(s.startAt)} → ${weekday(s.endAt)} ${md(s.endAt)} ${hm(s.endAt)}`);

/** The job history in the client's words (the shared titles are written for the service partner). */
const CLIENT_TITLE: Record<string, string> = {
  "job.created": "Request created", "job.offered": "HQ booked a service partner", "offer.accepted": "Service partner accepted", "offer.declined": "Service partner declined — HQ is rebooking",
  offer_expired: "HQ is rebooking", "offer.access_extended": "Service partner’s access extended", "job.assigned": "Technician assigned", "assignment.accepted": "Technician confirmed the time",
  "assignment.cant_make": "Technician can’t make it — HQ is rebooking", "partner.review": "Report checked by the service partner", "job.reviewed": "Report reviewed by HQ",
  "job.rated": "You rated the job", "job.problem_reported": "You reported a problem", "proposal.sent": "New time proposed", "proposal.accepted": "You accepted the proposed time",
  "proposal.declined": "You declined the proposed time", "job.reschedule_requested": "You asked for another time",
};
export const clientEventTitle = (action: string, shared: Record<string, string>) => CLIENT_TITLE[action] ?? shared[action] ?? action.replace(/[._]/g, " ");
export const sameSlot = (a: Slot | null, b: Slot | null) => !!a && !!b && Date.parse(a.startAt) === Date.parse(b.startAt) && Date.parse(a.endAt) === Date.parse(b.endAt);

export type StatusTab = "all" | "reply" | "requested" | "scheduled" | "progress" | "completed" | "cancelled";
export const STATUS_TABS: [StatusTab, string][] = [["all", "All"], ["reply", "Needs your reply"], ["requested", "Requested"], ["scheduled", "Scheduled"], ["progress", "In progress"], ["completed", "Completed"], ["cancelled", "Cancelled"]];
/** The tab of a request: Needs your reply = a pending proposal; Scheduled = a time is agreed (offered … assigned). */
export const tabOf = (displayStatus: string): StatusTab =>
  displayStatus === "time_proposed" ? "reply" : displayStatus === "requested" ? "requested" : ["offered", "accepted", "assigned"].includes(displayStatus) ? "scheduled"
    : displayStatus === "completed" ? "completed" : displayStatus === "cancelled" ? "cancelled" : "progress";
const ORDER = ["time_proposed", "requested", "in_progress", "rework_requested", "submitted", "offered", "accepted", "assigned", "on_hold", "completed", "cancelled"];
export const tabOfQuery = (v?: string): StatusTab => (STATUS_TABS.some(([t]) => t === v) ? (v as StatusTab) : "all");

export type ClientRow = { id: string; short: string; unit: string; place: string; origin: "plan" | "request"; type: string; status: string; tab: StatusTab; line: string; warn: boolean };
const TYPE: Record<string, string> = { reactive: "Repair", preventive: "Preventive maintenance", periodic: "Periodic inspection" };
/** One request of the list (Figma 07a): unit and place, origin, type and what is next — business status order. */
export function clientRows(rows: ApiClientRow[], unit: (id: string) => { name: string; place: string }): ClientRow[] {
  return [...rows].sort((a, b) => ORDER.indexOf(a.displayStatus) - ORDER.indexOf(b.displayStatus) || Date.parse(a.dueAt) - Date.parse(b.dueAt)).map((j) => {
    const u = unit(j.unitId);
    const plan = j.origin === "periodic_plan";
    let line = "", warn = false;
    switch (j.displayStatus) {
      case "time_proposed": [line, warn] = ["⇄ A new visit time needs your reply — nothing is booked until you accept", true]; break;
      case "requested": line = j.preferredSlots.length ? `◷ Preferred ${slotShort(j.preferredSlots[0])} · +${j.preferredSlots.length - 1} more (not confirmed yet)` : plan ? `◷ Plan visit ${slotShort(j.requestedSlot)} · HQ is booking it` : "◷ Waiting for HQ"; break;
      case "completed": line = `✓ Completed${j.scheduledSlot ? ` · visit ${slotShort(j.scheduledSlot)}` : ""} · report available`; break;
      case "cancelled": line = "Cancelled"; break;
      case "offered": case "accepted": line = `◷ ${plan ? "Scheduled" : "Confirmed"} ${j.scheduledSlot ? slotShort(j.scheduledSlot) : ""} · booking the technician`; break;
      default: line = `◷ ${plan ? "Scheduled" : "Confirmed"} ${j.scheduledSlot ? slotShort(j.scheduledSlot) : ""}${j.assignmentAcknowledgement === "accepted" ? " · technician confirmed" : ""}`;
    }
    return { id: j.id, short: j.id.slice(0, 8), unit: u.name, place: u.place, origin: plan ? "plan" : "request", type: plan ? "Periodic inspection" : TYPE[j.type] ?? j.type, status: j.displayStatus, tab: tabOf(j.displayStatus), line, warn };
  });
}

/** The detail facts (Figma 07b): unit, origin and type, follow-up, request, the agreed time, the technician once accepted. */
export function detailFacts(j: ApiClientJob, unit: { name: string; place: string }): [string, string][] {
  const plan = j.origin === "periodic_plan";
  const out: [string, string][] = [["Unit", `${unit.name}${unit.place ? ` · ${unit.place}` : ""}`], ["Origin · type", plan ? `Periodic plan · visit ${j.occurrenceAt ? md(j.occurrenceAt) : ""}` : `Client request · ${TYPE[j.type] ?? j.type}`]];
  if (j.followUpOfJobId) out.push(["Follow-up of", `${j.followUpOfJobId.slice(0, 8)} · ${followUpText(j.followUpClass)}`]);
  if (!plan && j.symptom) out.push(["Request", `“${j.symptom}”`]);
  if (j.contactWindow) out.push(["Contact window", j.contactWindow]);
  if (j.scheduledSlot && j.slotProposal?.status !== "pending") out.push([plan ? "Scheduled" : "Confirmed time", `${slotLong(j.scheduledSlot)}${plan ? " · set by HQ from your plan" : ""}`]);
  const a = j.assignment;
  if (a?.acknowledgement === "accepted") out.push(["Technician", `${a.technicianName ?? "your technician"} · ${j.contractorOrgId ? "service partner" : "HQ"}`]);
  else if (j.scheduledSlot && ["offered", "accepted", "assigned"].includes(j.status)) out.push(["Technician", "being confirmed — the name shows once the technician accepts"]);
  return out;
}
export const followUpText = (c: ApiClientJob["followUpClass"]) => (c === "rework" ? "Rework (free)" : c === "new_request" ? "New request" : "Under HQ review");

/** The preferred times with the booked one marked (request origin, open jobs). */
export function preferredLines(j: ApiClientJob): { text: string; note: string | null }[] {
  if (j.origin === "periodic_plan" || ["completed", "cancelled"].includes(j.status)) return [];
  return j.preferredSlots.map((s, i) => ({ text: `${["1st", "2nd", "3rd"][i] ?? `${i + 1}th`} · ${slotLong(s)}`, note: sameSlot(s, j.scheduledSlot) ? "✓ booked" : null }));
}

/** The proposal to answer (Figma 07k): who proposes, the time, who comes, the message and the reply deadline. */
export function proposalCard(j: ApiClientJob, now: number) {
  const p = j.slotProposal;
  if (!p || p.status !== "pending") return null;
  return {
    id: p.id, title: p.source === "contractor" ? "Your service partner asks for another time (via HQ)" : "HQ proposes a new time", when: slotLong(p.slot),
    who: p.hold.kind === "contractor" ? "A service partner technician (name shown once assigned)" : "An HQ technician (name shown once confirmed)",
    message: p.message, replyBy: klTime(p.replyBy).slice(5), late: Date.parse(p.replyBy) <= now,
  };
}
/** The declined-proposal notice shown while HQ looks again (Figma 07m). */
export const declinedNotice = (j: ApiClientJob) => (j.status === "requested" && j.slotProposal?.status === "declined"
  ? `You declined the proposed time (${(j.slotProposal.declineReason ?? "other").replace(/_/g, " ")}). HQ is looking at your new times.` : null);

/** A plan visit the client may move (Figma 07n): an agreed plan visit at least 48 h ahead (IR113). */
export function planVisit(j: ApiClientJob, now: number): { can: boolean; why: string | null } | null {
  if (j.origin !== "periodic_plan" || !["offered", "accepted", "assigned"].includes(j.status)) return null;
  const start = j.scheduledSlot ? Date.parse(j.scheduledSlot.startAt) : null;
  if (start !== null && start - now < 48 * 3600_000) return { can: false, why: "Less than 48 h before the visit — contact HQ to change it." };
  return { can: true, why: null };
}

/** What a completed job offers (IR110, DD-C17): Confirm & rate for 7 days, editing the rating until editableUntil. */
export function feedback(j: ApiClientJob, now: number): { kind: "rate" | "edit" | "rated" | "none"; text: string } {
  if (j.status !== "completed") return { kind: "none", text: "" };
  const r = j.rating;
  if (!r) {
    const open = j.completedAt ? now < Date.parse(j.completedAt) + 7 * 86_400_000 : true;
    return open ? { kind: "rate", text: "Is everything working? Your confirmation closes the job." } : { kind: "none", text: "Confirmed automatically after 7 days." };
  }
  const editable = now < Date.parse(r.editableUntil);
  return { kind: editable ? "edit" : "rated", text: `Rated ${"★".repeat(r.stars)}${r.tags.length ? ` · ${r.tags.join(", ")}` : ""}${editable ? ` · editable until ${klTime(r.editableUntil).slice(5)}` : ""}` };
}
export const RATING_TAGS = ["On time", "Clean work", "Explained clearly", "Polite", "Fixed the problem"];
export const PROBLEMS: { id: "same_problem" | "new_damage" | "not_completed" | "other"; label: string }[] = [
  { id: "same_problem", label: "Same problem again" }, { id: "new_damage", label: "New damage" }, { id: "not_completed", label: "Work not completed" }, { id: "other", label: "Other" },
];

/** The 3 preferred times (IR113 item 2): exactly 3, each 1–4 h, starting on a later day than today, all different. */
export function preferredError(slots: Slot[], now: number, count = 3): string | null {
  if (slots.length !== count) return `Give ${count} preferred times.`;
  const today = new Date(now + 8 * 3600_000).toISOString().slice(0, 10);
  const keys = new Set<string>();
  for (const s of slots) {
    const h = (Date.parse(s.endAt) - Date.parse(s.startAt)) / 3600_000;
    if (!(h >= 1 && h <= 4)) return "Each time is 1–4 hours long.";
    if (new Date(Date.parse(s.startAt) + 8 * 3600_000).toISOString().slice(0, 10) <= today) return "Times start on a later day than today.";
    const k = `${Date.parse(s.startAt)}-${Date.parse(s.endAt)}`;
    if (keys.has(k)) return "The 3 times must be different.";
    keys.add(k);
  }
  return null;
}
/** The contact window rule (IR64, IR90): up to 200 characters without an e-mail address or a phone number. */
export function contactError(v: string): string | null {
  const t = v.trim();
  if (t.length > 200) return "Up to 200 characters.";
  if (t.includes("@") || /\d{7,}/.test(t.replace(/[\s\-()+]/g, ""))) return "No e-mail addresses or phone numbers — use HH:mm for times (example: Weekdays 09:00-18:00).";
  return null;
}

/** The client's notes and the job history from jobs.events (internal notes never reach the client, IR42). */
export type ApiClientEvent = { id: string; action: string; occurredAt: string; actorUserId: string | null; note: { visibility: string; message: string } | null };
export function notesOf(events: ApiClientEvent[], me: string): { id: string; who: string; at: string; text: string }[] {
  return events.filter((e) => e.note && e.note.visibility === "customer").sort((a, b) => Date.parse(a.occurredAt) - Date.parse(b.occurredAt))
    .map((e) => ({ id: e.id, who: e.actorUserId === me ? "You" : "Coordinator", at: klTime(e.occurredAt).slice(5), text: e.note!.message }));
}

/** Readable refusals of the client's maintenance actions. */
export function clientJobRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  const keys: Record<string, string> = {
    "error.invalidState": "not possible in the request's current state", "error.versionConflict": "the request changed meanwhile — the latest state is shown",
    "errors.reschedule_too_late": "less than 48 h before the visit — contact HQ", "errors.rating_locked": "the rating can no longer be changed",
    "errors.proposal_expired": "the proposal has expired — HQ will look again", "error.slotRules": "the times break the rules (1–4 h, from tomorrow, all different)",
    "errors.contact_details_forbidden": "no e-mail addresses or phone numbers in the contact window", "error.length": "wrong length", "error.count": "wrong number of entries",
    "error.invalidFile": "photos must be JPEG or PNG up to 5 MiB", "error.unitArchived": "the unit is archived", "error.required": "required", "error.invalid": "not a valid value",
  };
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${keys[v] ?? v}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return `${f.code === "CONFLICT" ? "Not saved: " : ""}${keys[f.messageKey]}.`;
  if (f.code === "NOT_FOUND") return "The request no longer exists.";
  return `${f.code} — ${f.messageKey}`;
}
