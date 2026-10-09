// The reads of /customer/maintenance (FR-C09, FR-C17, FR-C18, SCR-C09) through the DAL: the customer's units with their
// place (units.list, properties.list, spaces.list); on My requests the requests (jobs.list, client projection) and for
// jobId the request (jobs.get — the technician's name once accepted, IR237), its events (jobs.events: notes and history,
// internal notes never reach the client) and its accepted report (reports.get with up to four ready photos,
// attachments.getContent); on Filter care (tab=filter-care) the status per AC (filterCare.list) and the reminder
// settings or their defaults (filterCare.getSettings, IR238).
import "server-only";
import { coreAll, coreNow, coreOp, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { unitPlaces } from "@ac/web/lib/clientBilling";
import { jobEventTitle } from "@ac/web/lib/partnerOverview";
import { inspectionRows, readingRows, resultText, type ApiWorkReport } from "@ac/web/lib/partnerReview";
import {
  clientEventTitle, clientRows, declinedNotice, detailFacts, feedback, notesOf, planVisit, preferredLines, proposalCard, type ApiClientEvent, type ApiClientJob, type ApiClientRow,
} from "@ac/web/lib/customerMaintenance";
import { filterLines, reminderForm, reminderLines, type ApiFilterSettings, type ApiFilterStatus } from "@ac/web/lib/customerFilterCare";
import { klTime } from "@ac/web/lib/devices";

type Unit = { id: string; displayName: string; propertyId: string; spaceId: string | null; archived: boolean; connection: string };
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
const quiet = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN")) return fallback;
  throw e;
});

export async function loadMaintenance(sp: Record<string, string | string[] | undefined>) {
  const page = one(sp.tab) === "filter-care" || one(sp.tab) === "filters" ? "filters" as const : "requests" as const;
  const [now, me, units, properties, spaces] = await Promise.all([
    coreNow(), corePrincipal(), coreAll<Unit>("units.list"), coreAll<{ id: string; name: string }>("properties.list"),
    coreAll<{ id: string; name: string; parentSpaceId: string | null }>("spaces.list"),
  ]);
  const nowMs = now.getTime();
  const place = unitPlaces(units, properties, spaces);
  const unitOf = (id: string) => place.get(id) ?? { name: "Unit", place: "" };
  const active = units.filter((u) => !u.archived);
  // new=<unitId>: open New request for that unit (an alert's “Request repair”, Figma Client 06g)
  const newFor = active.some((u) => u.id === one(sp.new)) ? one(sp.new)! : null;
  const common = { now: now.toISOString(), page, owner: me.clientRole === "owner", newFor, units: active.map((u) => ({ id: u.id, ...unitOf(u.id) })) };
  if (page === "filters") {
    const [items, settings] = await Promise.all([coreAll<ApiFilterStatus>("filterCare.list"), coreOp<ApiFilterSettings>("filterCare.getSettings", {})]);
    const lines = filterLines(items, units.map((u) => ({ id: u.id, ...unitOf(u.id), spaceId: u.spaceId, connection: u.connection })), nowMs);
    return { ...common, rows: [], rateBanner: null, detail: null, filters: { lines, settings, reminders: reminderLines(settings), form: reminderForm(settings) } };
  }
  const list = await coreAll<ApiClientRow>("jobs.list", { sort: { field: "status", direction: "asc" } });
  const rows = clientRows(list.filter((j) => j.projection === "summary"), unitOf);
  // the newest completed requests: the first one still waiting for Confirm & rate gets the banner (IR110, DD-C17)
  const done = await Promise.all(list.filter((j) => j.projection === "summary" && j.status === "completed")
    .sort((a, b) => Date.parse(b.scheduledSlot?.startAt ?? b.dueAt) - Date.parse(a.scheduledSlot?.startAt ?? a.dueAt)).slice(0, 3)
    .map((j) => quiet(coreOp<ApiClientJob>("jobs.get", { jobId: j.id }), null)));
  const toRate = done.find((j) => j && j.projection === "detail" && feedback(j, nowMs).kind === "rate") ?? null;
  const base = {
    ...common, rows, filters: null,
    rateBanner: toRate ? { jobId: toRate.id, text: `Work finished — ${toRate.id.slice(0, 8)} · ${unitOf(toRate.unitId).name}${toRate.completedAt ? ` · ${klTime(toRate.completedAt).slice(5, 10)}` : ""}.` } : null,
  };
  const pick = one(sp.jobId);
  if (!pick) return { ...base, detail: null };
  const job = await quiet(coreOp<ApiClientJob>("jobs.get", { jobId: pick }), null);
  if (!job || job.projection !== "detail") return { ...base, detail: null, missing: pick };
  const ref = job.reportRefs[job.reportRefs.length - 1];
  const [events, report] = await Promise.all([
    quiet(coreOp<{ items: ApiClientEvent[] }>("jobs.events", { jobId: job.id, query: { limit: 100 } }).then((r) => r.items), [] as ApiClientEvent[]),
    ref ? quiet(coreOp<ApiWorkReport>("reports.get", { jobId: job.id, reportId: ref.reportId, reportVersion: ref.reportVersion }), null) : Promise.resolve(null),
  ]);
  const photos = report ? (await Promise.all(report.attachmentRefs.filter((a) => a.status === "ready" && a.size <= 2_000_000).slice(0, 4).map(async (a) => {
    const b = await quiet(coreOp<{ mime: string; bytes: string }>("attachments.getContent", { jobId: job.id, reportId: report.id, reportVersion: report.version, attachmentId: a.id }), null);
    return b ? { id: a.id, name: a.name, url: `data:${b.mime};base64,${b.bytes}` } : null;
  }))).filter((p): p is { id: string; name: string; url: string } => !!p) : [];
  const u = unitOf(job.unitId);
  return {
    ...base,
    detail: {
      job, unit: u, facts: detailFacts(job, u), preferred: preferredLines(job), proposal: proposalCard(job, nowMs), declined: declinedNotice(job), plan: planVisit(job, nowMs),
      feedback: feedback(job, nowMs), notes: notesOf(events, me.userId),
      history: [...events].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt)).slice(0, 8).map((e) => ({ id: e.id, time: klTime(e.occurredAt).slice(5), title: e.note ? (e.actorUserId === me.userId ? "You added a note" : "Note from the coordinator") : clientEventTitle(e.action, jobEventTitle) })),
      report: report ? {
        version: report.version, acceptedAt: report.acceptedAt, items: inspectionRows(report.items).map((r) => ({ id: r.id, label: r.label, result: r.result ? resultText[r.result].label : "—", tone: r.result ? resultText[r.result].tone : "muted", reason: r.reason })),
        readings: readingRows(report.measurements), parts: report.parts.map((p) => `${p.name} × ${p.quantity}`), workText: report.workText, photos,
      } : null,
    },
  };
}

export type MaintenanceLive = Awaited<ReturnType<typeof loadMaintenance>>;
