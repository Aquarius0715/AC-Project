// The contractor job list (FR-P01, DD-P01 job list, Figma Contractor 02-1…02-6) from the Core API: status tabs with
// their counts, the sort (IR34) and the period of the overview, and one row per job with the technician, the visit
// slot or deadline and the status badge; rows open the offer, the job detail, the schedule or the quality review.
// Pure code shared by the server loader and the client view; Vitest covers it. Rows in the display language with the
// slots and deadlines in the user's display time zone (IR271).
import { EN, showSpan, showTime, type I18n, textOf } from "@ac/web/lib/i18n";
import { bucket, firstSlot, partnerJob, until, type ApiPartnerJob, type Slot } from "@ac/web/lib/partnerOverview";
import type { OpInput } from "@ac/web/lib/opTypes";

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
export function tabFilters(tab: TabId, fromAt: string, toAt: string): NonNullable<OpInput<"jobs.list">["filters"]> {
  const statuses = (TABS.find((t) => t.id === tab) ?? TABS[0]).statuses;
  return { from: fromAt, to: toAt, ...(statuses && { statuses: [...statuses] }) };
}

export const SORTS = [
  { id: "status:asc", field: "status", direction: "asc", label: "Status, earliest stage first", text: "status, then deadline" }, { id: "status:desc", field: "status", direction: "desc", label: "Status, latest stage first", text: "status (last first)" },
  { id: "dueAt:asc", field: "dueAt", direction: "asc", label: "Deadline, soonest first", text: "deadline (soonest first)" }, { id: "dueAt:desc", field: "dueAt", direction: "desc", label: "Deadline, latest first", text: "deadline (latest first)" },
  { id: "severity:desc", field: "severity", direction: "desc", label: "Unit urgency, most urgent first", text: "unit urgency" },
] as const;
export type SortId = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): SortId => (SORTS.find((s) => s.id === v)?.id ?? "status:asc");
type SortSpec = NonNullable<OpInput<"jobs.list">["sort"]>;
const SORT_SPECS = new Map(SORTS.map((sort): [SortId, SortSpec] => [sort.id, { field: sort.field, direction: sort.direction }]));
export const sortSpec = (id: SortId): SortSpec => SORT_SPECS.get(id) ?? { field: "status", direction: "asc" };

export const PAGE_SIZE = 25;

type Tone = "primary" | "ok" | "warn" | "crit" | "muted";
export interface JobRow {
  id: string; title: string; origin: string | null; line: string; href: string;
  /** `subTone` colours the line under the name (a technician who can't make the slot); "Unassigned" colours only the name. */
  tech: { name: string; sub: string; tone?: Tone; subTone?: Tone }; slot: { main: string; sub: string; tone?: Tone }; badge: { text: string; tone: Tone };
}

const typeText: Record<string, string> = { periodic: "Periodic inspection", reactive: "Repair", preventive: "Preventive maintenance" };
const ack: Record<string, { sub: string; tone?: Tone }> = { pending: { sub: "awaiting the technician’s acceptance" }, accepted: { sub: "accepted" }, cant_make: { sub: "can’t make it — reassign", tone: "warn" } };
const historyStatus: Record<string, string> = { completed: "Completed", cancelled: "Cancelled" };

/** An offer's candidate times (IR636) on one line: the only time, or the earliest and how many more the contractor may pick. */
function candidatesText(slots: readonly Slot[], span: (s: Slot) => string, t: I18n["t"]): string {
  const first = firstSlot(slots);
  if (!first) return "—";
  return slots.length > 1 ? t("{slot} · +{n} more to choose from", { slot: span(first), n: slots.length - 1 }) : span(first);
}

/** One list row: what the company sees of a job in its projection (offer, summary inside the window, history after). */
export function jobRow(raw: ApiPartnerJob, now: number, units: Map<string, string>, names: Map<string, string>, i: I18n = EN): JobRow {
  const { t, display } = i;
  const j = partnerJob(raw, units);
  const typeLabel = (type: string) => (textOf(typeText, type, t) ?? type);
  const span = (x: Slot) => showSpan(x.startAt, x.endAt, display);
  const at = (iso: string) => showTime(iso, display);
  const place = j.unit ?? j.address ?? typeLabel(j.type);
  const title = `${j.short} · ${place}`;
  const tech = j.technicianId ? names.get(j.technicianId) ?? t("Technician") : null;
  const origin = "origin" in raw ? raw.origin ?? null : null; // offers and summaries carry it, histories do not
  const b = bucket(j, now);
  const detail = `/partner/jobs/${j.id}`;
  if (raw.projection === "offer" && raw.status === "offered") {
    const proposal = raw.partnerSlotProposal;
    const pending = proposal && (proposal.status === "pending" || proposal.status === "sent_to_client");
    const left = until(raw.offerExpiresAt, now, t);
    return {
      id: j.id,
      href: detail,
      line: pending ? t("Time change proposed · waiting for HQ and the client") : t("Offered by HQ · {times}", { times: candidatesText(raw.candidateSlots, span, t) }),
      tech: { name: "—", sub: t("not assignable before acceptance") },
      slot: { main: t("Answer by {time}", { time: at(raw.offerExpiresAt) }), sub: Date.parse(raw.offerExpiresAt) <= now ? left : t("{left} left", { left }), tone: Date.parse(raw.offerExpiresAt) - now < 6 * 3_600_000 ? "warn" : undefined },
      badge: pending ? { text: t("Time proposed"), tone: "warn" } : { text: t("Offered"), tone: "primary" },
      title,
      origin,
    };
  }
  if (raw.projection === "offer") { // accepted before the access window opens
    return {
      id: j.id,
      href: `/partner/schedule?jobId=${j.id}`,
      line: t("Accepted · {type} · {times}", { type: typeLabel(raw.type), times: candidatesText(raw.candidateSlots, span, t) }),
      tech: { name: t("Unassigned"), sub: t("assign before the visit"), tone: "warn" },
      slot: { main: candidatesText(raw.candidateSlots, span, t), sub: t("Due {time}", { time: at(raw.dueAt) }) },
      badge: { text: t("Accepted"), tone: "primary" },
      title,
      origin,
    };
  }
  if (raw.projection === "history") {
    const status = textOf(historyStatus, raw.status, t) ?? raw.status.replaceAll('_', " ");
    return {
      id: j.id,
      origin: null,
      href: `/partner/history?jobId=${j.id}`,
      line: t("{status} · delegation ended, history only", { status }),
      tech: { name: "—", sub: t("history only") },
      slot: { main: raw.completedAt ? t("Completed {time}", { time: at(raw.completedAt) }) : "—", sub: t("access closed") },
      badge: raw.status === "completed" ? { text: t("Completed"), tone: "ok" } : { text: status, tone: "muted" },
      title,
    };
  }
  const none = { name: "—", sub: "" };
  const due = j.dueAt ? t("Due {time}", { time: at(j.dueAt) }) : "";
  const known = ack[raw.assignmentAcknowledgement ?? ""]; // a summary: offers and histories returned above
  const technician = tech ? { name: tech, ...(known ? { sub: t(known.sub), ...(known.tone && { tone: known.tone, subTone: known.tone }) } : { sub: t("assigned") }) } : null;
  switch (b) {
    case "unassigned": {
      return { id: j.id, href: `/partner/schedule?jobId=${j.id}`, line: t("Accepted · assign a technician for the agreed time"), tech: { name: t("Unassigned"), sub: t("assign before the delegation ends"), tone: "warn" }, slot: { main: due || "—", sub: t("Assign a technician") }, badge: { text: t("Accepted"), tone: "primary" }, title, origin };
    }
    case "overdue": {
      const ended = j.slot ? at(j.slot.endAt) : "—";
      return { id: j.id, href: detail, line: t("Work window ended {time} · report not submitted", { time: ended }), tech: technician ?? none, slot: { main: t("Ended {time}", { time: ended }), sub: t(raw.status === "assigned" ? "Ask HQ for a new time" : "Extend or reassign"), tone: "crit" }, badge: { text: t("Overdue"), tone: "crit" }, title, origin };
    }
    case "review": {
      return { id: j.id, href: `/partner/jobs/${j.id}/review`, line: tech ? t("Submitted by {name} · awaiting your review", { name: tech }) : t("Submitted · awaiting your review"), tech: tech ? { name: tech, sub: t("report author") } : none, slot: { main: t("Report submitted"), sub: due }, badge: { text: t("Submitted"), tone: "primary" }, title, origin };
    }
    case "completed": {
      return { id: j.id, href: detail, line: t("Quality review accepted · report visible to the customer"), tech: tech ? { name: tech, sub: t("report author") } : none, slot: { main: t("Completed"), sub: due }, badge: { text: t("Completed"), tone: "ok" }, title, origin };
    }
    default: {
      break; // the other kinds are worded below, by the job's status
    }
  }
  const slot = j.slot ? { main: span(j.slot), sub: due } : { main: due || "—", sub: "" };
  if (j.status === "in_progress") return { id: j.id, href: detail, line: tech ? t("In progress · {name} on site", { name: tech }) : t("In progress"), tech: technician ?? none, badge: { text: t("In progress"), tone: "primary" }, title, origin, slot };
  if (j.status === "on_hold") return { id: j.id, href: detail, line: t("On hold by HQ · saving and submitting are blocked"), tech: technician ?? none, badge: { text: t("On hold"), tone: "warn" }, title, origin, slot };
  if (j.status === "rework_requested") return { id: j.id, href: detail, line: tech ? t("Returned for rework · {name}", { name: tech }) : t("Returned for rework"), tech: technician ?? none, badge: { text: t("Rework"), tone: "warn" }, title, origin, slot };
  if (j.status === "cancelled") return { id: j.id, href: detail, line: t("Cancelled by HQ"), tech: none, slot: { main: "—", sub: "" }, badge: { text: t("Cancelled"), tone: "muted" }, title, origin };
  const assignedLine = [tech ? t("Assigned {name}", { name: tech }) : t("Assigned"), ...(j.slot ? [span(j.slot)] : [])].join(" · ");
  return { id: j.id, href: detail, line: assignedLine, tech: technician ?? none, badge: { text: t("Assigned"), tone: "primary" }, title, origin, slot };
}

/** "Page 2 of 3" from the total and the page number of the URL. */
export const pages = (total: number, page: number) => ({ page: Math.max(1, page), of: Math.max(1, Math.ceil(total / PAGE_SIZE)) });
