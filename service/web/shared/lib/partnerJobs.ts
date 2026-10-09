// The contractor job list (FR-P01, DD-P01 job list, Figma Contractor 02-1…02-6) from the Core API: status tabs with
// their counts, the sort (IR34) and the period of the overview, and one row per job with the technician, the visit
// slot or deadline and the status badge; rows open the offer, the job detail, the schedule or the quality review.
// Pure code shared by the server loader and the client view; Vitest covers it.
import { klTime } from "@ac/web/lib/devices";
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
  tech: { name: string; sub: string; tone?: Tone }; slot: { main: string; sub: string; tone?: Tone }; badge: { text: string; tone: Tone };
};

const mmddhm = (iso: string) => klTime(iso).slice(5);
export const slotText = (s: Slot) => (klTime(s.startAt).slice(0, 10) === klTime(s.endAt).slice(0, 10) ? `${mmddhm(s.startAt)}–${hhmm(s.endAt)}` : `${mmddhm(s.startAt)} → ${mmddhm(s.endAt)}`);
const typeLabel = (t: string) => ({ periodic: "Periodic inspection", reactive: "Repair", preventive: "Preventive maintenance" })[t] ?? t;
const ack: Record<string, { sub: string; tone?: Tone }> = { pending: { sub: "awaiting the technician’s acceptance" }, accepted: { sub: "accepted ✓" }, cant_make: { sub: "can’t make it ⚠ — reassign", tone: "warn" } };

/** One list row: what the company sees of a job in its projection (offer, summary inside the window, history after). */
export function jobRow(raw: ApiPartnerJob, now: number, units: Map<string, string>, names: Map<string, string>): JobRow {
  const j = partnerJob(raw, units);
  const place = j.unit ?? j.address ?? typeLabel(j.type);
  const title = `${j.short} · ${place}`;
  const tech = j.technicianId ? names.get(j.technicianId) ?? "Technician" : null;
  const origin = "origin" in raw ? (raw as { origin?: string }).origin ?? null : null;
  const b = bucket(j, now);
  const detail = `/partner/jobs/${j.id}`;
  if (raw.projection === "offer" && raw.status === "offered") {
    const proposal = (raw as { partnerSlotProposal?: { status: string } | null }).partnerSlotProposal;
    const pending = proposal && (proposal.status === "pending" || proposal.status === "sent_to_client");
    return {
      id: j.id, title, origin, href: detail,
      line: pending ? "Time change proposed · waiting for HQ and the client" : `Offered by HQ · visit ${raw.visitSlot ? slotText(raw.visitSlot) : "—"} (fixed)`,
      tech: { name: "—", sub: "not assignable before acceptance" },
      slot: { main: `Answer by ${mmddhm(raw.offerExpiresAt)}`, sub: `${until(raw.offerExpiresAt, now)}${until(raw.offerExpiresAt, now) === "ended" ? "" : " left"}`, tone: Date.parse(raw.offerExpiresAt) - now < 6 * 3600_000 ? "warn" : undefined },
      badge: pending ? { text: "Time proposed", tone: "warn" } : { text: "Offered", tone: "primary" },
    };
  }
  if (raw.projection === "offer") { // accepted before the access window opens
    return {
      id: j.id, title, origin, href: `/partner/schedule?jobId=${j.id}`,
      line: `Accepted · ${typeLabel(raw.type)} · visit ${raw.visitSlot ? slotText(raw.visitSlot) : "—"}`,
      tech: { name: "Unassigned", sub: "assign before the visit", tone: "warn" },
      slot: { main: raw.visitSlot ? slotText(raw.visitSlot) : "—", sub: `Due ${mmddhm(raw.dueAt)}` },
      badge: { text: "Accepted", tone: "primary" },
    };
  }
  if (raw.projection === "history") {
    return {
      id: j.id, title, origin: null, href: `/partner/history?jobId=${j.id}`,
      line: `${raw.status === "completed" ? "Completed" : raw.status.replace(/_/g, " ")} · delegation ended, history only`,
      tech: { name: "—", sub: "history only" },
      slot: { main: raw.completedAt ? `Completed ${mmddhm(raw.completedAt)}` : "—", sub: "access closed" },
      badge: raw.status === "completed" ? { text: "Completed", tone: "ok" } : { text: raw.status.replace(/_/g, " "), tone: "muted" },
    };
  }
  const due = j.dueAt ? `Due ${mmddhm(j.dueAt)}` : "";
  const technician = tech ? { name: tech, ...(ack[(raw as { assignmentAcknowledgement?: string | null }).assignmentAcknowledgement ?? ""] ?? { sub: "assigned" }) } : null;
  switch (b) {
    case "unassigned":
      return { id: j.id, title, origin, href: `/partner/schedule?jobId=${j.id}`, line: `Accepted · assign a technician for the agreed time`, tech: { name: "Unassigned", sub: "assign before the delegation ends", tone: "warn" }, slot: { main: due || "—", sub: "Assign a technician" }, badge: { text: "Accepted", tone: "primary" } };
    case "overdue":
      return { id: j.id, title, origin, href: detail, line: `Work window ended ${mmddhm(j.slot!.endAt)} · report not submitted`, tech: technician ?? { name: "—", sub: "" }, slot: { main: `Ended ${mmddhm(j.slot!.endAt)}`, sub: "Extend or reassign", tone: "crit" }, badge: { text: "Overdue", tone: "crit" } };
    case "review":
      return { id: j.id, title, origin, href: `/partner/jobs/${j.id}/review`, line: `Submitted${tech ? ` by ${tech}` : ""} · awaiting your review`, tech: tech ? { name: tech, sub: "report author" } : { name: "—", sub: "" }, slot: { main: "Report submitted", sub: due }, badge: { text: "Submitted", tone: "primary" } };
    case "completed":
      return { id: j.id, title, origin, href: detail, line: "Quality review accepted · report visible to the customer", tech: tech ? { name: tech, sub: "report author" } : { name: "—", sub: "" }, slot: { main: "Completed", sub: due }, badge: { text: "Completed", tone: "ok" } };
  }
  const slot = j.slot ? { main: slotText(j.slot), sub: due } : { main: due || "—", sub: "" };
  if (j.status === "in_progress") return { id: j.id, title, origin, href: detail, line: `In progress${tech ? ` · ${tech} on site` : ""}`, tech: technician ?? { name: "—", sub: "" }, slot, badge: { text: "In progress", tone: "primary" } };
  if (j.status === "on_hold") return { id: j.id, title, origin, href: detail, line: "On hold by HQ · saving and submitting are blocked", tech: technician ?? { name: "—", sub: "" }, slot, badge: { text: "On hold", tone: "warn" } };
  if (j.status === "rework_requested") return { id: j.id, title, origin, href: detail, line: `Returned for rework${tech ? ` · ${tech}` : ""}`, tech: technician ?? { name: "—", sub: "" }, slot, badge: { text: "Rework", tone: "warn" } };
  if (j.status === "cancelled") return { id: j.id, title, origin, href: detail, line: "Cancelled by HQ", tech: { name: "—", sub: "" }, slot: { main: "—", sub: "" }, badge: { text: "Cancelled", tone: "muted" } };
  return { id: j.id, title, origin, href: detail, line: `Assigned${tech ? ` ${tech}` : ""}${j.slot ? ` · ${slotText(j.slot)}` : ""}`, tech: technician ?? { name: "—", sub: "" }, slot, badge: { text: "Assigned", tone: "primary" } };
}

/** "Page 2 of 3" from the total and the page number of the URL. */
export const pages = (total: number, page: number) => ({ page: Math.max(1, page), of: Math.max(1, Math.ceil(total / PAGE_SIZE)) });
