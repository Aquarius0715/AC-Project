// The contractor overview (FR-P01, DD-P01, Figma Contractor 01-1…01-4) from the Core API: the period, the KPI tiles,
// the weekly progress by status, the jobs that need the company's action, today's timeline and the week's team
// capacity, and the recent job activity. Pure code shared by the server loader and the client view; Vitest covers it.
// Texts in the display language (`t` / `i`, IR270). The periods and today's timeline are Kuala Lumpur days and hours;
// deadlines, starts and the activity are instants in the user's display time zone (IR44).
import { klTime } from "@ac/web/lib/devices";
import { EN, relativeTime, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";

const en = translator("en");

export type Slot = { startAt: string; endAt: string };

/** jobs.list rows of a contractor (IR23): the offer before the access window, the summary inside it, the history after. */
export type ApiPartnerJob =
  | { projection: "offer"; status: "offered" | "accepted"; jobId: string; jobVersion: number; offerId: string; type: string; siteAddress: string | null; requestedSlot: Slot; dueAt: string; offerExpiresAt: string; visitSlot: Slot | null; accessValidFrom?: string; accessValidUntil?: string }
  | { projection: "summary"; id: string; version: number; unitId: string; type: string; status: string; dueAt: string; requestedSlot: Slot; scheduledSlot: Slot | null; assignmentId: string | null; displayStatus: string; technicianMembershipId: string | null; assignmentAcknowledgement?: "pending" | "accepted" | "cant_make" | null; accessValidFrom?: string | null; accessValidUntil?: string | null; origin?: string }
  | { projection: "history"; jobId: string; type: string; status: string; completedAt: string | null };
export type ApiCounts = { offerCount: number; activeCount: number; reviewCount: number; overdueCount: number };
export type ApiCapacity = { membershipId: string; date: string; availableSlots: Slot[]; assignedSlots: Slot[]; availableMinutes: number | null; assignedMinutes: number; unavailability: string | null };
export type ApiMember = { id: string; userId: string; displayName: string; role: string; qualifications?: { code: string; validFrom?: string; validUntil?: string | null; revokedAt?: string | null }[] };
export type ApiJobEvent = { id: string; jobId: string; actorUserId: string | null; action: string; occurredAt: string };

/** One job of the company as the overview shows it. */
export type PartnerJob = {
  id: string; short: string; status: string; type: string; unit: string | null; address: string | null;
  slot: Slot | null; dueAt: string | null; offerExpiresAt: string | null; assignmentId: string | null; technicianId: string | null;
};

const KL_OFFSET = 8 * 3600_000; // Asia/Kuala_Lumpur, no daylight saving
const day = 24 * 3600_000;
const ymd = (ms: number) => new Date(ms + KL_OFFSET).toISOString().slice(0, 10);

/** The Monday–Sunday week (KL dates) of an instant. */
export function weekOf(nowIso: string): { from: string; to: string } {
  const local = Date.parse(nowIso) + KL_OFFSET;
  const weekday = (new Date(local).getUTCDay() + 6) % 7; // Monday 0
  const monday = Math.floor(local / day) * day - weekday * day;
  return { from: new Date(monday).toISOString().slice(0, 10), to: new Date(monday + 6 * day).toISOString().slice(0, 10) };
}

/** The period of the URL (from/to KL dates, at most 366 days) or this week, and its half-open instant range. */
export function period(nowIso: string, from?: string, to?: string): { from: string; to: string; fromAt: string; toAt: string; days: string[]; label: string } {
  const ok = (d?: string) => !!d && /^\d{4}-\d{2}-\d{2}$/.test(d) && !Number.isNaN(Date.parse(`${d}T00:00:00Z`));
  let p = weekOf(nowIso);
  if (ok(from) && ok(to) && from! <= to! && Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`) <= 365 * day) p = { from: from!, to: to! };
  const start = Date.parse(`${p.from}T00:00:00Z`) - KL_OFFSET, end = Date.parse(`${p.to}T00:00:00Z`) - KL_OFFSET + day;
  const days: string[] = [];
  for (let t = start; t < end && days.length < 31; t += day) days.push(ymd(t));
  const week = weekOf(nowIso);
  const label = p.from === week.from && p.to === week.to ? "This week" : p.from === shift(week, -7).from && p.to === shift(week, -7).to ? "Last week" : p.from === shift(week, 7).from && p.to === shift(week, 7).to ? "Next week" : "Period";
  return { ...p, fromAt: new Date(start).toISOString(), toAt: new Date(end).toISOString(), days, label };
}

/** A week moved by n days. */
export function shift(p: { from: string; to: string }, n: number) {
  const mv = (d: string) => new Date(Date.parse(`${d}T00:00:00Z`) + n * day).toISOString().slice(0, 10);
  return { from: mv(p.from), to: mv(p.to) };
}

/** A jobs.list row with its unit name; the summary names the active assignment's technician (IR225). */
export function partnerJob(j: ApiPartnerJob, units: Map<string, string>): PartnerJob {
  if (j.projection === "offer") {
    return { id: j.jobId, short: j.jobId.slice(0, 8), status: j.status, type: j.type, unit: null, address: j.siteAddress, slot: j.visitSlot, dueAt: j.dueAt, offerExpiresAt: j.status === "offered" ? j.offerExpiresAt : null, assignmentId: null, technicianId: null };
  }
  if (j.projection === "history") {
    return { id: j.jobId, short: j.jobId.slice(0, 8), status: j.status, type: j.type, unit: null, address: null, slot: null, dueAt: null, offerExpiresAt: null, assignmentId: null, technicianId: null };
  }
  return {
    id: j.id, short: j.id.slice(0, 8), status: j.status, type: j.type, unit: units.get(j.unitId) ?? null, address: null,
    slot: j.scheduledSlot, dueAt: j.dueAt, offerExpiresAt: null, assignmentId: j.assignmentId, technicianId: j.technicianMembershipId ?? null,
  };
}

export type Bucket = "offered" | "unassigned" | "active" | "overdue" | "review" | "completed" | "other";
/** The progress bucket of a job: overdue is an assigned or started job whose work window has ended (IR89). */
export function bucket(j: PartnerJob, now: number): Bucket {
  switch (j.status) {
    case "offered":
      return "offered";
    case "accepted":
      return j.assignmentId ? "active" : "unassigned";
    case "assigned":
    case "in_progress":
      return j.slot && Date.parse(j.slot.endAt) <= now ? "overdue" : "active";
    case "rework_requested":
    case "on_hold":
      return "active";
    case "submitted":
      return "review";
    case "completed":
      return "completed";
  }
  return "other";
}

const typeText: Record<string, string> = { periodic: "Periodic inspection", reactive: "Repair", preventive: "Preventive maintenance" };
const label = (j: PartnerJob, t: T = en) => [j.short, j.unit ?? j.address ?? (typeText[j.type] ? t(typeText[j.type]) : j.type)].filter(Boolean).join(" · ");
export const hhmm = (iso: string) => klTime(iso).slice(11);
/** "15 h", "3 d 8 h" or "45 min" until an instant (negative → "ended"), in the display language. */
export function until(iso: string, now: number, t: T = en): string {
  const m = Math.round((Date.parse(iso) - now) / 60_000);
  if (m <= 0) return t("ended");
  if (m < 60) return t("{n} min", { n: m });
  const h = Math.floor(m / 60);
  if (h < 24) return t("{n} h", { n: h });
  return h % 24 ? t("{d} d {h} h", { d: Math.floor(h / 24), h: h % 24 }) : t("{d} d", { d: Math.floor(h / 24) });
}

export type KpiTile = { label: string; value: number; sub: string; href: string; link: string; tone?: "warn" | "crit" };
/** The five KPI tiles (DD-P01): offers, review and overdue from summaries.get; the accepted jobs without a technician
 * from the list, and in progress / scheduled as the summary's active jobs without them. Each opens the job list. */
export function kpis(counts: ApiCounts, jobs: PartnerJob[], now: number, names: Map<string, string>, period?: { from: string; to: string }, i: I18n = EN): KpiTile[] {
  const { t } = i;
  const list = (tab: string) => `/partner/jobs?tab=${tab}${period ? `&from=${period.from}&to=${period.to}` : ""}`; // the list with the same period
  const offers = jobs.filter((j) => j.status === "offered" && j.offerExpiresAt).sort((a, b) => Date.parse(a.offerExpiresAt!) - Date.parse(b.offerExpiresAt!));
  const unassigned = jobs.filter((j) => bucket(j, now) === "unassigned");
  const running = jobs.filter((j) => (j.status === "assigned" || j.status === "in_progress") && j.slot).sort((a, b) => Date.parse(a.slot!.startAt) - Date.parse(b.slot!.startAt));
  const next = running.find((j) => j.status === "in_progress") ?? running.find((j) => Date.parse(j.slot!.endAt) > now);
  const review = jobs.filter((j) => j.status === "submitted");
  const active = Math.max(0, counts.activeCount - unassigned.length);
  const who = (id: string | null) => names.get(id ?? "") ?? t("Technician");
  return [
    { label: t("Offers to answer"), value: counts.offerCount, sub: offers[0] ? t("{job} · expires in {left}", { job: offers[0].short, left: until(offers[0].offerExpiresAt!, now, t) }) : t("No open offers"), href: list("offered"), link: t("Open offers →") },
    { label: t("Awaiting assignment"), value: unassigned.length, sub: unassigned.length ? t("Accepted, no technician yet") : "—", href: "/partner/schedule", link: t("Assign →"), tone: unassigned.length ? "warn" : undefined },
    {
      label: t("In progress / scheduled"), value: active, href: list("active"), link: t("View jobs →"),
      sub: next ? t(next.status === "in_progress" ? "{name} on site from {time}" : "{name} starts {time}", { name: who(next.technicianId), time: relativeTime(next.slot!.startAt, now, i) }) : "—",
    },
    { label: t("Reports to review"), value: counts.reviewCount, sub: review[0] ? t("{job} · awaiting review", { job: label(review[0], t) }) : "—", href: review.length === 1 ? `/partner/jobs/${review[0].id}/review` : list("review"), link: t("Review →") },
    { label: t("Overdue"), value: counts.overdueCount, sub: counts.overdueCount ? t("Past the due date or the work window (IR89)") : "—", href: list("active"), link: t("View →"), tone: counts.overdueCount ? "crit" : undefined },
  ];
}

export const BUCKETS: { id: Exclude<Bucket, "other">; label: string; tone: string }[] = [
  { id: "offered", label: "Offered", tone: "bg-warn/25" }, { id: "unassigned", label: "Accepted · unassigned", tone: "bg-primary/20" },
  { id: "active", label: "Assigned / in progress", tone: "bg-primary" }, { id: "overdue", label: "Overdue", tone: "bg-crit/70" },
  { id: "review", label: "Submitted · in review", tone: "bg-warn" }, { id: "completed", label: "Completed", tone: "bg-ok" },
];
/** The progress bar: jobs per bucket with the same period as the list. */
export function progress(jobs: PartnerJob[], now: number) {
  const counts = Object.fromEntries(BUCKETS.map((b) => [b.id, 0])) as Record<Exclude<Bucket, "other">, number>;
  for (const j of jobs) {
    const b = bucket(j, now);
    if (b !== "other") counts[b]++;
  }
  const total = BUCKETS.reduce((s, b) => s + counts[b.id], 0);
  return { counts, total, completedPct: total ? Math.round((counts.completed / total) * 100) : 0 };
}

export type ActionRow = { kind: "offer" | "overdue" | "review" | "unassigned"; badge: string; title: string; detail: string; href: string; button: string };
/** Needs your action (DD-P01): offers to answer, ended work windows, submitted reports, accepted jobs without a technician. */
export function actions(jobs: PartnerJob[], now: number, names: Map<string, string>, i: I18n = EN): ActionRow[] {
  const { t, display } = i;
  const rows: ActionRow[] = [];
  for (const j of jobs.filter((x) => x.status === "offered").sort((a, b) => Date.parse(a.offerExpiresAt ?? a.dueAt ?? "") - Date.parse(b.offerExpiresAt ?? b.dueAt ?? ""))) {
    const detail = j.offerExpiresAt ? t("Answer by {time} ({left} left)", { time: showTime(j.offerExpiresAt, display), left: until(j.offerExpiresAt, now, t) }) : t("Answer the offer");
    rows.push({ kind: "offer", badge: t("Offer"), title: label(j, t), detail, href: `/partner/jobs/${j.id}`, button: t("Respond") });
  }
  for (const j of jobs.filter((x) => bucket(x, now) === "overdue")) {
    const ended = t("Work window ended {time}", { time: showTime(j.slot!.endAt, display) });
    rows.push({ kind: "overdue", badge: t("Overdue"), title: label(j, t), detail: j.technicianId ? `${ended} · ${names.get(j.technicianId) ?? t("technician")}` : ended, href: `/partner/schedule?jobId=${j.id}`, button: t("Reassign") });
  }
  for (const j of jobs.filter((x) => x.status === "submitted")) {
    const by = j.technicianId ? t("Report by {name} · awaiting review", { name: names.get(j.technicianId) ?? t("your technician") }) : t("Report · awaiting review");
    rows.push({ kind: "review", badge: t("Submitted"), title: label(j, t), detail: by, href: `/partner/jobs/${j.id}/review`, button: t("Review") });
  }
  for (const j of jobs.filter((x) => bucket(x, now) === "unassigned")) {
    const detail = j.dueAt ? t("No technician · due {time}", { time: showTime(j.dueAt, display) }) : t("No technician");
    rows.push({ kind: "unassigned", badge: t("Accepted"), title: label(j, t), detail, href: `/partner/schedule?jobId=${j.id}`, button: t("Assign") });
  }
  return rows;
}

export const DAY_START = 8, DAY_END = 18; // the timeline runs 08:00–18:00 in Kuala Lumpur
/** Position (0–100 %) of an instant on the date's 08:00–18:00 timeline, clamped. */
export function timelinePct(iso: string, date: string): number {
  const start = Date.parse(`${date}T00:00:00Z`) - KL_OFFSET + DAY_START * 3600_000;
  return Math.min(100, Math.max(0, ((Date.parse(iso) - start) / ((DAY_END - DAY_START) * 3600_000)) * 100));
}

export type TimelineRow = { id: string; name: string; sub: string; blocks: { left: number; width: number; text: string }[]; off: string | null };
/** Today's timeline per technician from members.capacity of the date (assigned slots inside 08:00–18:00), in Kuala
 * Lumpur hours like its axis. */
export function timeline(today: ApiCapacity[], members: ApiMember[], date: string, t: T = en): TimelineRow[] {
  const cap = new Map(today.map((c) => [c.membershipId, c]));
  return members.filter((m) => m.role === "technician").map((m) => {
    const c = cap.get(m.id);
    const blocks = (c?.assignedSlots ?? []).map((s) => {
      const left = timelinePct(s.startAt, date), right = timelinePct(s.endAt, date);
      return { left, width: Math.max(right - left, 0), text: `${hhmm(s.startAt)}–${hhmm(s.endAt)}` };
    }).filter((b) => b.width > 0);
    const quals = (m.qualifications ?? []).filter((q) => !q.revokedAt).map((q) => q.code.replace(/^demo_/, "").replace(/_/g, " "));
    return { id: m.id, name: m.displayName, sub: quals.slice(0, 2).join(", ") || t("Technician"), blocks, off: c?.unavailability ? c.unavailability.replace(/_/g, " ") : null };
  });
}

export type CapacityRow = { id: string; name: string; text: string; pct: number | null };
/** The period's team capacity: assigned ÷ available hours per technician (undefined availability shows “—”). */
export function capacity(days: ApiCapacity[][], members: ApiMember[], t: T = en): CapacityRow[] {
  const sum = new Map<string, { assigned: number; available: number | null }>();
  for (const list of days) {
    for (const c of list) {
      const s = sum.get(c.membershipId) ?? { assigned: 0, available: null };
      s.assigned += c.assignedMinutes;
      if (c.availableMinutes !== null) s.available = (s.available ?? 0) + c.availableMinutes;
      sum.set(c.membershipId, s);
    }
  }
  const h = (min: number) => t("{n} h", { n: Math.round((min / 60) * 10) / 10 });
  return members.filter((m) => m.role === "technician").map((m) => {
    const s = sum.get(m.id) ?? { assigned: 0, available: null };
    const pct = s.available ? Math.min(100, Math.round((s.assigned / s.available) * 100)) : null;
    return { id: m.id, name: m.displayName, text: `${h(s.assigned)} / ${s.available === null ? "—" : h(s.available)} · ${pct === null ? "—" : `${pct}%`}`, pct };
  });
}

/** Titles of the job events (jobs.events actions). */
export const jobEventTitle: Record<string, string> = {
  "job.created": "Job created", "job.offered": "Offer received from HQ", "offer.accepted": "Job accepted", "offer.declined": "Offer declined", offer_expired: "Offer expired",
  "offer.access_extended": "Access window extended", "job.assigned": "Technician assigned", "assignment.accepted": "Technician confirmed the slot", "assignment.cant_make": "Technician can’t make the slot",
  "job.checked_in": "Checked in on site", "job.started": "Work started", "job.paused": "Work paused", "job.resumed_work": "Work resumed", "job.submitted": "Report submitted",
  "partner.review": "Partner review", "job.reviewed": "Report reviewed", "job.rework_started": "Rework started", "job.costs_saved": "Costs saved", "job.held": "Job on hold",
  "job.resumed": "Job resumed", "job.cancelled": "Job cancelled", "job.reschedule_requested": "Reschedule requested", "proposal.sent": "Time proposed", "proposal.accepted": "Proposal accepted",
  "proposal.declined": "Proposal declined", "proposal.withdrawn": "Proposal withdrawn", "note.added": "Note added", "job.rated": "Customer rating", "job.problem_reported": "Problem reported",
  "job.follow_up_classified": "Follow-up classified", "job.warranty_claim_filed": "Warranty claim filed",
  "partner_proposal.sent": "Time change proposed to HQ", "partner_proposal.withdrawn": "Time change withdrawn", "partner_proposal.sent_to_client": "HQ asked the client about the time change",
  "partner_proposal.kept": "HQ kept the agreed time",
};

export type ActivityRow = { id: string; time: string; text: string; href: string };
/** The latest job events of the company's jobs, newest first, with the actor's name when it is a member; the time is
 * “today …” on the days next to now (IR44). */
export function activity(events: ApiJobEvent[], jobs: Map<string, PartnerJob>, members: ApiMember[], now: number, max = 6, i: I18n = EN): ActivityRow[] {
  const { t } = i;
  const byUser = new Map(members.map((m) => [m.userId, m.displayName]));
  return [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt) || b.id.localeCompare(a.id)).slice(0, max).map((e) => {
    const j = jobs.get(e.jobId);
    const who = e.actorUserId ? byUser.get(e.actorUserId) : null;
    const title = jobEventTitle[e.action] ? t(jobEventTitle[e.action]) : e.action.replace(/[._]/g, " ");
    return { id: e.id, time: relativeTime(e.occurredAt, now, i), text: `${title} — ${j ? label(j, t) : e.jobId.slice(0, 8)}${who ? ` (${who})` : ""}`, href: `/partner/history?jobId=${e.jobId}` };
  });
}
