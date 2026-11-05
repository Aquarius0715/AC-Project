// HQ maintenance jobs (FR-A06, DD-A06, Figma Admin 06-1, 06-11…06-18) from the Core API: the pipeline counts and
// filters of the Jobs tab, one row per job, and for the selected job the status stepper, the key facts, the delivery
// (internal assignment or contractor offer with the technician's acknowledgement), the client's preferred times with
// the HQ technicians free for each (IR113), the proposal and partner time-change cards, and the rules of hold, cancel
// (IR56), follow-up classification (IR114), the submitted report with the HQ review mode (IR31), the cost lines with
// their totals per currency and the New job form. Pure code shared by the server loader and the client view; Vitest
// covers it. Texts in the display language (`i` / `t`, IR290); instants in the user's display time zone (IR44), and the
// times HQ types are read in it too (NFR-08). Business days (a follow-up's classify-by day, "from tomorrow") stay
// Kuala Lumpur days.
import { EN, showSpan, showTime, translator, zonedInstant, zonedParts, type I18n, type T, textOf } from "@ac/web/lib/i18n";
import { actionMessage, fieldName } from "@ac/web/lib/actionMessage";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { Slot } from "@ac/web/lib/partnerOverview";
import { inspectionRows, readingRows, resultText, reworkItems, type ApiWorkReport } from "@ac/web/lib/partnerReview";

export interface ApiHqRow {
  projection: "summary"; id: string; version: number; unitId: string; type: string; status: string; dueAt: string; requestedSlot: Slot; scheduledSlot: Slot | null;
  assignmentId: string | null; origin: string; preferredSlots: Slot[]; severity: string; displayStatus: string; assignmentAcknowledgement: "pending" | "accepted" | "cant_make" | null;
  technicianMembershipId: string | null; accessValidFrom: string | null; accessValidUntil: string | null;
  createdAt: string; followUpOfJobId: string | null; followUpClass: "pending" | "rework" | "new_request" | null;
}
export interface ApiSlotProposal { id: string; source: "hq" | "contractor"; slot: Slot; hold: { kind: "internal"; membershipId: string } | { kind: "contractor"; contractorOrgId: string; technicianMembershipId: string | null }; message: string; replyBy: string; status: string; decidedAt: string | null; declineReason: string | null; declineComment: string | null }
export interface ApiPartnerProposal { id: string; offerId: string; slot: Slot; technicianMembershipId: string; reason: string; sentAt: string; status: string }
export interface ApiHqJob {
  projection: "detail"; id: string; version: number; unitId: string; type: string; status: string; symptom: string; contactWindow: string | null; origin: string; planId: string | null; occurrenceAt: string | null;
  requestedSlot: Slot; preferredSlots: Slot[]; preferenceRound: number; slotProposal: ApiSlotProposal | null; partnerSlotProposal: ApiPartnerProposal | null; scheduledSlot: Slot | null;
  dueAt: string; startedAt: string | null; completedAt: string | null; contractorOrgId: string | null; createdAt: string; followUpOfJobId: string | null; followUpClass: "pending" | "rework" | "new_request" | null;
  /** the photos the client sent with the problem report (IR437); absent in the demo data */
  problemPhotos?: { id: string; name: string; status: string }[];
  assignment: { technicianMembershipId: string; scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: string; cantMakeReason: string | null; alternativeSlot: Slot | null } | null;
  offer: { id: string; contractorOrgId: string; visitSlot: Slot; offerExpiresAt: string; accessValidFrom: string; accessValidUntil: string; decision: string | null; decidedAt: string | null; declineReason: string | null } | null;
  reportRefs: { reportId: string; reportVersion: number }[]; draftReportRef: { reportId: string; reportVersion: number } | null; costs: CostLine[] | null;
}
export interface Names { units: Map<string, string>; customers: Map<string, string>; people: Map<string, string>; orgs: Map<string, string> }

type Tone = "ok" | "warn" | "crit" | "primary" | "muted";
const en = translator("en");
/** “15 Sept, 10:00 am – 12:00 pm MYT” — a slot as one span in the display time zone. */
export const slotText = (s: Slot, i: I18n = EN) => showSpan(s.startAt, s.endAt, i.display);
/** “Mon, 28 Sept, 2:00 – 4:00 pm MYT” for the preferred-times table and the dialogs: the span with its weekday. */
export const longSlot = (s: Slot, i: I18n = EN) => showSpan(s.startAt, s.endAt, i.display, true);
/** A datetime-local value in the display time zone, and back (the times HQ types, NFR-08). */
export const zonedInput = (iso: string, zone: string) => { const p = zonedParts(iso, zone); return `${p.date}T${p.time}`; };
export const fromZonedInput = (v: string, zone: string) => zonedInstant(v.slice(0, 10), v.slice(11, 16), zone);
/** Tomorrow in the display time zone (YYYY-MM-DD): the date the time dialogs start on. A time from 10:00 that day always
starts on a later Kuala Lumpur day than now, which IR113 item 2 asks of new preferred times. */
export const tomorrowIn = (now: number, zone: string) => zonedParts(new Date(now + 86_400_000).toISOString(), zone).date;

/** The pipeline stages with their jobs.list filter; Time proposed is a requested job with a pending SlotProposal (IR113). */
export const STAGES = [
  { id: "requested", label: "Requested", filter: { status: "requested", proposalPending: false } },
  { id: "time_proposed", label: "Time proposed", filter: { proposalPending: true } },
  { id: "offered", label: "Offered", filter: { status: "offered" } }, { id: "accepted", label: "Accepted", filter: { status: "accepted" } },
  { id: "assigned", label: "Assigned", filter: { status: "assigned" } }, { id: "in_progress", label: "In progress", filter: { status: "in_progress" } },
  { id: "submitted", label: "Submitted", filter: { status: "submitted" } }, { id: "rework_requested", label: "Rework", filter: { status: "rework_requested" } },
  { id: "completed", label: "Completed", filter: { status: "completed" } }, { id: "on_hold", label: "On hold", filter: { status: "on_hold" } },
] as const;
export type Stage = (typeof STAGES)[number]["id"];
export const stageOf = (v?: string): Stage | null => (STAGES.some((s) => s.id === v) ? (v as Stage) : null);
export const SORTS = [{ id: "status", text: "Status, earliest stage first", field: "status", direction: "asc" }, { id: "dueAt", text: "Deadline, soonest first", field: "dueAt", direction: "asc" }, { id: "severity", text: "Severity, most severe first", field: "severity", direction: "desc" }] as const;
export type HqSort = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): HqSort => (SORTS.some((s) => s.id === v) ? (v as HqSort) : "status");

export interface Query { customerId?: string; propertyId?: string; unitId?: string; origin?: string; delivery?: string; assignee?: string; overdue?: string; type?: string; stage?: string; from?: string; to?: string }
/** The requested-time period of the list (URL from / to, ISO instants; the HQ overview's job counts, IR245): both valid
and from < to, else none. */
export function periodOfQuery(q: Pick<Query, "from" | "to">): { from: string; to: string } | null {
  const f = q.from ? Date.parse(q.from) : NaN, t = q.to ? Date.parse(q.to) : NaN;
  return Number.isFinite(f) && Number.isFinite(t) && f < t ? { from: new Date(f).toISOString(), to: new Date(t).toISOString() } : null;
}
/** “Requested time 8 Sept, 12:00 am – 14 Sept, 12:09 pm MYT” for the period chip. */
export const periodChip = (p: { from: string; to: string }, i: I18n = EN) => i.t("Requested time {span}", { span: showSpan(p.from, p.to, i.display) });
/** Every status but cancelled: the list without a stage tile, and the jobs counted "in scope" (no tile names cancelled). */
export const LISTED = ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "rework_requested", "completed", "on_hold"];
const TYPE_IDS = new Set(["reactive", "periodic", "preventive"]);
/** The jobs.list filters of the scope and the filter bar; delivery is the HQ organization (internal) or a contractor.
The type and the default "not cancelled" are filters of jobs.list too (IR290), so its total counts what the list shows. */
export function filtersOf(q: Query, hqOrgId: string | null, withStage = true): Record<string, unknown> {
  const f: Record<string, unknown> = {};
  if (q.customerId) f.customerId = q.customerId;
  if (q.propertyId) f.propertyId = q.propertyId;
  if (q.unitId) f.unitId = q.unitId;
  if (q.origin === "client_request" || q.origin === "periodic_plan") f.origin = q.origin;
  if (q.type && TYPE_IDS.has(q.type)) f.type = q.type;
  if (q.delivery === "internal" && hqOrgId) f.organizationId = hqOrgId;
  else if (q.delivery && q.delivery !== "internal") f.organizationId = q.delivery;
  if (q.assignee) f.membershipId = q.assignee;
  if (q.overdue === "1") f.overdueOnly = true;
  const period = periodOfQuery(q);
  if (period) Object.assign(f, period); // jobs.list from / to: the requested slot's start (IR245)
  const stage = withStage ? STAGES.find((s) => s.id === q.stage) : undefined;
  if (stage) Object.assign(f, stage.filter);
  else if (withStage) f.statuses = LISTED;
  return f;
}

export interface HqRow { id: string; short: string; title: string; customer: string; type: string; origin: string; status: string; badge: { status: string; overdue: boolean }; line: string; lineTone?: Tone }
const ACK: Record<string, string> = { accepted: "accepted", cant_make: "can’t make it" };
const ack = (a: string | null, t: T) => textOf(ACK, a, t) ?? t("awaiting acceptance");
/** What HQ does next with a requested job: book a preferred time, the plan occurrence, or wait for the due time. */
function requestedLine(j: ApiHqRow, i: I18n): string {
  const { t } = i;
  const n = j.preferredSlots.length;
  if (n === 1) return t("1 preferred time · book one or propose", { n });
  if (n > 1) return t("{n} preferred times · book one or propose", { n });
  if (j.origin === "periodic_plan") return t("plan occurrence {slot} · book it", { slot: slotText(j.requestedSlot, i) });
  return t("due {time} · unassigned", { time: showTime(j.dueAt, i.display) });
}
/** One job of the list: unit, customer, type, origin, status (Time proposed derived, IR89 overdue) and what is next. */
export function hqRow(j: ApiHqRow, now: number, n: Names, unitOrg: Map<string, string>, i: I18n = EN): HqRow {
  const { t } = i;
  const tech = j.technicianMembershipId ? n.people.get(j.technicianMembershipId) ?? t("technician") : null;
  const isOverdue = (j.status === "assigned" || j.status === "in_progress") && !!j.scheduledSlot && Date.parse(j.scheduledSlot.endAt) <= now;
  const ended = isOverdue ? ` · ${t("window ended")}` : "";
  let line = "", lineTone: Tone | undefined;
  switch (j.displayStatus === "time_proposed" ? "time_proposed" : j.status) {
    case "requested": { line = requestedLine(j, i); break;
    }
    case "time_proposed": { line = t("Time proposed · waiting for the client"); break;
    }
    case "offered": { line = j.scheduledSlot ? t("Offer open · {slot}", { slot: slotText(j.scheduledSlot, i) }) : t("Offer open"); break;
    }
    case "accepted": { line = t("Contractor accepted · assigning their technician"); break;
    }
    case "assigned": { [line, lineTone] = [`${tech ?? t("technician")} · ${ack(j.assignmentAcknowledgement, t)}${ended}`, j.assignmentAcknowledgement === "cant_make" ? "warn" : undefined]; break;
    }
    case "in_progress": { line = `${t("{name} on site", { name: tech ?? t("technician") })}${ended}`; break;
    }
    case "submitted": { line = t(j.accessValidUntil ? "report submitted · contractor review first" : "report submitted · awaiting HQ review"); break;
    }
    case "rework_requested": { line = t("returned for rework"); break;
    }
    case "on_hold": { [line, lineTone] = [t("on hold — scheduling and reports blocked"), "warn"]; break;
    }
    case "completed": { line = statusWord("completed", t); break;
    }
    case "cancelled": { line = statusWord("cancelled", t); break;
    }
  }
  // a client's follow-up waits for HQ's classification within one business day (IR114, Figma Admin 06-17)
  if (j.followUpClass === "pending" && j.followUpOfJobId) [line, lineTone] = [t("Follow-up of {id} · classify by {time}", { id: j.followUpOfJobId.slice(0, 8), time: classifyBy(j.createdAt, i) }), "warn"];
  return {
    id: j.id, short: j.id.slice(0, 8), title: n.units.get(j.unitId) ?? t("Unit"), customer: n.customers.get(unitOrg.get(j.unitId) ?? "") ?? t("customer"),
    type: typeLabel(j.type, t), origin: j.origin, status: j.displayStatus === "time_proposed" ? "time_proposed" : j.status, badge: { status: j.displayStatus === "time_proposed" ? "time_proposed" : j.status, overdue: isOverdue }, line, lineTone,
  };
}

export interface Step { label: string; state: "done" | "current" | "todo" }
/** The status stepper: internal delivery skips Offered / Accepted (DD-A06 item 6). */
export function stepper(status: string, internal: boolean, t: T = en): Step[] {
  const all = internal ? ["requested", "assigned", "in_progress", "submitted", "completed"] : ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed"];
  const labels: Record<string, string> = { requested: "Requested", offered: "Offered", accepted: "Accepted", assigned: "Assigned", in_progress: "In progress", submitted: "Submitted", completed: "Completed" };
  // a returned or held job is back at in progress; a cancelled one reaches no step
  const reached = status === "rework_requested" || status === "on_hold" ? "in_progress" : status;
  const at = status === "cancelled" ? -1 : all.indexOf(reached);
  const stateOf = (k: number): Step["state"] => {
    if (status === "completed" || k < at) return "done";
    return k === at ? "current" : "todo";
  };
  return all.map((s, k) => ({ label: t(labels[s] ?? s), state: stateOf(k) }));
}

/** The four facts of the detail: requested window, due, scheduled slot, symptom with the contact window. */
export function facts(j: ApiHqJob, n: Names, i: I18n = EN): { label: string; value: string; sub: string }[] {
  const { t } = i;
  const dueWhy = j.type === "reactive" ? "reactive SLA" : "plan occurrence";
  const offeredTo = j.offer ? n.orgs.get(j.offer.contractorOrgId) ?? t("contractor") : "—";
  return [
    { label: t("Requested window"), value: slotText(j.requestedSlot, i), sub: t(j.origin === "periodic_plan" ? "the plan occurrence" : "the client’s 1st preferred time") },
    { label: t("Due"), value: showTime(j.dueAt, i.display), sub: t(j.dueAt === j.requestedSlot.endAt ? "= requested end" : dueWhy) },
    { label: t("Scheduled"), value: j.scheduledSlot ? slotText(j.scheduledSlot, i) : t("not booked"), sub: j.assignment ? n.people.get(j.assignment.technicianMembershipId) ?? t("technician") : offeredTo },
    { label: t("Symptom"), value: j.symptom || "—", sub: j.contactWindow ? t("contact {window}", { window: j.contactWindow }) : t("no contact window") },
  ];
}

export interface Delivery { kind: "internal" | "contractor"; title: string; lines: [string, string][]; ack: { tone: Tone; text: string } | null; cantMake: boolean }
/** The technician's answer to an assignment: completed, accepted, can't make it (with reason and another time), or waiting. */
function acknowledgement(a: NonNullable<ApiHqJob["assignment"]>, tech: string, i: I18n): { tone: Tone; text: string } {
  const { t } = i;
  if (a.status === "completed") return { tone: "ok", text: t("{name} completed the job — the assignment ended with it, so {name}’s time is free again.", { name: tech }) };
  if (a.acknowledgement === "accepted") return { tone: "ok", text: t("{name} accepted the assignment", { name: tech }) };
  if (a.acknowledgement !== "cant_make") return { tone: "primary", text: t("Waiting for {name} to accept.", { name: tech }) };
  const why = a.cantMakeReason ? t("{name} can’t make this time: “{reason}”", { name: tech, reason: a.cantMakeReason }) : t("{name} can’t make this time", { name: tech });
  const other = a.alternativeSlot ? ` · ${t("could do {slot}", { slot: slotText(a.alternativeSlot, i) })}` : "";
  return { tone: "warn", text: `${why}${other}. ${t("Any new time needs the client’s approval.")}` };
}
/** Where an offer stands: accepted (when), expired, or open until it expires. */
function offerState(o: NonNullable<ApiHqJob["offer"]>, now: number, i: I18n): string {
  const { t, display } = i;
  if (o.decision === "accept") return o.decidedAt ? t("accepted {time}", { time: showTime(o.decidedAt, display) }) : t("accepted");
  return Date.parse(o.offerExpiresAt) <= now ? t("expired") : t("offer open · expires {time}", { time: showTime(o.offerExpiresAt, display) });
}
/** Who does the work: the internal assignment, or the contractor's offer with its access window and the technician. */
export function delivery(j: ApiHqJob, n: Names, now: number, i: I18n = EN): Delivery | null {
  const { t, display } = i;
  const a = j.assignment, o = j.offer;
  const tech = a ? n.people.get(a.technicianMembershipId) ?? t("technician") : "";
  const window = a ? slotText({ startAt: a.scheduledStart, endAt: a.scheduledEnd }, i) : "";
  const done = a?.status === "completed" ? ` · ${t("ended at completion")}` : "";
  const ackLine = a ? acknowledgement(a, tech, i) : null;
  if (o && j.contractorOrgId) {
    const org = n.orgs.get(o.contractorOrgId) ?? t("contractor");
    const state = offerState(o, now, i);
    return {
      kind: "contractor", title: t("Delivery · contractor {name}", { name: org }),
      lines: [[t("Visit"), `${slotText(o.visitSlot, i)} · ${state}`], [t("Access"), `${showSpan(o.accessValidFrom, o.accessValidUntil, display)}${o.decision === "accept" && Date.parse(o.accessValidUntil) <= now ? ` · ${t("ended")}` : ""}`],
        [t("Technician"), a ? `${tech} · ${window}${done}` : t("not assigned by the contractor yet")]],
      ack: ackLine, cantMake: a?.status !== "completed" && a?.acknowledgement === "cant_make",
    };
  }
  return a ? { kind: "internal", title: t("Delivery · internal"), lines: [[t("Technician"), `${tech} · ${t("assignment {slot}", { slot: window })}${done}`]], ack: ackLine, cantMake: a.status !== "completed" && a.acknowledgement === "cant_make" } : null;
}

/** The times HQ may book (IR113): the client's preferred times, or — for a plan's job without them — the plan
occurrence (the requested window the plan generated, D16). */
export function agreedSlots(j: Pick<ApiHqJob, "preferredSlots" | "planId" | "occurrenceAt" | "requestedSlot">): Slot[] {
  if (j.preferredSlots.length > 0) return j.preferredSlots;
  return j.planId && j.occurrenceAt ? [j.requestedSlot] : [];
}

export interface PreferredRow { rank: number; slot: Slot; text: string; hq: string; fits: boolean; people: { id: string; displayName: string }[]; plan: boolean }
/** The agreed times with the HQ technicians free and qualified for each (members.eligible). */
export function preferredRows(j: ApiHqJob, eligible: { id: string; displayName: string }[][], now: number, i: I18n = EN): PreferredRow[] {
  const { t } = i;
  const isPlan = j.preferredSlots.length === 0;
  return agreedSlots(j).map((s, k) => {
    const people = eligible[k] ?? [];
    const isPast = Date.parse(s.startAt) <= now;
    const free = people.length > 0 ? people.map((p) => p.displayName).join(", ") : t("no qualified HQ technician free");
    return { rank: k + 1, slot: s, text: longSlot(s, i), hq: isPast ? t("in the past") : free, fits: !isPast && people.length > 0, people: isPast ? [] : people, plan: isPlan };
  });
}

/** The active contractors any agreed time can be offered to, by name: the first three and how many more, so the
booking table keeps its times and HQ technicians readable however many companies HQ works with (IR572). */
export function contractorNames(names: string[], t: T = en): string {
  const sorted = [...names].toSorted((a, b) => a.localeCompare(b));
  return sorted.length > 3 ? t("{names} and {n} more", { names: sorted.slice(0, 3).join(", "), n: sorted.length - 3 }) : sorted.join(", ") || "—";
}

/** The default windows of an HQ offer (IR141 item 2): answer within 24 h (not after the visit), access from now to a day after. */
export function offerDefaults(slot: Slot, now: number): { offerExpiresAt: string; accessValidFrom: string; accessValidUntil: string } {
  const expires = Math.min(now + 24 * 3_600_000, Date.parse(slot.startAt));
  return { offerExpiresAt: new Date(expires).toISOString(), accessValidFrom: new Date(now).toISOString(), accessValidUntil: new Date(Date.parse(slot.endAt) + 24 * 3_600_000).toISOString() };
}

/** Which of hold / resume / cancel HQ may use (IR56; hold only from in progress or submitted). */
export function controls(status: string, t: T = en): { hold: boolean; resume: boolean; cancel: boolean; cancelWhy: string | null } {
  const isCancel = ["requested", "offered", "accepted", "assigned", "on_hold", "rework_requested"].includes(status);
  const isRunning = status === "in_progress" || status === "submitted";
  const why = isRunning ? "Put the job on hold first." : "Completed and cancelled jobs cannot be cancelled.";
  const cancelWhy = isCancel ? null : t(why);
  return { hold: status === "in_progress" || status === "submitted", resume: status === "on_hold", cancel: isCancel, cancelWhy };
}

/** The classify-by time of a follow-up: one business day (Mon–Fri in Kuala Lumpur) after it was created, as an instant
in the display time zone (display only, IR114). */
export function classifyBy(createdAt: string, i: I18n = EN): string {
  const d = new Date(Date.parse(createdAt) + 8 * 3_600_000);
  let add = 1;
  while (add > 0) { d.setUTCDate(d.getUTCDate() + 1); if (d.getUTCDay() !== 0 && d.getUTCDay() !== 6) add--; }
  return showTime(new Date(d.getTime() - 8 * 3_600_000).toISOString(), i.display);
}

/** The HQ job refusals in words, by message key (English keys of the display-language dictionary). */
const HQ_REFUSAL_TEXT: Record<string, string> = {
  "errors.slot_not_agreed": "only one of the client’s preferred times (or an accepted proposal) can be booked",
  "errors.slot_is_preferred": "that is one of the client’s own times — book it directly instead",
  "errors.proposal_pending": "a proposal is already waiting for the client — withdraw it first",
  "errors.assignment_overlap": "the technician has a confirmed job at that time",
  "errors.qualification_missing": "the technician is not qualified for this unit at that time",
  "errors.technician_out_of_scope": "the technician’s scope does not include this unit",
  "errors.contractor_suspended": "offers to this contractor are suspended",
  "errors.reschedule_too_late": "too late to change the time",
  "error.invalidState": "not allowed in the job’s current state",
  "error.versionConflict": "the job changed meanwhile — the latest state is shown",
  "errors.review_mode": "HQ reviews its internal jobs, and a contractor's jobs only by escalation with a reason",
  "errors.self_review": "you contributed to this report version, so another reviewer must decide (self-approval is rejected)",
  "errors.report_version_changed": "a newer report version exists — the latest one is shown",
  "errors.no_accepted_offer": "only the access of an accepted offer can be extended",
  "error.mustExtend": "the new end must be later than the current end and in the future",
  "error.unitArchived": "the unit is archived",
  "error.slotRules": "each time is 1–4 hours, starts on a later day than today, and the times differ",
  "error.beforeRequestedEnd": "the due time cannot be before the end of the 1st preferred time",
  "errors.contact_details_forbidden": "no e-mail addresses or phone numbers in the contact window",
  "error.count": "too many entries", "error.invalid": "not a valid value",
  "error.length": "1–1000 characters", "error.required": "required", "error.range": "the end must be after the start", "error.past": "must be in the future",
  "error.afterAccess": "must not be after the access ends", "error.afterVisit": "must not be after the visit starts",
};
/** A field's own limit where it differs from the general one. */
const HQ_FIELD_TEXT: Record<string, string> = { "symptom:error.length": "10–2000 characters", "contactWindow:error.length": "up to 200 characters", "alternativeSlots:error.count": "up to two more times" };
/** The dictionary keys of the refusals (the key check of the tests, IR493). */
export const HQ_REFUSAL_KEYS = [...Object.values(HQ_REFUSAL_TEXT), ...Object.values(HQ_FIELD_TEXT)];

/** Readable refusals of the HQ job actions, in the display language; a field is named in words (IR493). */
export function hqRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null }, t: T = en): string {
  const word = (k: string | undefined) => (k ? t(k) : undefined);
  const fields = Object.entries(f.fieldErrors).map(([k, v]) => `${fieldName(k)}: ${word(HQ_FIELD_TEXT[`${k}:${v}`]) ?? word(HQ_REFUSAL_TEXT[v]) ?? v}`);
  if (fields.length > 0) return fields.join(" · ");
  if (HQ_REFUSAL_TEXT[f.messageKey]) return f.code === "CONFLICT" ? t("Not saved: {reason}.", { reason: textOf(HQ_REFUSAL_TEXT, f.messageKey, t) ?? f.messageKey }) : `${textOf(HQ_REFUSAL_TEXT, f.messageKey, t) ?? f.messageKey}.`;
  if (f.code === "NOT_FOUND") return t("The job no longer exists in your scope.");
  return actionMessage({ ...f, fieldErrors: {} }, t); // the write limit's wait and other codes (IR380)
}

export interface CostLine { kind: "estimate" | "actual"; description: string; visibility: "internal" | "customer"; amountMinor: number; currency: "MYR" | "USD" }
/** “80.00 MYR” from minor units. */
export { money } from "@ac/web/lib/money";
import { money } from "@ac/web/lib/money";
import { codePoints } from "@ac/web/lib/text";
/** Estimate and actual totals per currency, never converted (“80.00 MYR + 6.20 USD”). */
export function costTotals(lines: CostLine[]): { estimate: string; actual: string } {
  const total = (kind: CostLine["kind"]) => {
    const by = new Map<string, number>();
    for (const l of lines) if (l.kind === kind) by.set(l.currency, (by.get(l.currency) ?? 0) + l.amountMinor);
    return by.size > 0 ? [...by].toSorted(([a], [b]) => a.localeCompare(b)).map(([c, v]) => money(v, c)).join(" + ") : "—";
  };
  return { estimate: total("estimate"), actual: total("actual") };
}
/** A cost line as entered: description 1–200, a non-negative amount with two decimals at most. */
export function costLineOf(f: { kind: string; description: string; visibility: string; amount: string; currency: string }, t: T = en): { line: CostLine | null; error: string | null } {
  const d = f.description.trim();
  if (!d || d.length > 200) return { line: null, error: t("A description of 1–200 characters is required.") };
  if (!/^\d+(\.\d{1,2})?$/.test(f.amount.trim())) return { line: null, error: t("Enter a non-negative amount with at most two decimals.") };
  return { line: { kind: f.kind === "actual" ? "actual" : "estimate", description: d, visibility: f.visibility === "customer" ? "customer" : "internal", amountMinor: Math.round(Number(f.amount) * 100), currency: f.currency === "USD" ? "USD" : "MYR" }, error: null };
}

/** HQ reviews internal jobs normally and contractor jobs only by escalation (reason required, IR31). */
export const reviewModeOf = (j: Pick<ApiHqJob, "contractorOrgId">) => (j.contractorOrgId ? "hq_escalation" : "normal");

/** The New job form (DD-A06 item 8): symptom 10–2000, the 1st preferred time plus up to two more (IR113 item 2: future,
on a later calendar day than today, 1–4 hours, distinct). */
export function jobRequestErrors(f: { unitId: string; symptom: string; slots: Slot[]; contactWindow?: string }, now: number, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!f.unitId) e.unitId = t("Choose the unit.");
  const s = codePoints(f.symptom.trim());
  if (s < 10 || s > 2000) e.symptom = t("Describe the symptom in 10–2000 characters.");
  // the API's rule (ContactWindowOK, IR64): up to 200 characters, no e-mail address and no run of 7 digits once spaces and
  // - ( ) + tabs are removed
  const contact = (f.contactWindow ?? "").trim();
  if (codePoints(contact) > 200) e.contactWindow = t("Up to 200 characters.");
  else if (contact.includes("@") || /\d{7,}/.test(contact.replaceAll(/[ \-()+\t]/g, ""))) e.contactWindow = t("No e-mail addresses or phone numbers in the contact window.");
  const today = new Date(now + 8 * 3_600_000).toISOString().slice(0, 10);
  const keys = new Set<string>();
  for (const [i, x] of f.slots.entries()) {
    const h = (Date.parse(x.endAt) - Date.parse(x.startAt)) / 3_600_000;
    const day = new Date(Date.parse(x.startAt) + 8 * 3_600_000).toISOString().slice(0, 10);
    if (!(h >= 1 && h <= 4)) e[`slot${String(i)}`] = t("Each time is 1–4 hours long.");
    else if (day <= today) e[`slot${String(i)}`] = t("Times start on a later day than today (Kuala Lumpur).");
    else if (keys.has(x.startAt + x.endAt)) e[`slot${String(i)}`] = t("The times must differ.");
    keys.add(x.startAt + x.endAt);
  }
  return e;
}

/** The default new end of an access extension: one day after the current end. */
export const extendDefault = (until: string) => new Date(Date.parse(until) + 24 * 3_600_000).toISOString();

/** The submitted report card (Figma 06-1 “Work report · version 1”): who submitted it when, checks and photos, each
inspection result and reading, and whether HQ may decide — a normal review of an internal job, an escalation (reason
required) of a contractor's job, never a version the reviewer authored (IR31). */
export interface ReportCard {
  id: string; version: number; title: string; sub: string; author: string; rows: { id: string; label: string; result: string; tone: Tone; reason: string | null; photos: string[] }[]; readings: [string, string][];
  workText: string; mode: "normal" | "hq_escalation"; decide: boolean; note: string; tone: "primary" | "warn" | "ok" | "muted"; rework: { label: string; checked: boolean }[];
  reviews: { text: string; tone: Tone }[];
  /** The rest of the report for “Open report →”: parts and refrigerant, the next action and the customer's sign-off. */
  parts: [string, string][]; next: string; signOff: string;
}
/** Why HQ may or may not review the report, in the job's state (English; the caller translates it). */
function reviewNote(status: string, av: ApiWorkReport["reviewAvailability"], mode: ReportCard["mode"]): string {
  if (status === "completed") return "Accepted — the job is completed. Completion does not resolve linked alerts.";
  if (status === "rework_requested") return "Returned for rework — the technician works on the next version.";
  if (status !== "submitted") return "The report is not under review in the job’s current state.";
  if (av.allowed) {
    return mode === "hq_escalation" ? "The contractor reviews its own reports first. HQ may accept or return this one only by escalation, with a reason."
      : "You did not contribute to this report, so you can review it (self-approval is rejected).";
  }
  if (av.reason === "self_authored") return "You submitted this report version, so you cannot accept or return it (self-approval is rejected).";
  return av.reason === "not_current" ? "A newer report version exists — reload to review it." : "Reviewing needs the job.write permission.";
}
/** The card's colour: a decision HQ may take (warn when it is an escalation), or the outcome. */
function cardTone(isDecide: boolean, mode: ReportCard["mode"], status: string): ReportCard["tone"] {
  if (isDecide) return mode === "hq_escalation" ? "warn" : "primary";
  return status === "completed" ? "ok" : "muted";
}
/** The customer's sign-off: absent with the reason, signed by whom and when, or none. */
function signOffText(s: ApiWorkReport["signOff"], i: I18n): string {
  const { t, display } = i;
  if (!s) return t("Not signed");
  return s.absentReason ? t("Customer absent — {reason}", { reason: s.absentReason }) : t("Signed by {name} {time}", { name: s.signerName, time: showTime(s.signedAt, display) });
}
export function reportCard(j: ApiHqJob, r: ApiWorkReport, byUser: Map<string, string>, i: I18n = EN): ReportCard {
  const { t, display } = i;
  const author = byUser.get(r.authorId) ?? t("technician");
  const photos = r.attachmentRefs.filter((a) => a.status === "ready").length;
  const mode = reviewModeOf(j);
  const av = r.reviewAvailability;
  const isDecide = j.status === "submitted" && av.allowed;
  const note = t(reviewNote(j.status, av, mode));
  const rows = inspectionRows(r.items, t, r.attachmentRefs).map((it) => {
    const x = it.result ? resultText[it.result] : { label: "No result", tone: "crit" as const };
    return { id: it.id, label: it.label, result: t(x.label), tone: x.tone, reason: it.reason, photos: it.photos };
  });
  const readings = readingRows(r.measurements, t);
  const submittedText = j.reportRefs.length > 1 ? "Resubmitted {time}" : "Submitted {time}";
  const submitted = r.submittedAt ? t(submittedText, { time: showTime(r.submittedAt, display) }) : t("Not submitted");
  return {
    id: r.id, version: r.version, title: t("Work report · version {version}", { version: r.version }), author, rows, readings, workText: r.workText, mode, decide: isDecide, note,
    sub: t("{submitted} by {author} · {checks} · {photos}", {
      submitted, author, checks: t(r.items.length === 1 ? "1 check" : "{n} checks", { n: r.items.length }), photos: t(photos === 1 ? "1 photo" : "{n} photos", { n: photos }),
    }),
    tone: cardTone(isDecide, mode, j.status),
    // Figma 06-1 “Items that need rework”: the items needing attention (ticked), the readings, and the photos (shared with
    // the contractor's dialog, IR609)
    rework: reworkItems(inspectionRows(r.items, t, r.attachmentRefs), readings, photos, t),
    parts: [...r.parts.map((p): [string, string] => [`${p.name} × ${String(p.quantity)}`, `${p.source.replaceAll('_', " ")}${p.replacesComponentKey ? ` · ${t("replaces {part}", { part: p.replacesComponentKey.replaceAll('_', " ") })}` : ""}`]),
      ...r.refrigerant.map((x): [string, string] => [t("{refrigerant} · cylinder {id}", { refrigerant: x.refrigerant, id: x.cylinderId }), t("recovered {recovered} kg · charged {charged} kg · leak check {check}", { recovered: x.recoveredKg, charged: x.chargedKg, check: x.leakCheck.replaceAll('_', " ") })])],
    next: r.nextAction?.kind === "follow_up" ? t("Follow-up {date} — {note}", { date: r.nextAction.date.slice(0, 10), note: r.nextAction.note }) : t("No follow-up needed"),
    signOff: signOffText(r.signOff, i),
    reviews: [...r.reviewHistory].toSorted((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt)).map((h) => {
      return {
        text: `${t(h.decision === "accept" ? "v{version} accepted {time} by {name}" : "v{version} returned {time} by {name}", { version: h.reportVersion, time: showTime(h.occurredAt, display), name: byUser.get(h.reviewerUserId) ?? t("reviewer") })}${h.reason ? ` — “${h.reason}”` : ""}`,
        tone: h.decision === "accept" ? "ok" : "warn",
      };
    }),
  };
}
export { returnReason } from "@ac/web/lib/partnerReview"; // the HQ dialog's reason, shared with the contractor's (IR609)
