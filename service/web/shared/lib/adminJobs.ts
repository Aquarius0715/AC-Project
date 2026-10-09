// HQ maintenance jobs (FR-A06, DD-A06, Figma Admin 06-1, 06-11…06-18) from the Core API: the pipeline counts and
// filters of the Jobs tab, one row per job, and for the selected job the status stepper, the key facts, the delivery
// (internal assignment or contractor offer with the technician's acknowledgement), the client's preferred times with
// the HQ technicians free for each (IR113), the proposal and partner time-change cards, and the rules of hold, cancel
// (IR56) and follow-up classification (IR114). Pure code shared by the server loader and the client view; Vitest
// covers it.
import { klTime } from "@ac/web/lib/devices";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { Slot } from "@ac/web/lib/partnerOverview";

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
};
export type Names = { units: Map<string, string>; customers: Map<string, string>; people: Map<string, string>; orgs: Map<string, string> };

type Tone = "ok" | "warn" | "crit" | "primary" | "muted";
const md = (iso: string) => klTime(iso).slice(5, 10);
const hm = (iso: string) => klTime(iso).slice(11);
/** “09-15 10:00–12:00”, or both ends across days. */
export const slotText = (s: Slot) => (klTime(s.startAt).slice(0, 10) === klTime(s.endAt).slice(0, 10) ? `${md(s.startAt)} ${hm(s.startAt)}–${hm(s.endAt)}` : `${klTime(s.startAt).slice(5)} → ${klTime(s.endAt).slice(5)}`);
/** “Mon 09-28 · 14:00–16:00” for the preferred-times table. */
export const longSlot = (s: Slot) => `${new Date(Date.parse(s.startAt) + 8 * 3600_000).toLocaleDateString("en-GB", { weekday: "short", timeZone: "UTC" })} ${md(s.startAt)} · ${hm(s.startAt)}–${hm(s.endAt)}`;

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

export type Query = { customerId?: string; propertyId?: string; unitId?: string; origin?: string; delivery?: string; assignee?: string; overdue?: string; type?: string; stage?: string };
/** The jobs.list filters of the scope and the filter bar; delivery is the HQ organization (internal) or a contractor. */
export function filtersOf(q: Query, hqOrgId: string | null, withStage = true): Record<string, unknown> {
  const f: Record<string, unknown> = {};
  if (q.customerId) f.customerId = q.customerId;
  if (q.propertyId) f.propertyId = q.propertyId;
  if (q.unitId) f.unitId = q.unitId;
  if (q.origin === "client_request" || q.origin === "periodic_plan") f.origin = q.origin;
  if (q.delivery === "internal" && hqOrgId) f.organizationId = hqOrgId;
  else if (q.delivery && q.delivery !== "internal") f.organizationId = q.delivery;
  if (q.assignee) f.membershipId = q.assignee;
  if (q.overdue === "1") f.overdueOnly = true;
  const stage = withStage ? STAGES.find((s) => s.id === q.stage) : undefined;
  if (stage) Object.assign(f, stage.filter);
  return f;
}

export type HqRow = { id: string; short: string; title: string; customer: string; type: string; origin: string; status: string; badge: { status: string; overdue: boolean }; line: string; lineTone?: Tone };
const ack = (a: string | null) => (a === "accepted" ? "accepted ✓" : a === "cant_make" ? "can’t make it ⚠" : "awaiting acceptance");
/** One job of the list: unit, customer, type, origin, status (Time proposed derived, IR89 overdue) and what is next. */
export function hqRow(j: ApiHqRow, now: number, n: Names, unitOrg: Map<string, string>): HqRow {
  const tech = j.technicianMembershipId ? n.people.get(j.technicianMembershipId) ?? "technician" : null;
  const overdue = (j.status === "assigned" || j.status === "in_progress") && !!j.scheduledSlot && Date.parse(j.scheduledSlot.endAt) <= now;
  let line = "", lineTone: Tone | undefined;
  switch (j.displayStatus === "time_proposed" ? "time_proposed" : j.status) {
    case "requested": [line, lineTone] = [j.preferredSlots.length ? `${j.preferredSlots.length} preferred time${j.preferredSlots.length === 1 ? "" : "s"} · book one or propose` : `due ${md(j.dueAt)} ${hm(j.dueAt)} · unassigned`, undefined]; break;
    case "time_proposed": line = "Time proposed · waiting for the client"; break;
    case "offered": line = `Offer open${j.scheduledSlot ? ` · ${slotText(j.scheduledSlot)}` : ""}`; break;
    case "accepted": line = "Contractor accepted · assigning their technician"; break;
    case "assigned": [line, lineTone] = [`${tech ?? "technician"} · ${ack(j.assignmentAcknowledgement)}${overdue ? " · window ended" : ""}`, j.assignmentAcknowledgement === "cant_make" ? "warn" : undefined]; break;
    case "in_progress": line = `${tech ?? "technician"} on site${overdue ? " · window ended" : ""}`; break;
    case "submitted": line = `report submitted · ${j.accessValidUntil ? "contractor review first" : "awaiting HQ review"}`; break;
    case "rework_requested": line = "returned for rework"; break;
    case "on_hold": [line, lineTone] = ["on hold — scheduling and reports blocked", "warn"]; break;
    case "completed": line = "completed"; break;
    case "cancelled": line = "cancelled"; break;
  }
  return {
    id: j.id, short: j.id.slice(0, 8), title: n.units.get(j.unitId) ?? "Unit", customer: n.customers.get(unitOrg.get(j.unitId) ?? "") ?? "customer",
    type: typeLabel(j.type), origin: j.origin, status: j.displayStatus === "time_proposed" ? "time_proposed" : j.status, badge: { status: j.displayStatus === "time_proposed" ? "time_proposed" : j.status, overdue }, line, lineTone,
  };
}

export type Step = { label: string; state: "done" | "current" | "todo" };
/** The status stepper: internal delivery skips Offered / Accepted (DD-A06 item 6). */
export function stepper(status: string, internal: boolean): Step[] {
  const all = internal ? ["requested", "assigned", "in_progress", "submitted", "completed"] : ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed"];
  const labels: Record<string, string> = { requested: "Requested", offered: "Offered", accepted: "Accepted", assigned: "Assigned", in_progress: "In progress", submitted: "Submitted", completed: "Completed" };
  const at = status === "rework_requested" || status === "on_hold" ? all.indexOf("in_progress") : status === "cancelled" ? -1 : all.indexOf(status);
  return all.map((s, i) => ({ label: labels[s], state: status === "completed" || i < at ? "done" : i === at ? "current" : "todo" }));
}

/** The four facts of the detail: requested window, due, scheduled slot, symptom with the contact window. */
export function facts(j: ApiHqJob, n: Names): { label: string; value: string; sub: string }[] {
  return [
    { label: "Requested window", value: slotText(j.requestedSlot), sub: "Asia/Kuala_Lumpur" },
    { label: "Due", value: `${md(j.dueAt)} ${hm(j.dueAt)}`, sub: j.dueAt === j.requestedSlot.endAt ? "= requested end" : j.type === "reactive" ? "reactive SLA" : "plan occurrence" },
    { label: "Scheduled", value: j.scheduledSlot ? slotText(j.scheduledSlot) : "not booked", sub: j.assignment ? n.people.get(j.assignment.technicianMembershipId) ?? "technician" : j.offer ? n.orgs.get(j.offer.contractorOrgId) ?? "contractor" : "—" },
    { label: "Symptom", value: j.symptom || "—", sub: j.contactWindow ? `contact ${j.contactWindow}` : "no contact window" },
  ];
}

export type Delivery = { kind: "internal" | "contractor"; title: string; lines: [string, string][]; ack: { tone: Tone; text: string } | null; cantMake: boolean };
/** Who does the work: the internal assignment, or the contractor's offer with its access window and the technician. */
export function delivery(j: ApiHqJob, n: Names, now: number): Delivery | null {
  const a = j.assignment, o = j.offer;
  const tech = a ? n.people.get(a.technicianMembershipId) ?? "technician" : null;
  const ackLine = a ? (a.acknowledgement === "accepted" ? { tone: "ok" as Tone, text: `${tech} accepted the assignment ✓` } : a.acknowledgement === "cant_make" ? { tone: "warn" as Tone, text: `${tech} can’t make this time${a.cantMakeReason ? `: “${a.cantMakeReason}”` : ""}${a.alternativeSlot ? ` · could do ${slotText(a.alternativeSlot)}` : ""}. Any new time needs the client’s approval.` } : { tone: "primary" as Tone, text: `Waiting for ${tech} to accept (受領).` }) : null;
  if (o && j.contractorOrgId) {
    const org = n.orgs.get(o.contractorOrgId) ?? "contractor";
    const state = o.decision === "accept" ? `accepted ${o.decidedAt ? md(o.decidedAt) : ""}`.trim() : Date.parse(o.offerExpiresAt) <= now ? "expired" : `offer open · expires ${md(o.offerExpiresAt)} ${hm(o.offerExpiresAt)}`;
    return { kind: "contractor", title: `Delivery · contractor ${org}`, lines: [["Visit", `${slotText(o.visitSlot)} · ${state}`], ["Access", `${klTime(o.accessValidFrom).slice(5)} → ${klTime(o.accessValidUntil).slice(5)}`], ["Technician", a ? `${tech} · ${slotText({ startAt: a.scheduledStart, endAt: a.scheduledEnd })}` : "not assigned by the contractor yet"]], ack: ackLine, cantMake: a?.acknowledgement === "cant_make" };
  }
  if (a) return { kind: "internal", title: "Delivery · internal", lines: [["Technician", `${tech} · assignment ${slotText({ startAt: a.scheduledStart, endAt: a.scheduledEnd })}`]], ack: ackLine, cantMake: a.acknowledgement === "cant_make" };
  return null;
}

export type PreferredRow = { rank: number; slot: Slot; text: string; hq: string; fits: boolean; people: { id: string; displayName: string }[] };
/** The client's preferred times with the HQ technicians free and qualified for each (members.eligible). */
export function preferredRows(j: ApiHqJob, eligible: { id: string; displayName: string }[][], now: number): PreferredRow[] {
  return j.preferredSlots.map((s, i) => {
    const people = eligible[i] ?? [];
    const past = Date.parse(s.startAt) <= now;
    return { rank: i + 1, slot: s, text: longSlot(s), hq: past ? "in the past" : people.length ? people.map((p) => p.displayName).join(", ") : "no qualified HQ technician free", fits: !past && people.length > 0, people: past ? [] : people };
  });
}

/** The default windows of an HQ offer (IR141 item 2): answer within 24 h (not after the visit), access from now to a day after. */
export function offerDefaults(slot: Slot, now: number): { offerExpiresAt: string; accessValidFrom: string; accessValidUntil: string } {
  const expires = Math.min(now + 24 * 3600_000, Date.parse(slot.startAt));
  return { offerExpiresAt: new Date(expires).toISOString(), accessValidFrom: new Date(now).toISOString(), accessValidUntil: new Date(Date.parse(slot.endAt) + 24 * 3600_000).toISOString() };
}

/** Which of hold / resume / cancel HQ may use (IR56; hold only from in progress or submitted). */
export function controls(status: string): { hold: boolean; resume: boolean; cancel: boolean; cancelWhy: string | null } {
  const cancel = ["requested", "offered", "accepted", "assigned", "on_hold", "rework_requested"].includes(status);
  const cancelWhy = cancel ? null : status === "in_progress" || status === "submitted" ? "Put the job on hold first (IR56)." : "Completed and cancelled jobs cannot be cancelled.";
  return { hold: status === "in_progress" || status === "submitted", resume: status === "on_hold", cancel, cancelWhy };
}

/** The classify-by time of a follow-up: one business day after it was created (display only, IR114). */
export function classifyBy(createdAt: string): string {
  const d = new Date(Date.parse(createdAt) + 8 * 3600_000);
  let add = 1;
  while (add > 0) { d.setUTCDate(d.getUTCDate() + 1); if (d.getUTCDay() !== 0 && d.getUTCDay() !== 6) add--; }
  return d.toISOString().slice(5, 16).replace("T", " ");
}

/** Readable refusals of the HQ job actions. */
export function hqRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
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
    "error.length": "1–1000 characters", "error.required": "required", "error.range": "the end must be after the start", "error.past": "must be in the future",
  };
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${keys[v] ?? v}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return `${f.code === "CONFLICT" ? "Not saved (CONFLICT): " : ""}${keys[f.messageKey]}.`;
  if (f.code === "NOT_FOUND") return "The job no longer exists in your scope.";
  return `${f.code} — ${f.messageKey}`;
}
