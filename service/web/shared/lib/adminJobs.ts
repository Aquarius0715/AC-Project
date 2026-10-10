// HQ maintenance jobs (FR-A06, DD-A06, Figma Admin 06-1, 06-11…06-18) from the Core API: the pipeline counts and
// filters of the Jobs tab, one row per job, and for the selected job the status stepper, the key facts, the delivery
// (internal assignment or contractor offer with the technician's acknowledgement), the client's preferred times with
// the HQ technicians free for each (IR113), the proposal and partner time-change cards, and the rules of hold, cancel
// (IR56), follow-up classification (IR114), the submitted report with the HQ review mode (IR31), the cost lines with
// their totals per currency and the New job form. Pure code shared by the server loader and the client view; Vitest
// covers it. Texts in the display language (`i` / `t`, IR290); instants in the user's display time zone (IR44), and the
// times HQ types are read in it too (NFR-08). Business days (a follow-up's classify-by day, "from tomorrow") stay
// Kuala Lumpur days.
import { EN, showSpan, showTime, translator, zonedInstant, zonedParts, type I18n, type T } from "@ac/web/lib/i18n";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { Slot } from "@ac/web/lib/partnerOverview";
import { inspectionRows, readingRows, resultText, type ApiWorkReport } from "@ac/web/lib/partnerReview";

export type ApiHqRow = {
  projection: "summary"; id: string; version: number; unitId: string; type: string; status: string; dueAt: string; requestedSlot: Slot; scheduledSlot: Slot | null;
  assignmentId: string | null; origin: string; preferredSlots: Slot[]; severity: string; displayStatus: string; assignmentAcknowledgement: "pending" | "accepted" | "cant_make" | null;
  technicianMembershipId: string | null; accessValidFrom: string | null; accessValidUntil: string | null;
};
export type ApiSlotProposal = { id: string; source: "hq" | "contractor"; slot: Slot; hold: { kind: "internal"; membershipId: string } | { kind: "contractor"; contractorOrgId: string; technicianMembershipId: string | null }; message: string; replyBy: string; status: string; decidedAt: string | null; declineReason: string | null; declineComment: string | null };
export type ApiPartnerProposal = { id: string; offerId: string; slot: Slot; technicianMembershipId: string; reason: string; sentAt: string; status: string };
export type ApiHqJob = {
  projection: "detail"; id: string; version: number; unitId: string; type: string; status: string; symptom: string; contactWindow: string | null; origin: string; planId: string | null; occurrenceAt: string | null;
  requestedSlot: Slot; preferredSlots: Slot[]; preferenceRound: number; slotProposal: ApiSlotProposal | null; partnerSlotProposal: ApiPartnerProposal | null; scheduledSlot: Slot | null;
  dueAt: string; startedAt: string | null; completedAt: string | null; contractorOrgId: string | null; createdAt: string; followUpOfJobId: string | null; followUpClass: "pending" | "rework" | "new_request" | null;
  assignment: { technicianMembershipId: string; scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: string; cantMakeReason: string | null; alternativeSlot: Slot | null } | null;
  offer: { id: string; contractorOrgId: string; visitSlot: Slot; offerExpiresAt: string; accessValidFrom: string; accessValidUntil: string; decision: string | null; decidedAt: string | null; declineReason: string | null } | null;
  reportRefs: { reportId: string; reportVersion: number }[]; draftReportRef: { reportId: string; reportVersion: number } | null; costs: CostLine[] | null;
};
export type Names = { units: Map<string, string>; customers: Map<string, string>; people: Map<string, string>; orgs: Map<string, string> };

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
 * starts on a later Kuala Lumpur day than now, which IR113 item 2 asks of new preferred times. */
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
export const SORTS = [{ id: "status", text: "Status ↑", field: "status", direction: "asc" }, { id: "dueAt", text: "Deadline ↑", field: "dueAt", direction: "asc" }, { id: "severity", text: "Severity ↓", field: "severity", direction: "desc" }] as const;
export type HqSort = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): HqSort => (SORTS.some((s) => s.id === v) ? (v as HqSort) : "status");

export type Query = { customerId?: string; propertyId?: string; unitId?: string; origin?: string; delivery?: string; assignee?: string; overdue?: string; type?: string; stage?: string; from?: string; to?: string };
/** The requested-time period of the list (URL from / to, ISO instants; the HQ overview's job counts, IR245): both valid
 * and from < to, else none. */
export function periodOfQuery(q: Pick<Query, "from" | "to">): { from: string; to: string } | null {
  const f = q.from ? Date.parse(q.from) : NaN, t = q.to ? Date.parse(q.to) : NaN;
  return Number.isFinite(f) && Number.isFinite(t) && f < t ? { from: new Date(f).toISOString(), to: new Date(t).toISOString() } : null;
}
/** “Requested time 8 Sept, 12:00 am – 14 Sept, 12:09 pm MYT” for the period chip. */
export const periodChip = (p: { from: string; to: string }, i: I18n = EN) => i.t("Requested time {span}", { span: showSpan(p.from, p.to, i.display) });
/** Every status but cancelled: the list without a stage tile, and the jobs counted "in scope" (no tile names cancelled). */
export const LISTED = ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "rework_requested", "completed", "on_hold"];
const TYPE_IDS = ["reactive", "periodic", "preventive"];
/** The jobs.list filters of the scope and the filter bar; delivery is the HQ organization (internal) or a contractor.
 * The type and the default "not cancelled" are filters of jobs.list too (IR290), so its total counts what the list shows. */
export function filtersOf(q: Query, hqOrgId: string | null, withStage = true): Record<string, unknown> {
  const f: Record<string, unknown> = {};
  if (q.customerId) f.customerId = q.customerId;
  if (q.propertyId) f.propertyId = q.propertyId;
  if (q.unitId) f.unitId = q.unitId;
  if (q.origin === "client_request" || q.origin === "periodic_plan") f.origin = q.origin;
  if (q.type && TYPE_IDS.includes(q.type)) f.type = q.type;
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

export type HqRow = { id: string; short: string; title: string; customer: string; type: string; origin: string; status: string; badge: { status: string; overdue: boolean }; line: string; lineTone?: Tone };
const ack = (a: string | null, t: T) => t(a === "accepted" ? "accepted ✓" : a === "cant_make" ? "can’t make it ⚠" : "awaiting acceptance");
/** One job of the list: unit, customer, type, origin, status (Time proposed derived, IR89 overdue) and what is next. */
export function hqRow(j: ApiHqRow, now: number, n: Names, unitOrg: Map<string, string>, i: I18n = EN): HqRow {
  const { t } = i;
  const tech = j.technicianMembershipId ? n.people.get(j.technicianMembershipId) ?? t("technician") : null;
  const overdue = (j.status === "assigned" || j.status === "in_progress") && !!j.scheduledSlot && Date.parse(j.scheduledSlot.endAt) <= now;
  const ended = overdue ? ` · ${t("window ended")}` : "";
  let line = "", lineTone: Tone | undefined;
  switch (j.displayStatus === "time_proposed" ? "time_proposed" : j.status) {
    case "requested": line = j.preferredSlots.length ? t(j.preferredSlots.length === 1 ? "1 preferred time · book one or propose" : "{n} preferred times · book one or propose", { n: j.preferredSlots.length })
      : j.origin === "periodic_plan" ? t("plan occurrence {slot} · book it", { slot: slotText(j.requestedSlot, i) }) : t("due {time} · unassigned", { time: showTime(j.dueAt, i.display) }); break;
    case "time_proposed": line = t("Time proposed · waiting for the client"); break;
    case "offered": line = j.scheduledSlot ? t("Offer open · {slot}", { slot: slotText(j.scheduledSlot, i) }) : t("Offer open"); break;
    case "accepted": line = t("Contractor accepted · assigning their technician"); break;
    case "assigned": [line, lineTone] = [`${tech ?? t("technician")} · ${ack(j.assignmentAcknowledgement, t)}${ended}`, j.assignmentAcknowledgement === "cant_make" ? "warn" : undefined]; break;
    case "in_progress": line = `${t("{name} on site", { name: tech ?? t("technician") })}${ended}`; break;
    case "submitted": line = t(j.accessValidUntil ? "report submitted · contractor review first" : "report submitted · awaiting HQ review"); break;
    case "rework_requested": line = t("returned for rework"); break;
    case "on_hold": [line, lineTone] = [t("on hold — scheduling and reports blocked"), "warn"]; break;
    case "completed": line = statusWord("completed", t); break;
    case "cancelled": line = statusWord("cancelled", t); break;
  }
  return {
    id: j.id, short: j.id.slice(0, 8), title: n.units.get(j.unitId) ?? t("Unit"), customer: n.customers.get(unitOrg.get(j.unitId) ?? "") ?? t("customer"),
    type: typeLabel(j.type, t), origin: j.origin, status: j.displayStatus === "time_proposed" ? "time_proposed" : j.status, badge: { status: j.displayStatus === "time_proposed" ? "time_proposed" : j.status, overdue }, line, lineTone,
  };
}

export type Step = { label: string; state: "done" | "current" | "todo" };
/** The status stepper: internal delivery skips Offered / Accepted (DD-A06 item 6). */
export function stepper(status: string, internal: boolean, t: T = en): Step[] {
  const all = internal ? ["requested", "assigned", "in_progress", "submitted", "completed"] : ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed"];
  const labels: Record<string, string> = { requested: "Requested", offered: "Offered", accepted: "Accepted", assigned: "Assigned", in_progress: "In progress", submitted: "Submitted", completed: "Completed" };
  const at = status === "rework_requested" || status === "on_hold" ? all.indexOf("in_progress") : status === "cancelled" ? -1 : all.indexOf(status);
  return all.map((s, k) => ({ label: t(labels[s]), state: status === "completed" || k < at ? "done" : k === at ? "current" : "todo" }));
}

/** The four facts of the detail: requested window, due, scheduled slot, symptom with the contact window. */
export function facts(j: ApiHqJob, n: Names, i: I18n = EN): { label: string; value: string; sub: string }[] {
  const { t } = i;
  return [
    { label: t("Requested window"), value: slotText(j.requestedSlot, i), sub: t(j.origin === "periodic_plan" ? "the plan occurrence" : "the client’s 1st preferred time") },
    { label: t("Due"), value: showTime(j.dueAt, i.display), sub: t(j.dueAt === j.requestedSlot.endAt ? "= requested end" : j.type === "reactive" ? "reactive SLA" : "plan occurrence") },
    { label: t("Scheduled"), value: j.scheduledSlot ? slotText(j.scheduledSlot, i) : t("not booked"), sub: j.assignment ? n.people.get(j.assignment.technicianMembershipId) ?? t("technician") : j.offer ? n.orgs.get(j.offer.contractorOrgId) ?? t("contractor") : "—" },
    { label: t("Symptom"), value: j.symptom || "—", sub: j.contactWindow ? t("contact {window}", { window: j.contactWindow }) : t("no contact window") },
  ];
}

export type Delivery = { kind: "internal" | "contractor"; title: string; lines: [string, string][]; ack: { tone: Tone; text: string } | null; cantMake: boolean };
/** Who does the work: the internal assignment, or the contractor's offer with its access window and the technician. */
export function delivery(j: ApiHqJob, n: Names, now: number, i: I18n = EN): Delivery | null {
  const { t, display } = i;
  const a = j.assignment, o = j.offer;
  const tech = a ? n.people.get(a.technicianMembershipId) ?? t("technician") : "";
  const window = a ? slotText({ startAt: a.scheduledStart, endAt: a.scheduledEnd }, i) : "";
  const done = a?.status === "completed" ? ` · ${t("ended at completion")}` : "";
  const ackLine = a?.status === "completed" ? { tone: "ok" as Tone, text: t("{name} completed the job — the assignment ended with it, so {name}’s time is free again (IR234).", { name: tech }) }
    : a ? (a.acknowledgement === "accepted" ? { tone: "ok" as Tone, text: t("{name} accepted the assignment ✓", { name: tech }) }
      : a.acknowledgement === "cant_make" ? { tone: "warn" as Tone, text: `${a.cantMakeReason ? t("{name} can’t make this time: “{reason}”", { name: tech, reason: a.cantMakeReason }) : t("{name} can’t make this time", { name: tech })}${a.alternativeSlot ? ` · ${t("could do {slot}", { slot: slotText(a.alternativeSlot, i) })}` : ""}. ${t("Any new time needs the client’s approval.")}` }
      : { tone: "primary" as Tone, text: t("Waiting for {name} to accept (受領).", { name: tech }) }) : null;
  if (o && j.contractorOrgId) {
    const org = n.orgs.get(o.contractorOrgId) ?? t("contractor");
    const state = o.decision === "accept" ? (o.decidedAt ? t("accepted {time}", { time: showTime(o.decidedAt, display) }) : t("accepted")) : Date.parse(o.offerExpiresAt) <= now ? t("expired") : t("offer open · expires {time}", { time: showTime(o.offerExpiresAt, display) });
    return {
      kind: "contractor", title: t("Delivery · contractor {name}", { name: org }),
      lines: [[t("Visit"), `${slotText(o.visitSlot, i)} · ${state}`], [t("Access"), `${showSpan(o.accessValidFrom, o.accessValidUntil, display)}${o.decision === "accept" && Date.parse(o.accessValidUntil) <= now ? ` · ${t("ended")}` : ""}`],
        [t("Technician"), a ? `${tech} · ${window}${done}` : t("not assigned by the contractor yet")]],
      ack: ackLine, cantMake: a?.status !== "completed" && a?.acknowledgement === "cant_make",
    };
  }
  if (a) return { kind: "internal", title: t("Delivery · internal"), lines: [[t("Technician"), `${tech} · ${t("assignment {slot}", { slot: window })}${done}`]], ack: ackLine, cantMake: a.status !== "completed" && a.acknowledgement === "cant_make" };
  return null;
}

/** The times HQ may book (IR113): the client's preferred times, or — for a plan's job without them — the plan
 * occurrence (the requested window the plan generated, D16). */
export const agreedSlots = (j: Pick<ApiHqJob, "preferredSlots" | "planId" | "occurrenceAt" | "requestedSlot">): Slot[] => (j.preferredSlots.length ? j.preferredSlots : j.planId && j.occurrenceAt ? [j.requestedSlot] : []);

export type PreferredRow = { rank: number; slot: Slot; text: string; hq: string; fits: boolean; people: { id: string; displayName: string }[]; plan: boolean };
/** The agreed times with the HQ technicians free and qualified for each (members.eligible). */
export function preferredRows(j: ApiHqJob, eligible: { id: string; displayName: string }[][], now: number, i: I18n = EN): PreferredRow[] {
  const { t } = i;
  const plan = !j.preferredSlots.length;
  return agreedSlots(j).map((s, k) => {
    const people = eligible[k] ?? [];
    const past = Date.parse(s.startAt) <= now;
    return { rank: k + 1, slot: s, text: longSlot(s, i), hq: past ? t("in the past") : people.length ? people.map((p) => p.displayName).join(", ") : t("no qualified HQ technician free"), fits: !past && people.length > 0, people: past ? [] : people, plan };
  });
}

/** The default windows of an HQ offer (IR141 item 2): answer within 24 h (not after the visit), access from now to a day after. */
export function offerDefaults(slot: Slot, now: number): { offerExpiresAt: string; accessValidFrom: string; accessValidUntil: string } {
  const expires = Math.min(now + 24 * 3600_000, Date.parse(slot.startAt));
  return { offerExpiresAt: new Date(expires).toISOString(), accessValidFrom: new Date(now).toISOString(), accessValidUntil: new Date(Date.parse(slot.endAt) + 24 * 3600_000).toISOString() };
}

/** Which of hold / resume / cancel HQ may use (IR56; hold only from in progress or submitted). */
export function controls(status: string, t: T = en): { hold: boolean; resume: boolean; cancel: boolean; cancelWhy: string | null } {
  const cancel = ["requested", "offered", "accepted", "assigned", "on_hold", "rework_requested"].includes(status);
  const cancelWhy = cancel ? null : t(status === "in_progress" || status === "submitted" ? "Put the job on hold first (IR56)." : "Completed and cancelled jobs cannot be cancelled.");
  return { hold: status === "in_progress" || status === "submitted", resume: status === "on_hold", cancel, cancelWhy };
}

/** The classify-by time of a follow-up: one business day (Mon–Fri in Kuala Lumpur) after it was created, as an instant
 * in the display time zone (display only, IR114). */
export function classifyBy(createdAt: string, i: I18n = EN): string {
  const d = new Date(Date.parse(createdAt) + 8 * 3600_000);
  let add = 1;
  while (add > 0) { d.setUTCDate(d.getUTCDate() + 1); if (d.getUTCDay() !== 0 && d.getUTCDay() !== 6) add--; }
  return showTime(new Date(d.getTime() - 8 * 3600_000).toISOString(), i.display);
}

/** Readable refusals of the HQ job actions. */
export function hqRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): string {
  const keys: Record<string, string> = {
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
    "errors.contact_details_forbidden": "no e-mail addresses or phone numbers in the contact window (IR64)",
    "error.count": "too many entries", "error.invalid": "not a valid value",
    "error.length": "1–1000 characters", "error.required": "required", "error.range": "the end must be after the start", "error.past": "must be in the future",
  };
  const byField: Record<string, string> = { "symptom:error.length": "10–2000 characters", "contactWindow:error.length": "up to 200 characters", "alternativeSlots:error.count": "up to two more times" };
  const word = (k: string | undefined) => (k ? t(k) : undefined);
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${word(byField[`${k}:${v}`]) ?? word(keys[v]) ?? v}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return f.code === "CONFLICT" ? t("Not saved (CONFLICT): {reason}.", { reason: t(keys[f.messageKey]) }) : `${t(keys[f.messageKey])}.`;
  if (f.code === "NOT_FOUND") return t("The job no longer exists in your scope.");
  return `${f.code} — ${f.messageKey}`;
}

export type CostLine = { kind: "estimate" | "actual"; description: string; visibility: "internal" | "customer"; amountMinor: number; currency: "MYR" | "USD" };
/** “80.00 MYR” from minor units. */
export const money = (amountMinor: number, currency: string) => `${(amountMinor / 100).toFixed(2)} ${currency}`;
/** Estimate and actual totals per currency, never converted (“80.00 MYR + 6.20 USD”). */
export function costTotals(lines: CostLine[]): { estimate: string; actual: string } {
  const total = (kind: CostLine["kind"]) => {
    const by = new Map<string, number>();
    for (const l of lines) if (l.kind === kind) by.set(l.currency, (by.get(l.currency) ?? 0) + l.amountMinor);
    return by.size ? [...by.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([c, v]) => money(v, c)).join(" + ") : "—";
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
export const reviewModeOf = (j: Pick<ApiHqJob, "contractorOrgId">) => (j.contractorOrgId ? "hq_escalation" : "normal") as "normal" | "hq_escalation";

/** The New job form (DD-A06 item 8): symptom 10–2000, the 1st preferred time plus up to two more (IR113 item 2: future,
 * on a later calendar day than today, 1–4 hours, distinct). */
export function newJobErrors(f: { unitId: string; symptom: string; slots: Slot[] }, now: number, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!f.unitId) e.unitId = t("Choose the unit.");
  const s = f.symptom.trim().length;
  if (s < 10 || s > 2000) e.symptom = t("Describe the symptom in 10–2000 characters.");
  const today = new Date(now + 8 * 3600_000).toISOString().slice(0, 10);
  const keys = new Set<string>();
  f.slots.forEach((x, i) => {
    const h = (Date.parse(x.endAt) - Date.parse(x.startAt)) / 3600_000;
    const day = new Date(Date.parse(x.startAt) + 8 * 3600_000).toISOString().slice(0, 10);
    if (!(h >= 1 && h <= 4)) e[`slot${i}`] = t("Each time is 1–4 hours long.");
    else if (day <= today) e[`slot${i}`] = t("Times start on a later day than today (Kuala Lumpur).");
    else if (keys.has(x.startAt + x.endAt)) e[`slot${i}`] = t("The times must differ.");
    keys.add(x.startAt + x.endAt);
  });
  return e;
}

/** The default new end of an access extension: one day after the current end. */
export const extendDefault = (until: string) => new Date(Date.parse(until) + 24 * 3600_000).toISOString();

/** The submitted report card (Figma 06-1 “Work report · version 1”): who submitted it when, checks and photos, each
 * inspection result and reading, and whether HQ may decide — a normal review of an internal job, an escalation (reason
 * required) of a contractor's job, never a version the reviewer authored (IR31). */
export type ReportCard = {
  id: string; version: number; title: string; sub: string; author: string; rows: { id: string; label: string; result: string; tone: Tone; reason: string | null }[]; readings: [string, string][];
  workText: string; mode: "normal" | "hq_escalation"; decide: boolean; note: string; tone: "primary" | "warn" | "ok" | "muted"; rework: { label: string; checked: boolean }[];
  reviews: { text: string; tone: Tone }[];
  /** The rest of the report for “Open report →”: parts and refrigerant, the next action and the customer's sign-off. */
  parts: [string, string][]; next: string; signOff: string;
};
export function reportCard(j: ApiHqJob, r: ApiWorkReport, byUser: Map<string, string>, i: I18n = EN): ReportCard {
  const { t, display } = i;
  const author = byUser.get(r.authorId) ?? t("technician");
  const photos = r.attachmentRefs.filter((a) => a.status === "ready").length;
  const mode = reviewModeOf(j);
  const av = r.reviewAvailability;
  const decide = j.status === "submitted" && av.allowed;
  const note = t(j.status === "completed" ? "Accepted — the job is completed. Completion does not resolve linked alerts."
    : j.status === "rework_requested" ? "Returned for rework — the technician works on the next version."
    : j.status !== "submitted" ? "The report is not under review in the job’s current state."
    : !av.allowed ? (av.reason === "self_authored" ? "You submitted this report version, so you cannot accept or return it (self-approval is rejected)." : av.reason === "not_current" ? "A newer report version exists — reload to review it." : "Reviewing needs the job.write permission.")
    : mode === "hq_escalation" ? "The contractor reviews its own reports first. HQ may accept or return this one only by escalation, with a reason (IR31)."
    : "You did not contribute to this report, so you can review it (self-approval is rejected).");
  const rows = inspectionRows(r.items, t).map((it) => {
    const x = it.result ? resultText[it.result] : { label: "No result", tone: "crit" as const };
    return { id: it.id, label: it.label, result: t(x.label), tone: x.tone, reason: it.reason };
  });
  const readings = readingRows(r.measurements, t);
  const submitted = r.submittedAt ? t(j.reportRefs.length > 1 ? "Resubmitted {time}" : "Submitted {time}", { time: showTime(r.submittedAt, display) }) : t("Not submitted");
  return {
    id: r.id, version: r.version, title: t("Work report · version {version}", { version: r.version }), author, rows, readings, workText: r.workText, mode, decide, note,
    sub: t("{submitted} by {author} · {checks} · {photos}", {
      submitted, author, checks: t(r.items.length === 1 ? "1 check" : "{n} checks", { n: r.items.length }), photos: t(photos === 1 ? "1 photo" : "{n} photos", { n: photos }),
    }),
    tone: decide ? (mode === "hq_escalation" ? "warn" : "primary") : j.status === "completed" ? "ok" : "muted",
    // Figma 06-1 “Items that need rework”: the items needing attention (ticked), the readings, and the photos.
    rework: [...rows.filter((x) => x.tone === "warn" || x.tone === "crit").map((x) => ({ label: `${x.label} — ${x.result.toLowerCase()}${x.reason ? ` (${x.reason})` : ""}`, checked: true })),
      ...readings.map(([label]) => ({ label: t("{reading} — re-measure", { reading: label }), checked: false })), { label: t("Photos — add more evidence"), checked: photos === 0 }],
    parts: [...r.parts.map((p): [string, string] => [`${p.name} × ${p.quantity}`, `${p.source.replace(/_/g, " ")}${p.replacesComponentKey ? ` · ${t("replaces {part}", { part: p.replacesComponentKey.replace(/_/g, " ") })}` : ""}`]),
      ...r.refrigerant.map((x): [string, string] => [t("{refrigerant} · cylinder {id}", { refrigerant: x.refrigerant, id: x.cylinderId }), t("recovered {recovered} kg · charged {charged} kg · leak check {check}", { recovered: x.recoveredKg, charged: x.chargedKg, check: x.leakCheck.replace(/_/g, " ") })])],
    next: r.nextAction?.kind === "follow_up" ? t("Follow-up {date} — {note}", { date: r.nextAction.date.slice(0, 10), note: r.nextAction.note }) : t("No follow-up needed"),
    signOff: r.signOff ? (r.signOff.absentReason ? t("Customer absent — {reason}", { reason: r.signOff.absentReason }) : t("Signed by {name} {time}", { name: r.signOff.signerName, time: showTime(r.signOff.signedAt, display) })) : t("Not signed"),
    reviews: [...r.reviewHistory].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt)).map((h) => ({
      text: `${t(h.decision === "accept" ? "v{version} accepted {time} by {name}" : "v{version} returned {time} by {name}", { version: h.reportVersion, time: showTime(h.occurredAt, display), name: byUser.get(h.reviewerUserId) ?? t("reviewer") })}${h.reason ? ` — “${h.reason}”` : ""}`,
      tone: h.decision === "accept" ? "ok" : "warn",
    })),
  };
}
/** The return reason sent to jobs.review: the ticked items, then the reviewer's words (1–1000 together). */
export function returnReason(items: string[], text: string, t: T = en): string {
  const words = text.trim();
  return items.length ? `${t("Rework: {items}.", { items: items.join("; ") })}${words ? ` ${words}` : ""}` : words;
}
