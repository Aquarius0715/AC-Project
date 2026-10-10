// The contractor's offer and job detail (FR-P02, FR-P08, DD-P02, Figma Contractor 02-7…02-14) from the Core API:
// what a projection may show (an offer only the job type, the registered site address, the required qualifications
// and the fixed visit; the detail inside the delegation period; the history after it), which of the company's
// technicians fit the visit, the status timeline from the job events and the refusals of the decisions.
// Pure code shared by the server loader and the client view; Vitest covers it. Texts in the display language (`t` / `i`,
// IR272); the loader formats every time on the server — instants in the user's display time zone, the capacity days
// as Kuala Lumpur dates — while `when` / `range` keep the Kuala Lumpur form for the screens not translated yet.
import { klTime } from "@ac/web/lib/devices";
import { EN, intlTag, showSpan, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { jobEventTitle, klDay, until, type ApiCapacity, type ApiJobEvent, type ApiMember, type Slot } from "@ac/web/lib/partnerOverview";

const en = translator("en");

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
  assignment: { technicianMembershipId: string; scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: string; cantMakeReason: string | null; alternativeSlot?: Slot | null } | null;
  offer: { id: string; termsVersion: string; visitSlot: Slot; offeredAt: string; accessValidFrom: string; accessValidUntil: string; decision: string | null; decidedAt: string | null } | null;
};
export type ApiHistory = { projection: "history"; jobId: string; type: string; status: string; asOf: string; completedAt: string | null; ownDecisionEvents: ApiJobEvent[]; redactedReportSummary?: unknown };
export type ApiPartnerJobDetail = ApiOffer | ApiDetail | ApiHistory;

const QUALIFICATIONS: Record<string, string> = { demo_indoor: "Indoor unit work", demo_outdoor: "Outdoor unit work", demo_electrical: "Electrical work" };
export const qualificationLabel = (code: string, t: T = en) => (QUALIFICATIONS[code] ? t(QUALIFICATIONS[code]) : code.replace(/^demo_/, "").replace(/_/g, " "));
const SCOPE_QUALIFICATION: Record<string, string> = { indoor: "demo_indoor", outdoor: "demo_outdoor", electrical: "demo_electrical" }; // fixture qualificationRequirements
/** The qualifications a delegated job requires: one per entry of the unit's maintenance scope (IR123 item 3). */
export const requiredFor = (serviceScope: string[]) => serviceScope.map((s) => SCOPE_QUALIFICATION[s]).filter(Boolean);
const TYPES: Record<string, string> = { periodic: "Periodic inspection", reactive: "Repair", preventive: "Preventive maintenance" };
export const typeLabel = (type: string, t: T = en) => (TYPES[type] ? t(TYPES[type]) : type);
export const originLabel = (o: string, t: T = en) => t(o === "periodic_plan" ? "from the periodic plan" : "agreed with the client");
const STATUS_WORDS: Record<string, string> = {
  offered: "offered", accepted: "accepted", assigned: "assigned", in_progress: "in progress", on_hold: "on hold", submitted: "submitted",
  rework_requested: "rework requested", completed: "completed", cancelled: "cancelled",
};
/** A job status inside a sentence (“the job is on hold”), in the display language. */
export const statusWord = (status: string, t: T = en) => (STATUS_WORDS[status] ? t(STATUS_WORDS[status]) : status.replace(/_/g, " "));
export const when = (iso: string) => klTime(iso); // YYYY-MM-DD HH:MM in Kuala Lumpur
export const range = (from: string, to: string) => (when(from).slice(0, 10) === when(to).slice(0, 10) ? `${when(from)}–${when(to).slice(11)}` : `${when(from)} – ${when(to)}`);
/** “1st”, “2nd”, “3rd”, “4th” … in English, “ke-1” in Malay. */
export const ordinal = (n: number, locale: "en" | "ms" = "en") => (locale === "ms" ? `ke-${n}` : `${n}${n % 10 === 1 && n % 100 !== 11 ? "st" : n % 10 === 2 && n % 100 !== 12 ? "nd" : n % 10 === 3 && n % 100 !== 13 ? "rd" : "th"}`);

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
/** Can the company take the visit: each technician's qualification, free hours on the visit day and the next days, and
 * whether the visit slot lies in an available slot and clear of assigned ones (members.capacity). The offer (Figma
 * 02-7) shows a chip per day; the delegated job (02-11) sums the free hours of the days into the line, and its assigned
 * technician is “Assigned” (their own booking is not a conflict). The days are Kuala Lumpur dates in the user's
 * language; call it on the server. */
export function fits(members: ApiMember[], required: string[], visit: Slot, days: { date: string; capacity: ApiCapacity[] }[], mode: "offer" | "detail" = "offer", assignedId?: string, i: I18n = EN): FitRow[] {
  const { t, display: { locale } } = i;
  const hoursText = (h: number) => t("{n} h", { n: h });
  return members.filter((m) => m.role === "technician").map((m) => {
    const ok = qualified(m, required, visit.startAt);
    const cap = days.map((d) => d.capacity.find((c) => c.membershipId === m.id));
    const visitDay = cap[0];
    const free = !!visitDay && visitDay.availableSlots.some((s) => inside(visit, s)) && !visitDay.assignedSlots.some((s) => overlaps(s, visit));
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => qualificationLabel(q.code, t));
    const lacking = required.filter((r) => !qualified(m, [r], visit.startAt)).map((r) => qualificationLabel(r, t)).join(", ");
    if (mode === "detail") {
      const hours = cap.reduce((sum, c) => sum + (freeHours(c) ?? 0), 0);
      const span = days.length > 1 ? `${klDay(days[0].date, locale)} – ${klDay(days[days.length - 1].date, locale)}` : days[0] ? klDay(days[0].date, locale) : "";
      if (m.id === assignedId) return { id: m.id, name: m.displayName, days: [], sub: ok ? t("Qualified · assigned to this job") : t("Missing {list} · assigned to this job", { list: lacking }), badge: { text: t("Assigned"), tone: ok ? "ok" : "warn" } };
      return {
        id: m.id, name: m.displayName, days: [],
        sub: ok ? t("Qualified · {hours} free {days}", { hours: hoursText(hours), days: span }) : t("Missing {list}", { list: lacking }),
        badge: !ok ? { text: t("Not eligible"), tone: "muted" } : free ? { text: t("Best fit"), tone: "ok" } : { text: t("Busy at the visit"), tone: "warn" },
      };
    }
    return {
      id: m.id, name: m.displayName, sub: ok ? quals.join(" · ") || t("qualified") : t("missing {list}", { list: lacking }),
      days: days.map((d, k) => {
        const h = freeHours(cap[k]);
        return { label: new Date(`${d.date}T00:00:00Z`).toLocaleDateString(intlTag(locale), { weekday: "short", timeZone: "UTC" }), text: h === null ? "—" : hoursText(h), free: !!h };
      }),
      badge: !ok ? { text: t("Not qualified"), tone: "muted" } : free ? { text: t("Fits"), tone: "ok" } : { text: t("Busy at the visit"), tone: "warn" },
    };
  });
}

export type Step = { label: string; at: string | null; atText: string | null; sub: string; state: "done" | "current" | "todo" };
const ORDER = ["requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed"];
/** The status timeline of a delegated job (Figma 02-11) from its events; on hold and rework show on the current step.
 * `atText` is the step's time in the user's display time zone (IR44). */
export function timeline(status: string, events: ApiJobEvent[], hasTechnician: boolean, origin?: string, i: I18n = EN): Step[] {
  const { t, display } = i;
  const at = (actions: string[]) => [...events].filter((e) => actions.includes(e.action)).sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))[0]?.occurredAt ?? null;
  const reached = status === "on_hold" || status === "rework_requested" ? ORDER.indexOf(hasTechnician ? "in_progress" : "accepted") : ORDER.indexOf(status === "cancelled" ? "requested" : status);
  const steps: [string, string[], string][] = [
    ["Requested", ["job.created"], origin === "periodic_plan" ? "periodic plan" : origin === "client_request" ? "client request" : "by the client or the plan"], ["Offered", ["job.offered"], "HQ offered your company"], ["Accepted", ["offer.accepted"], "you"],
    ["Assigned", ["job.assigned"], "from Schedule"], ["In progress", ["job.checked_in", "job.started"], "technician checks in on site"],
    ["Submitted", ["job.submitted"], "report sent for your review"], ["Completed", ["job.reviewed"], "after the quality review"],
  ];
  return steps.map(([label, actions, sub], k) => {
    const when_ = at(actions);
    return { label: t(label), at: when_, atText: when_ ? showTime(when_, display) : null, sub: t(sub), state: k < reached ? "done" : k === reached ? (status === "completed" ? "done" : "current") : "todo" };
  });
}

/** The banner of a delegated job's state (Figma 02-11…02-13), with its times in the user's display time zone. */
export function detailBanner(j: ApiDetail, techName: string | null, now: number, i: I18n = EN): { tone: "ok" | "warn" | "crit" | "primary"; text: string } | null {
  const { t, display } = i;
  switch (j.status) {
    case "accepted":
      return { tone: "ok", text: j.offer?.decidedAt ? t("Accepted · decided {time} (receipt). Acceptance does not assign a technician or confirm a booking — assign one next.", { time: showTime(j.offer.decidedAt, display) })
        : t("Accepted (receipt). Acceptance does not assign a technician or confirm a booking — assign one next.") };
    case "assigned":
    case "in_progress": {
      const a = j.assignment;
      const ended = a && Date.parse(a.scheduledEnd) <= now;
      if (ended) return { tone: "crit", text: t("The work window ended {time} without a submitted report — extend or reassign in Schedule (IR89).", { time: showTime(a!.scheduledEnd, display) }) };
      const ack = a?.acknowledgement === "accepted" ? t("accepted by the technician ✓")
        : a?.acknowledgement === "cant_make" ? (a.cantMakeReason ? t("the technician can’t make it (“{reason}”) — reassign in Schedule", { reason: a.cantMakeReason }) : t("the technician can’t make it — reassign in Schedule"))
        : t("awaiting the technician’s acceptance");
      const slot = a ? showSpan(a.scheduledStart, a.scheduledEnd, display) : "";
      return { tone: a?.acknowledgement === "cant_make" ? "warn" : "ok", text: t(j.status === "in_progress" ? "In progress — {name} · {slot} · {ack}." : "Assigned — {name} · {slot} · {ack}.", { name: techName ?? t("technician"), slot, ack }) };
    }
    case "submitted":
      return { tone: "primary", text: t("The report is submitted — review it before HQ releases it to the customer.") };
    case "rework_requested":
      return { tone: "warn", text: t("Returned for rework — your technician continues in a new draft version.") };
    case "on_hold":
      return { tone: "warn", text: t("On hold by HQ — scheduling and reports are blocked until HQ resumes the job.") };
    case "completed": {
      const name = techName ?? t("The technician");
      return { tone: "ok", text: j.completedAt ? t("Completed {time} — accepted in the quality review. {name}’s assignment ended with it, so the time is free for other visits (IR234).", { time: showTime(j.completedAt, display), name })
        : t("Completed — accepted in the quality review. {name}’s assignment ended with it, so the time is free for other visits (IR234).", { name }) };
    }
    case "cancelled":
      return { tone: "crit", text: t("Cancelled by HQ — no further work.") };
  }
  return null;
}

/** The delegation countdown (“7 d 14 h to schedule the work” while nobody is assigned), its end in the display zone. */
export const delegationLeft = (until_: string, now: number, toSchedule = false, i: I18n = EN) => {
  const { t, display } = i;
  if (Date.parse(until_) <= now) return t("The delegation period has ended.");
  const p = { time: showTime(until_, display), left: until(until_, now, t) };
  return t(toSchedule ? "Delegation ends {time} — {left} to schedule the work." : "Delegation ends {time} — {left} left.", p);
};

const FIELD_TEXT: Record<string, string> = {
  "error.required": "required", "error.length": "required (1–1000 characters)", "error.slotRules": "pick a time on a later day, 1–4 hours long",
  "error.malformedInput": "the request was not understood — reload and try again",
};
/** Readable refusals of the offer decisions (DD-P02 boundary cases) in the display language; a field is named by its
 * API key. */
export function decisionRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): string {
  switch (f.messageKey) {
    case "errors.offer_expired": return t("This offer expired or was cancelled by HQ — no decision is possible. The page shows the latest state.");
    case "errors.terms_changed": return t("The delegation terms changed — reload to read the new terms before deciding.");
    case "errors.partner_proposal_pending": return t("Your time change is still waiting for the client — accept after it is answered or withdraw it.");
    case "errors.qualification_missing": return t("That technician lacks a required qualification valid at the proposed time — choose another.");
    case "errors.technician_out_of_scope": case "errors.technician_not_allowed": return t("That technician cannot be booked for this unit — choose another.");
    case "error.versionConflict": return t("The offer changed elsewhere — the page shows the latest version.");
  }
  if (f.code === "NOT_FOUND") return t("This offer is no longer available to your company.");
  const text = (v: string) => (FIELD_TEXT[v] ? t(FIELD_TEXT[v]) : v);
  if (f.code === "VALIDATION") return Object.entries(f.fieldErrors).map(([k, v]) => k === "_" ? text(v) : `${k}: ${text(v)}`).join(" · ") || t("Check the input.");
  return `${f.code} — ${f.messageKey}`;
}

/** The job event rows of a history snapshot or the detail's history card, the times in the display time zone. */
export const eventRows = (events: ApiJobEvent[], i: I18n = EN) => [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))
  .map((e) => ({ id: e.id, at: showTime(e.occurredAt, i.display), text: jobEventTitle[e.action] ? i.t(jobEventTitle[e.action]) : e.action.replace(/[._]/g, " ") }));
