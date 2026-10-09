// HQ maintenance jobs (FR-A06, DD-A06, Figma Admin 06-1, 06-11…06-18) from the Core API: the pipeline counts and
// filters of the Jobs tab, one row per job, and for the selected job the status stepper, the key facts, the delivery
// (internal assignment or contractor offer with the technician's acknowledgement), the client's preferred times with
// the HQ technicians free for each (IR113), the proposal and partner time-change cards, and the rules of hold, cancel
// (IR56), follow-up classification (IR114), the submitted report with the HQ review mode (IR31), the cost lines with
// their totals per currency and the New job form. Pure code shared by the server loader and the client view; Vitest
// covers it.
import { klTime } from "@ac/web/lib/devices";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";
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
    return { kind: "contractor", title: `Delivery · contractor ${org}`, lines: [["Visit", `${slotText(o.visitSlot)} · ${state}`], ["Access", `${klTime(o.accessValidFrom).slice(5)} → ${klTime(o.accessValidUntil).slice(5)}${o.decision === "accept" && Date.parse(o.accessValidUntil) <= now ? " · ended" : ""}`], ["Technician", a ? `${tech} · ${slotText({ startAt: a.scheduledStart, endAt: a.scheduledEnd })}` : "not assigned by the contractor yet"]], ack: ackLine, cantMake: a?.acknowledgement === "cant_make" };
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
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${byField[`${k}:${v}`] ?? keys[v] ?? v}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return `${f.code === "CONFLICT" ? "Not saved (CONFLICT): " : ""}${keys[f.messageKey]}.`;
  if (f.code === "NOT_FOUND") return "The job no longer exists in your scope.";
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
export function costLineOf(f: { kind: string; description: string; visibility: string; amount: string; currency: string }): { line: CostLine | null; error: string | null } {
  const d = f.description.trim();
  if (!d || d.length > 200) return { line: null, error: "A description of 1–200 characters is required." };
  if (!/^\d+(\.\d{1,2})?$/.test(f.amount.trim())) return { line: null, error: "Enter a non-negative amount with at most two decimals." };
  return { line: { kind: f.kind === "actual" ? "actual" : "estimate", description: d, visibility: f.visibility === "customer" ? "customer" : "internal", amountMinor: Math.round(Number(f.amount) * 100), currency: f.currency === "USD" ? "USD" : "MYR" }, error: null };
}

/** HQ reviews internal jobs normally and contractor jobs only by escalation (reason required, IR31). */
export const reviewModeOf = (j: Pick<ApiHqJob, "contractorOrgId">) => (j.contractorOrgId ? "hq_escalation" : "normal") as "normal" | "hq_escalation";

/** The New job form (DD-A06 item 8): symptom 10–2000, the 1st preferred time plus up to two more (IR113 item 2: future,
 * on a later calendar day than today, 1–4 hours, distinct). */
export function newJobErrors(f: { unitId: string; symptom: string; slots: Slot[] }, now: number): Record<string, string> {
  const e: Record<string, string> = {};
  if (!f.unitId) e.unitId = "Choose the unit.";
  const s = f.symptom.trim().length;
  if (s < 10 || s > 2000) e.symptom = "Describe the symptom in 10–2000 characters.";
  const today = new Date(now + 8 * 3600_000).toISOString().slice(0, 10);
  const keys = new Set<string>();
  f.slots.forEach((x, i) => {
    const h = (Date.parse(x.endAt) - Date.parse(x.startAt)) / 3600_000;
    const day = new Date(Date.parse(x.startAt) + 8 * 3600_000).toISOString().slice(0, 10);
    if (!(h >= 1 && h <= 4)) e[`slot${i}`] = "Each time is 1–4 hours long.";
    else if (day <= today) e[`slot${i}`] = "Times start on a later day than today.";
    else if (keys.has(x.startAt + x.endAt)) e[`slot${i}`] = "The times must differ.";
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
export function reportCard(j: ApiHqJob, r: ApiWorkReport, byUser: Map<string, string>): ReportCard {
  const author = byUser.get(r.authorId) ?? "technician";
  const photos = r.attachmentRefs.filter((a) => a.status === "ready").length;
  const mode = reviewModeOf(j);
  const av = r.reviewAvailability;
  const decide = j.status === "submitted" && av.allowed;
  const note = j.status === "completed" ? "Accepted — the job is completed. Completion does not resolve linked alerts."
    : j.status === "rework_requested" ? "Returned for rework — the technician works on the next version."
    : j.status !== "submitted" ? "The report is not under review in the job’s current state."
    : !av.allowed ? (av.reason === "self_authored" ? "You submitted this report version, so you cannot accept or return it (self-approval is rejected)." : av.reason === "not_current" ? "A newer report version exists — reload to review it." : "Reviewing needs the job.write permission.")
    : mode === "hq_escalation" ? "The contractor reviews its own reports first. HQ may accept or return this one only by escalation, with a reason (IR31)."
    : "You did not contribute to this report, so you can review it (self-approval is rejected).";
  const rows = inspectionRows(r.items).map((it) => {
    const t = it.result ? resultText[it.result] : { label: "No result", tone: "crit" as const };
    return { id: it.id, label: it.label, result: t.label, tone: t.tone, reason: it.reason };
  });
  return {
    id: r.id, version: r.version, title: `Work report · version ${r.version}`, author, rows, readings: readingRows(r.measurements), workText: r.workText, mode, decide, note,
    sub: `${r.submittedAt ? `${j.reportRefs.length > 1 ? "Resubmitted" : "Submitted"} ${klTime(r.submittedAt).slice(5)}` : "Not submitted"} by ${author} · ${r.items.length} check${r.items.length === 1 ? "" : "s"} · ${photos} photo${photos === 1 ? "" : "s"}`,
    tone: decide ? (mode === "hq_escalation" ? "warn" : "primary") : j.status === "completed" ? "ok" : "muted",
    // Figma 06-1 “Items that need rework”: the items needing attention (ticked), the readings, and the photos.
    rework: [...rows.filter((x) => x.tone === "warn" || x.tone === "crit").map((x) => ({ label: `${x.label} — ${x.result.toLowerCase()}${x.reason ? ` (${x.reason})` : ""}`, checked: true })),
      ...readingRows(r.measurements).map(([label]) => ({ label: `${label} — re-measure`, checked: false })), { label: "Photos — add more evidence", checked: photos === 0 }],
    parts: [...r.parts.map((p): [string, string] => [`${p.name} × ${p.quantity}`, `${p.source.replace(/_/g, " ")}${p.replacesComponentKey ? ` · replaces ${p.replacesComponentKey.replace(/_/g, " ")}` : ""}`]),
      ...r.refrigerant.map((x): [string, string] => [`${x.refrigerant} · cylinder ${x.cylinderId}`, `recovered ${x.recoveredKg} kg · charged ${x.chargedKg} kg · leak check ${x.leakCheck.replace(/_/g, " ")}`])],
    next: r.nextAction?.kind === "follow_up" ? `Follow-up ${r.nextAction.date.slice(0, 10)} — ${r.nextAction.note}` : "No follow-up needed",
    signOff: r.signOff ? (r.signOff.absentReason ? `Customer absent — ${r.signOff.absentReason}` : `Signed by ${r.signOff.signerName} ${klTime(r.signOff.signedAt).slice(5)}`) : "Not signed",
    reviews: [...r.reviewHistory].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt)).map((h) => ({
      text: `v${h.reportVersion} ${h.decision === "accept" ? "accepted" : "returned"} ${klTime(h.occurredAt).slice(5)} by ${byUser.get(h.reviewerUserId) ?? "reviewer"}${h.reason ? ` — “${h.reason}”` : ""}`, tone: h.decision === "accept" ? "ok" : "warn",
    })),
  };
}
/** The return reason sent to jobs.review: the ticked items, then the reviewer's words (1–1000 together). */
export function returnReason(items: string[], text: string): string {
  const t = text.trim();
  return items.length ? `Rework: ${items.join("; ")}.${t ? ` ${t}` : ""}` : t;
}
