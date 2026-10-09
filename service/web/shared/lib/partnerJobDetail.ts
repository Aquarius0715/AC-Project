// The contractor's offer and job detail (FR-P02, FR-P08, DD-P02, Figma Contractor 02-7…02-14) from the Core API:
// what a projection may show (an offer only the job type, the registered site address, the required qualifications
// and the fixed visit; the detail inside the delegation period; the history after it), which of the company's
// technicians fit the visit, the status timeline from the job events and the refusals of the decisions.
// Pure code shared by the server loader and the client view; Vitest covers it.
import { klTime } from "@ac/web/lib/devices";
import { jobEventTitle, until, type ApiCapacity, type ApiJobEvent, type ApiMember, type Slot } from "@ac/web/lib/partnerOverview";
import { slotText } from "@ac/web/lib/partnerJobs";

export type ApiProposal = { id: string; offerId: string; slot: Slot; technicianMembershipId: string; reason: string; sentAt: string; status: "pending" | "sent_to_client" | "kept" | "approved" | "declined" | "withdrawn" };
export type ApiOffer = {
  projection: "offer"; status: "offered" | "accepted"; jobId: string; jobVersion: number; offerId: string; type: string; siteAddress: string | null;
  requiredQualifications: string[]; requestedSlot: Slot; dueAt: string; offerExpiresAt: string; termsVersion: string; origin: string; visitSlot: Slot;
  offeredAt: string; accessValidFrom: string; accessValidUntil: string; partnerSlotProposal: ApiProposal | null;
};
export type ApiDetail = {
  projection: "detail"; id: string; version: number; unitId: string; type: string; status: string; symptom: string; origin: string; alertIds: string[];
  requestedSlot: Slot; scheduledSlot: Slot | null; dueAt: string; startedAt: string | null; completedAt: string | null; assignmentId: string | null;
  partnerSlotProposal: ApiProposal | null; reportRefs: { reportId: string; reportVersion: number }[];
  assignment: { technicianMembershipId: string; scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: string; cantMakeReason: string | null } | null;
  offer: { id: string; termsVersion: string; visitSlot: Slot; offeredAt: string; accessValidFrom: string; accessValidUntil: string; decision: string | null; decidedAt: string | null } | null;
};
export type ApiHistory = { projection: "history"; jobId: string; type: string; status: string; asOf: string; completedAt: string | null; ownDecisionEvents: ApiJobEvent[]; redactedReportSummary?: unknown };
export type ApiPartnerJobDetail = ApiOffer | ApiDetail | ApiHistory;

const QUALIFICATIONS: Record<string, string> = { demo_indoor: "Indoor unit work", demo_outdoor: "Outdoor unit work", demo_electrical: "Electrical work" };
export const qualificationLabel = (code: string) => QUALIFICATIONS[code] ?? code.replace(/^demo_/, "").replace(/_/g, " ");
export const typeLabel = (t: string) => ({ periodic: "Periodic inspection", reactive: "Repair", preventive: "Preventive maintenance" })[t] ?? t;
export const originLabel = (o: string) => (o === "periodic_plan" ? "from the periodic plan" : "agreed with the client");
export const when = (iso: string) => klTime(iso); // YYYY-MM-DD HH:MM in Kuala Lumpur
export const range = (from: string, to: string) => (when(from).slice(0, 10) === when(to).slice(0, 10) ? `${when(from)}–${when(to).slice(11)}` : `${when(from)} – ${when(to)}`);
/** “1st”, “2nd”, “3rd”, “4th” … */
export const ordinal = (n: number) => `${n}${n % 10 === 1 && n % 100 !== 11 ? "st" : n % 10 === 2 && n % 100 !== 12 ? "nd" : n % 10 === 3 && n % 100 !== 13 ? "rd" : "th"}`;

/** Whether a member holds every required qualification, valid at the instant and not revoked. */
export function qualified(m: ApiMember, required: string[], at: string): boolean {
  const t = Date.parse(at);
  return required.every((code) => (m.qualifications ?? []).some((q) => q.code === code && !q.revokedAt && (!q.validFrom || Date.parse(q.validFrom) <= t) && (!q.validUntil || t < Date.parse(q.validUntil))));
}

/** Free hours of a capacity day (available minus assigned; “—” without a working pattern). */
export const freeHours = (c: ApiCapacity | undefined) => (!c || c.availableMinutes === null ? null : Math.max(0, Math.round(((c.availableMinutes - c.assignedMinutes) / 60) * 10) / 10));
const overlaps = (a: Slot, b: Slot) => Date.parse(a.startAt) < Date.parse(b.endAt) && Date.parse(b.startAt) < Date.parse(a.endAt);
const inside = (inner: Slot, outer: Slot) => Date.parse(outer.startAt) <= Date.parse(inner.startAt) && Date.parse(inner.endAt) <= Date.parse(outer.endAt);

export type FitRow = { id: string; name: string; sub: string; days: { label: string; text: string; free: boolean }[]; badge: { text: string; tone: "ok" | "warn" | "muted" } };
const md = (date: string) => date.slice(5);
/** Can the company take the visit: each technician's qualification, free hours on the visit day and the next days, and
 * whether the visit slot lies in an available slot and clear of assigned ones (members.capacity). The offer (Figma
 * 02-7) shows a chip per day; the delegated job (02-11) sums the free hours of the days into the line, and its assigned
 * technician is “Assigned” (their own booking is not a conflict). */
export function fits(members: ApiMember[], required: string[], visit: Slot, days: { date: string; capacity: ApiCapacity[] }[], mode: "offer" | "detail" = "offer", assignedId?: string): FitRow[] {
  return members.filter((m) => m.role === "technician").map((m) => {
    const ok = qualified(m, required, visit.startAt);
    const cap = days.map((d) => d.capacity.find((c) => c.membershipId === m.id));
    const visitDay = cap[0];
    const free = !!visitDay && visitDay.availableSlots.some((s) => inside(visit, s)) && !visitDay.assignedSlots.some((s) => overlaps(s, visit));
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => qualificationLabel(q.code));
    const missing = `missing ${required.filter((r) => !qualified(m, [r], visit.startAt)).map(qualificationLabel).join(", ")}`;
    if (mode === "detail") {
      const hours = cap.reduce((sum, c) => sum + (freeHours(c) ?? 0), 0);
      const span = days.length > 1 ? `${md(days[0].date)}–${md(days[days.length - 1].date)}` : md(days[0]?.date ?? "");
      if (m.id === assignedId) return { id: m.id, name: m.displayName, days: [], sub: ok ? "Qualified · assigned to this job" : `${missing.replace(/^m/, "M")} · assigned to this job`, badge: { text: "Assigned", tone: ok ? "ok" : "warn" } };
      return {
        id: m.id, name: m.displayName, days: [],
        sub: ok ? `Qualified · ${hours} h free ${span}` : missing.replace(/^m/, "M"),
        badge: !ok ? { text: "Not eligible", tone: "muted" } : free ? { text: "Best fit", tone: "ok" } : { text: "Busy at the visit", tone: "warn" },
      };
    }
    return {
      id: m.id, name: m.displayName, sub: ok ? quals.join(" · ") || "qualified" : missing,
      days: days.map((d, i) => { const h = freeHours(cap[i]); return { label: new Date(`${d.date}T00:00:00Z`).toLocaleDateString("en-GB", { weekday: "short", timeZone: "UTC" }), text: h === null ? "—" : `${h} h`, free: !!h }; }),
      badge: !ok ? { text: "Not qualified", tone: "muted" } : free ? { text: "Fits", tone: "ok" } : { text: "Busy at the visit", tone: "warn" },
    };
  });
}

export type Step = { label: string; at: string | null; sub: string; state: "done" | "current" | "todo" };
const ORDER = ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed"];
/** The status timeline of a delegated job (Figma 02-11) from its events; on hold and rework show on the current step. */
export function timeline(status: string, events: ApiJobEvent[], hasTechnician: boolean, origin?: string): Step[] {
  const at = (actions: string[]) => [...events].filter((e) => actions.includes(e.action)).sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))[0]?.occurredAt ?? null;
  const reached = status === "on_hold" || status === "rework_requested" ? ORDER.indexOf(hasTechnician ? "in_progress" : "accepted") : ORDER.indexOf(status === "cancelled" ? "requested" : status);
  const steps: [string, string[], string][] = [
    ["Requested", ["job.created"], origin === "periodic_plan" ? "periodic plan" : origin === "client_request" ? "client request" : "by the client or the plan"], ["Offered", ["job.offered"], "HQ offered your company"], ["Accepted", ["offer.accepted"], "you"],
    ["Assigned", ["job.assigned"], "from Schedule"], ["In progress", ["job.checked_in", "job.started"], "technician checks in on site"],
    ["Submitted", ["job.submitted"], "report sent for your review"], ["Completed", ["job.reviewed"], "after the quality review"],
  ];
  return steps.map(([label, actions, sub], i) => ({ label, at: at(actions), sub, state: i < reached ? "done" : i === reached ? (status === "completed" ? "done" : "current") : "todo" }));
}

/** The banner of a delegated job's state (Figma 02-11…02-13). */
export function detailBanner(j: ApiDetail, techName: string | null, now: number): { tone: "ok" | "warn" | "crit" | "primary"; text: string } | null {
  const decided = j.offer?.decidedAt ? ` · decided ${when(j.offer.decidedAt)}` : "";
  switch (j.status) {
    case "accepted":
      return { tone: "ok", text: `Accepted${decided} (receipt). Acceptance does not assign a technician or confirm a booking — assign one next.` };
    case "assigned":
    case "in_progress": {
      const a = j.assignment;
      const ended = a && Date.parse(a.scheduledEnd) <= now;
      if (ended) return { tone: "crit", text: `The work window ended ${when(a!.scheduledEnd)} without a submitted report — extend or reassign in Schedule (IR89).` };
      const ack = a?.acknowledgement === "accepted" ? "accepted by the technician ✓" : a?.acknowledgement === "cant_make" ? `the technician can’t make it${a.cantMakeReason ? ` (“${a.cantMakeReason}”)` : ""} — reassign in Schedule` : "awaiting the technician’s acceptance";
      return { tone: a?.acknowledgement === "cant_make" ? "warn" : "ok", text: `${j.status === "in_progress" ? "In progress" : "Assigned"} — ${techName ?? "technician"} · ${a ? slotText({ startAt: a.scheduledStart, endAt: a.scheduledEnd }) : ""} · ${ack}.` };
    }
    case "submitted":
      return { tone: "primary", text: "The report is submitted — review it before HQ releases it to the customer." };
    case "rework_requested":
      return { tone: "warn", text: "Returned for rework — your technician continues in a new draft version." };
    case "on_hold":
      return { tone: "warn", text: "On hold by HQ — scheduling and reports are blocked until HQ resumes the job." };
    case "completed":
      return { tone: "ok", text: `Completed${j.completedAt ? ` ${when(j.completedAt)}` : ""} — accepted in the quality review.` };
    case "cancelled":
      return { tone: "crit", text: "Cancelled by HQ — no further work." };
  }
  return null;
}

/** The delegation countdown (“7 d 14 h to schedule the work” while nobody is assigned). */
export const delegationLeft = (until_: string, now: number, toSchedule = false) =>
  (Date.parse(until_) <= now ? "The delegation period has ended." : `Delegation ends ${when(until_)} — ${until(until_, now)} ${toSchedule ? "to schedule the work" : "left"}.`);

const FIELD_TEXT: Record<string, string> = {
  "error.required": "required", "error.length": "required (1–1000 characters)", "error.slotRules": "pick a time on a later day, 1–4 hours long",
  "error.malformedInput": "the request was not understood — reload and try again",
};
/** Readable refusals of the offer decisions (DD-P02 boundary cases). */
export function decisionRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  switch (f.messageKey) {
    case "errors.offer_expired": return "This offer expired or was cancelled by HQ — no decision is possible. The page shows the latest state.";
    case "errors.terms_changed": return "The delegation terms changed — reload to read the new terms before deciding.";
    case "errors.partner_proposal_pending": return "Your time change is still waiting for the client — accept after it is answered or withdraw it.";
    case "errors.qualification_missing": return "That technician lacks a required qualification valid at the proposed time — choose another.";
    case "errors.technician_out_of_scope": case "errors.technician_not_allowed": return "That technician cannot be booked for this unit — choose another.";
    case "error.versionConflict": return "The offer changed elsewhere — the page shows the latest version.";
  }
  if (f.code === "NOT_FOUND") return "This offer is no longer available to your company.";
  if (f.code === "VALIDATION") return Object.entries(f.fieldErrors).map(([k, v]) => k === "_" ? FIELD_TEXT[v] ?? v : `${k}: ${FIELD_TEXT[v] ?? v}`).join(" · ") || "Check the input.";
  return `${f.code} — ${f.messageKey}`;
}

/** The job event rows of a history snapshot or the detail's history card. */
export const eventRows = (events: ApiJobEvent[]) => [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt)).map((e) => ({ id: e.id, at: when(e.occurredAt), text: jobEventTitle[e.action] ?? e.action.replace(/[._]/g, " ") }));
