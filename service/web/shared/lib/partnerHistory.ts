// The contractor's job history (FR-P07, FR-P08, DD-P07, Figma Contractor 05-1…05-6) from the Core API: the company's
// jobs after acceptance with their events (tabs, latest event, what the customer can see), the timeline of one job,
// and the communication form — save a note and render a notification preview for a recipient of the job (nothing is
// sent, deliveryState stays preview). Pure code shared by the server loader and the client view; Vitest covers it.
import { klTime } from "@ac/web/lib/devices";
import { jobEventTitle, type ApiJobEvent, type ApiMember, type ApiPartnerJob } from "@ac/web/lib/partnerOverview";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";

/** The states a job has after the company accepted it; offers appear here only after acceptance. */
export const HISTORY_STATUSES = ["accepted", "assigned", "in_progress", "on_hold", "rework_requested", "submitted", "completed", "cancelled"] as const;
export const TABS = [
  { id: "all", label: "All" }, { id: "active", label: "Active" }, { id: "review", label: "Review" }, { id: "completed", label: "Completed" }, { id: "ended", label: "Delegation ended" },
] as const;
export type HistoryTab = (typeof TABS)[number]["id"];
export const tabOf = (v?: string): HistoryTab => (TABS.some((t) => t.id === v) ? (v as HistoryTab) : "all");
export const SORTS = [{ id: "latest", text: "Latest event ↓" }, { id: "status", text: "Status ↑" }] as const;
export type HistorySort = (typeof SORTS)[number]["id"];
export const sortOf = (v?: string): HistorySort => (SORTS.some((s) => s.id === v) ? (v as HistorySort) : "latest");
export const PERIODS = [{ id: "90d", label: "Last 90 days" }, { id: "all", label: "All time" }] as const;
export type HistoryPeriod = (typeof PERIODS)[number]["id"];
export const periodOf = (v?: string): HistoryPeriod => (v === "all" ? "all" : "90d");

export type ApiEvent = ApiJobEvent & { note: { visibility: "internal" | "customer"; message: string } | null };
type Tone = "ok" | "warn" | "crit" | "primary" | "muted";
/** The customer reads every event except internal notes (jobs.events for clients). */
export const customerVisible = (e: ApiEvent) => !(e.note && e.note.visibility === "internal");
const mmddhm = (iso: string) => klTime(iso).slice(5);
const ACTIVE = ["accepted", "assigned", "in_progress", "on_hold", "rework_requested"];
const statusText = (s: string) => s.charAt(0).toUpperCase() + s.slice(1).replace(/_/g, " ");
const STATUS_TONE: Record<string, Tone> = { accepted: "primary", assigned: "primary", in_progress: "primary", on_hold: "warn", rework_requested: "warn", submitted: "primary", completed: "ok", cancelled: "muted" };

export type HistoryRow = {
  id: string; short: string; title: string; origin: string | null; version: number | null; ended: boolean; status: string; tab: Exclude<HistoryTab, "all"> | null;
  sub: string; tech: { name: string; sub: string; tone?: Tone }; last: { title: string; at: string } | null; lastAt: number;
  badge: { text: string; tone: Tone }; delegation: string; unit: string | null;
};
/** One job of the list with its events (newest first): counts, the latest event, the technician and the badge. */
export function historyRow(j: ApiPartnerJob, events: ApiEvent[], now: number, units: Map<string, string>, names: Map<string, string>): HistoryRow | null {
  if (j.projection === "offer") return null; // offers are answered in Jobs; history starts at acceptance
  const latest = [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt) || b.id.localeCompare(a.id))[0];
  const notes = events.filter((e) => e.note).length;
  const visible = events.filter(customerVisible).length;
  const last = latest ? { title: jobEventTitle[latest.action] ?? latest.action.replace(/[._]/g, " "), at: latest.occurredAt } : null;
  if (j.projection === "history") {
    return {
      id: j.jobId, short: j.jobId.slice(0, 8), title: typeLabel(j.type), origin: null, version: null, ended: true, status: j.status, tab: "ended", unit: null,
      sub: `${events.length} own decision ${events.length === 1 ? "event" : "events"} · delegation ended (read-only snapshot)`, delegation: "ended — history only",
      tech: { name: "—", sub: "not shown after the delegation" }, last, lastAt: last ? Date.parse(last.at) : 0, badge: { text: statusText(j.status), tone: STATUS_TONE[j.status] ?? "muted" },
    };
  }
  const tech = j.technicianMembershipId ? names.get(j.technicianMembershipId) ?? "technician" : null;
  const ack = j.assignmentAcknowledgement === "accepted" ? "accepted the slot ✓" : j.assignmentAcknowledgement === "cant_make" ? "can’t make it ⚠" : j.assignmentAcknowledgement === "pending" ? "awaiting acceptance" : "";
  const overdue = (j.status === "assigned" || j.status === "in_progress") && !!j.scheduledSlot && Date.parse(j.scheduledSlot.endAt) <= now;
  const until = j.accessValidUntil ?? null;
  const delegation = until ? (Date.parse(until) <= now ? `ended ${mmddhm(until)}` : `until ${mmddhm(until)}`) : "—";
  const tab = ACTIVE.includes(j.status) ? "active" : j.status === "submitted" ? "review" : j.status === "completed" ? "completed" : null;
  return {
    id: j.id, short: j.id.slice(0, 8), title: units.get(j.unitId) ?? typeLabel(j.type), origin: j.origin ?? null, version: j.version, ended: false, status: j.status, tab, unit: units.get(j.unitId) ?? null,
    sub: `${events.length} ${events.length === 1 ? "event" : "events"} · ${visible} customer-visible · ${notes} ${notes === 1 ? "note" : "notes"} · delegation ${delegation}`, delegation,
    tech: tech ? { name: tech, sub: ack || "assigned" } : { name: "Unassigned", sub: j.status === "accepted" ? "assign in Schedule" : "—", tone: j.status === "accepted" ? "crit" : undefined },
    last, lastAt: last ? Date.parse(last.at) : 0,
    badge: overdue ? { text: "Overdue", tone: "crit" } : { text: statusText(j.status), tone: STATUS_TONE[j.status] ?? "muted" },
  };
}

/** The rows of a tab and their order (latest event first, or by status with the newest event first inside). */
export function visibleRows(rows: HistoryRow[], tab: HistoryTab, sort: HistorySort): HistoryRow[] {
  const order = [...HISTORY_STATUSES] as string[];
  const shown = rows.filter((r) => tab === "all" || r.tab === tab);
  return shown.sort((a, b) => (sort === "status" ? order.indexOf(a.status) - order.indexOf(b.status) : 0) || b.lastAt - a.lastAt || a.id.localeCompare(b.id));
}
export const tabCounts = (rows: HistoryRow[]) => Object.fromEntries(TABS.map((t) => [t.id, t.id === "all" ? rows.length : rows.filter((r) => r.tab === t.id).length])) as Record<HistoryTab, number>;

export type TimelineItem = { id: string; title: string; detail: string; at: string; badge: "customer" | "internal"; tone?: "ok" | "warn" };
/** One timeline row: the event's title, its note (in quotes, with the visibility) and who acted — “you”, a member of the
 * company or the system; people outside the company (HQ, the customer) are not named — and who can see it. */
export function eventItem(e: ApiEvent, byUser: Map<string, string>): TimelineItem {
  const title = jobEventTitle[e.action] ?? e.action.replace(/[._]/g, " ");
  const actor = e.actorUserId ? byUser.get(e.actorUserId) ?? null : "system";
  const detail = [e.note ? `“${e.note.message}” · ${e.note.visibility}` : "", actor ? `by ${actor}` : ""].filter(Boolean).join(" · ");
  const tone = /reviewed|accepted|completed/.test(e.action) ? "ok" : /rework|cant_make|held|cancel|problem/.test(e.action) ? "warn" : undefined;
  return { id: e.id, title, detail, at: klTime(e.occurredAt), badge: customerVisible(e) ? "customer" : "internal", tone };
}
export const byUser = (members: ApiMember[], me?: string) => new Map([...members.map((m) => [m.userId, m.displayName] as const), ...(me ? [[me, "you"] as const] : [])]);
/** “an in-app”, “an email”, “a WhatsApp” — the channel of a preview in a sentence. */
export const channelText = (c: string) => ({ inApp: "an in-app", email: "an email", whatsapp: "a WhatsApp" })[c] ?? `a ${c}`;

/** The templates and recipient roles of a job communication (DD-P07; roles map hq→admin, assigned technician→technician,
 * customer contact→client, IR90). */
export const TEMPLATES = [{ id: "schedule_change", label: "schedule_change" }, { id: "report_return", label: "report_return" }, { id: "completion", label: "completion" }] as const;
export const ROLES = [{ id: "admin", label: "hq", sub: "HQ of this job" }, { id: "technician", label: "assigned_technician", sub: "the job’s technician" }, { id: "client", label: "customer_contact", sub: "the customer’s contact" }] as const;
export const CHANNELS = [{ id: "inApp", label: "inApp · preview only" }, { id: "email", label: "email · preview only" }, { id: "whatsapp", label: "whatsapp · preview only" }] as const;

export type NoteMode = { kind: "note" | "preview" | "closed"; button: string; text?: string };
/** What the form may do: save a note and create the preview on an open job; only the preview once the job is completed
 * or cancelled (notes are refused then, IR123); nothing after the delegation (FR-P08). */
export function noteMode(row: Pick<HistoryRow, "ended" | "status">): NoteMode {
  if (row.ended) return { kind: "closed", button: "Create preview", text: "The delegation has ended — the job is a read-only snapshot (FR-P08)." };
  if (row.status === "completed" || row.status === "cancelled") return { kind: "preview", button: "+ Create preview", text: `Notes are closed on ${row.status} jobs — only the preview is created.` };
  return { kind: "note", button: "Save note & create preview" };
}

/** Readable refusals of the note and the preview. */
export function communicationRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  switch (f.messageKey) {
    case "error.invalidState": return "Notes are closed for this job’s state — nothing was saved.";
    case "error.versionConflict": return "The job changed meanwhile — the history is reloaded; send again.";
  }
  if (f.code === "NOT_FOUND") return "The job is no longer readable by your company.";
  if (f.code === "VALIDATION") {
    const text: Record<string, string> = { "error.length": "1–2000 characters", "error.required": "required", "error.invalid": "not allowed", "errors.recipient_ineligible": "not a recipient of this job — choose one from the list" };
    return Object.entries(f.fieldErrors).map(([k, v]) => `${k}: ${text[v] ?? v}`).join(" · ") || "Check the input.";
  }
  return `${f.code} — ${f.messageKey} (your text is kept; retry)`;
}
