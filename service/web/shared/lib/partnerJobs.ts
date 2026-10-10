// The contractor job list (FR-P01, DD-P01 job list, Figma Contractor 02-1…02-6) from the Core API: status tabs with
// their counts, the sort (IR34) and the period of the overview, and one row per job with the technician, the visit
// slot or deadline and the status badge; rows open the offer, the job detail, the schedule or the quality review.
// Pure code shared by the server loader and the client view; Vitest covers it. Rows in the display language with the
// slots and deadlines in the user's display time zone (IR271); `slotText` keeps the Kuala Lumpur form for the
// screens not translated yet.
import { klTime } from "@ac/web/lib/devices";
import { EN, showSpan, showTime, type I18n } from "@ac/web/lib/i18n";
import { bucket, hhmm, partnerJob, until, type ApiPartnerJob, type Slot } from "@ac/web/lib/partnerOverview";

export const TABS = [
  { id: "all", label: "All", statuses: null },
  { id: "offered", label: "Offered", statuses: ["offered"] },
  { id: "active", label: "Active", statuses: ["accepted", "assigned", "in_progress", "on_hold", "rework_requested"] },
  { id: "review", label: "Review", statuses: ["submitted"] },
  { id: "completed", label: "Completed", statuses: ["completed"] },
] as const;
export type TabId = (typeof TABS)[number]["id"];
export const tabOf = (v?: string): TabId => (TABS.find((t) => t.id === v)?.id ?? "all");
/** The jobs.list filters of a tab within the period. */
export function tabFilters(tab: TabId, fromAt: string, toAt: string): Record<string, unknown> {
  const statuses = TABS.find((t) => t.id === tab)!.statuses;
  return { from: fromAt, to: toAt, ...(statuses ? { statuses: [...statuses] } : {}) };
}

export const SORTS = [
  { id: "status:asc", label: "Status ↑", text: "status, then deadline" }, { id: "status:desc", label: "Status ↓", text: "status (last first)" },
  { id: "dueAt:asc", label: "Deadline ↑", text: "deadline (soonest first)" }, { id: "dueAt:desc", label: "Deadline ↓", text: "deadline (latest first)" },
  { id: "severity:desc", label: "Unit urgency ↓", text: "unit urgency (IR30)" },
] as const;
export type SortId = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): SortId => (SORTS.find((s) => s.id === v)?.id ?? "status:asc");
export const sortSpec = (id: SortId) => { const [field, direction] = id.split(":"); return { field, direction }; };

export const PAGE_SIZE = 25;

type Tone = "primary" | "ok" | "warn" | "crit" | "muted";
export type JobRow = {
  id: string; title: string; origin: string | null; line: string; href: string;
  /** `subTone` colours the line under the name (a technician who can't make the slot); "Unassigned" colours only the name. */
  tech: { name: string; sub: string; tone?: Tone; subTone?: Tone }; slot: { main: string; sub: string; tone?: Tone }; badge: { text: string; tone: Tone };
};

const mmddhm = (iso: string) => klTime(iso).slice(5);
/** A slot in Kuala Lumpur as "09-21 10:00–12:00" (schedule and job detail until they are translated). */
export const slotText = (s: Slot) => (klTime(s.startAt).slice(0, 10) === klTime(s.endAt).slice(0, 10) ? `${mmddhm(s.startAt)}–${hhmm(s.endAt)}` : `${mmddhm(s.startAt)} → ${mmddhm(s.endAt)}`);
const typeText: Record<string, string> = { periodic: "Periodic inspection", reactive: "Repair", preventive: "Preventive maintenance" };
const ack: Record<string, { sub: string; tone?: Tone }> = { pending: { sub: "awaiting the technician’s acceptance" }, accepted: { sub: "accepted ✓" }, cant_make: { sub: "can’t make it ⚠ — reassign", tone: "warn" } };
const historyStatus: Record<string, string> = { completed: "Completed", cancelled: "Cancelled" };

/** One list row: what the company sees of a job in its projection (offer, summary inside the window, history after). */
export function jobRow(raw: ApiPartnerJob, now: number, units: Map<string, string>, names: Map<string, string>, i: I18n = EN): JobRow {
  const { t, display } = i;
  const j = partnerJob(raw, units);
  const typeLabel = (type: string) => (typeText[type] ? t(typeText[type]) : type);
  const span = (x: Slot) => showSpan(x.startAt, x.endAt, display);
  const at = (iso: string) => showTime(iso, display);
  const place = j.unit ?? j.address ?? typeLabel(j.type);
  const title = `${j.short} · ${place}`;
  const tech = j.technicianId ? names.get(j.technicianId) ?? t("Technician") : null;
  const origin = "origin" in raw ? (raw as { origin?: string }).origin ?? null : null;
  const b = bucket(j, now);
  const detail = `/partner/jobs/${j.id}`;
  const none = { name: "—", sub: "" };
  if (raw.projection === "offer" && raw.status === "offered") {
    const proposal = (raw as { partnerSlotProposal?: { status: string } | null }).partnerSlotProposal;
    const pending = proposal && (proposal.status === "pending" || proposal.status === "sent_to_client");
    const left = until(raw.offerExpiresAt, now, t);
    return {
      id: j.id, title, origin, href: detail,
      line: pending ? t("Time change proposed · waiting for HQ and the client") : t("Offered by HQ · visit {slot} (fixed)", { slot: raw.visitSlot ? span(raw.visitSlot) : "—" }),
      tech: { name: "—", sub: t("not assignable before acceptance") },
      slot: { main: t("Answer by {time}", { time: at(raw.offerExpiresAt) }), sub: Date.parse(raw.offerExpiresAt) <= now ? left : t("{left} left", { left }), tone: Date.parse(raw.offerExpiresAt) - now < 6 * 3600_000 ? "warn" : undefined },
      badge: pending ? { text: t("Time proposed"), tone: "warn" } : { text: t("Offered"), tone: "primary" },
    };
  }
  if (raw.projection === "offer") { // accepted before the access window opens
    return {
      id: j.id, title, origin, href: `/partner/schedule?jobId=${j.id}`,
      line: t("Accepted · {type} · visit {slot}", { type: typeLabel(raw.type), slot: raw.visitSlot ? span(raw.visitSlot) : "—" }),
      tech: { name: t("Unassigned"), sub: t("assign before the visit"), tone: "warn" },
      slot: { main: raw.visitSlot ? span(raw.visitSlot) : "—", sub: t("Due {time}", { time: at(raw.dueAt) }) },
      badge: { text: t("Accepted"), tone: "primary" },
    };
  }
  if (raw.projection === "history") {
    const status = historyStatus[raw.status] ? t(historyStatus[raw.status]) : raw.status.replace(/_/g, " ");
    return {
      id: j.id, title, origin: null, href: `/partner/history?jobId=${j.id}`,
      line: t("{status} · delegation ended, history only", { status }),
      tech: { name: "—", sub: t("history only") },
      slot: { main: raw.completedAt ? t("Completed {time}", { time: at(raw.completedAt) }) : "—", sub: t("access closed") },
      badge: raw.status === "completed" ? { text: t("Completed"), tone: "ok" } : { text: status, tone: "muted" },
    };
  }
  const due = j.dueAt ? t("Due {time}", { time: at(j.dueAt) }) : "";
  const known = ack[(raw as { assignmentAcknowledgement?: string | null }).assignmentAcknowledgement ?? ""];
  const technician = tech ? { name: tech, ...(known ? { sub: t(known.sub), ...(known.tone ? { tone: known.tone, subTone: known.tone } : {}) } : { sub: t("assigned") }) } : null;
  switch (b) {
    case "unassigned":
      return { id: j.id, title, origin, href: `/partner/schedule?jobId=${j.id}`, line: t("Accepted · assign a technician for the agreed time"), tech: { name: t("Unassigned"), sub: t("assign before the delegation ends"), tone: "warn" }, slot: { main: due || "—", sub: t("Assign a technician") }, badge: { text: t("Accepted"), tone: "primary" } };
    case "overdue":
      return { id: j.id, title, origin, href: detail, line: t("Work window ended {time} · report not submitted", { time: at(j.slot!.endAt) }), tech: technician ?? none, slot: { main: t("Ended {time}", { time: at(j.slot!.endAt) }), sub: t("Extend or reassign"), tone: "crit" }, badge: { text: t("Overdue"), tone: "crit" } };
    case "review":
      return { id: j.id, title, origin, href: `/partner/jobs/${j.id}/review`, line: tech ? t("Submitted by {name} · awaiting your review", { name: tech }) : t("Submitted · awaiting your review"), tech: tech ? { name: tech, sub: t("report author") } : none, slot: { main: t("Report submitted"), sub: due }, badge: { text: t("Submitted"), tone: "primary" } };
    case "completed":
      return { id: j.id, title, origin, href: detail, line: t("Quality review accepted · report visible to the customer"), tech: tech ? { name: tech, sub: t("report author") } : none, slot: { main: t("Completed"), sub: due }, badge: { text: t("Completed"), tone: "ok" } };
  }
  const slot = j.slot ? { main: span(j.slot), sub: due } : { main: due || "—", sub: "" };
  if (j.status === "in_progress") return { id: j.id, title, origin, href: detail, line: tech ? t("In progress · {name} on site", { name: tech }) : t("In progress"), tech: technician ?? none, slot, badge: { text: t("In progress"), tone: "primary" } };
  if (j.status === "on_hold") return { id: j.id, title, origin, href: detail, line: t("On hold by HQ · saving and submitting are blocked"), tech: technician ?? none, slot, badge: { text: t("On hold"), tone: "warn" } };
  if (j.status === "rework_requested") return { id: j.id, title, origin, href: detail, line: tech ? t("Returned for rework · {name}", { name: tech }) : t("Returned for rework"), tech: technician ?? none, slot, badge: { text: t("Rework"), tone: "warn" } };
  if (j.status === "cancelled") return { id: j.id, title, origin, href: detail, line: t("Cancelled by HQ"), tech: none, slot: { main: "—", sub: "" }, badge: { text: t("Cancelled"), tone: "muted" } };
  const assignedLine = [tech ? t("Assigned {name}", { name: tech }) : t("Assigned"), ...(j.slot ? [span(j.slot)] : [])].join(" · ");
  return { id: j.id, title, origin, href: detail, line: assignedLine, tech: technician ?? none, slot, badge: { text: t("Assigned"), tone: "primary" } };
}

/** "Page 2 of 3" from the total and the page number of the URL. */
export const pages = (total: number, page: number) => ({ page: Math.max(1, page), of: Math.max(1, Math.ceil(total / PAGE_SIZE)) });
