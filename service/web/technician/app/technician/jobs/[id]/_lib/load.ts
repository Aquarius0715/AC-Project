// The reads of the technician job workspace (FR-T04–T06, T08, T09, T13–T15) through the DAL: the job (jobs.get detail
// projection with the assignment, time on site and report refs), the unit while the window allows it (units.get:
// name, place, service scope → required components), the open draft or the latest submitted report (reports.get) and
// the parts catalog for the add-part dialog (parts.list). Photos are streamed by the photos/[attachmentId] route.
// Texts in the user's display language; the instants the page shows are formatted here, in their display time zone,
// and handed to the view (IR282).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { componentsFor, historyCard, knownOf, timeInstants, warnAt, type ApiTechHistory, type ApiTechReport, type TimeOnSite } from "@ac/web/lib/techJob";
import type { WorkspaceLive } from "../_components/workspace-view";
import { recover } from "@ac/web/lib/promises";

interface Refused { refused: string } // FORBIDDEN / NOT_FOUND with the reason (e.g. errors.assignment_not_started)
const quiet = <T,>(p: Promise<T>): Promise<T | Refused> => {
  return recover(p, (error: unknown) => {
    if (error instanceof CoreError && (error.error.code === "FORBIDDEN" || error.error.code === "NOT_FOUND")) return { refused: error.error.messageKey };
    throw error;
  });
};
const isRefused = (x: unknown): x is Refused => x !== null && typeof x === "object" && "refused" in x;

export async function loadWorkspace(jobId: string): Promise<WorkspaceLive | "not_found" | { kind: "history"; card: ReturnType<typeof historyCard> }> {
  /** jobs.get for the technician: the detail while the job is theirs, the snapshot after the window (IR124); an offer
  projection is a contractor's and reads as not found here. */
  type JobRead = ApiTechHistory | {
  projection: "detail"; id: string; version: number; status: string; type: string; unitId: string; symptom: string; alertIds: string[];
  origin: string; dueAt: string | null; timeOnSite: TimeOnSite | null; draftReportRef: { reportId: string; reportVersion: number } | null;
  reportRefs: { reportId: string; reportVersion: number }[]; problemPhotos?: { id: string; name: string; status: string }[];
  assignment: { scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: "pending" | "accepted" | "cant_make"; cantMakeReason: string | null } | null;
  } | { projection: "offer" };
  interface Part { code: string; name: string; vanStockQuantity: number | null }
  const job = await recover(coreOp<JobRead>("jobs.get", { jobId }), (error: unknown) => {
    if (error instanceof CoreError && (error.error.code === "NOT_FOUND" || error.error.code === "FORBIDDEN" || error.error.fieldErrors.jobId)) return null;
    throw error;
  });
  const display = await coreDisplay();
  const i = i18nOf(display);
  if (job?.projection === "history") return { kind: "history", card: historyCard(job, i) }; // window ended: completed (IR234) or reassigned
  if (job?.projection !== "detail") return "not_found";
  const [now, unit] = await Promise.all([coreNow(), quiet(coreOp<ApiUnitDetail & { serviceScope?: string[] }>("units.get", { id: job.unitId }))]);
  // before the work window units.get refuses, but units.list still names the unit of the assignment, as the overview does (IR570)
  const listed = isRefused(unit) ? await quiet(coreOp<{ items: { displayName: string }[] }>("units.list", { limit: 1, filters: { unitIds: [job.unitId] } })) : null;
  const ref = job.draftReportRef ?? job.reportRefs.at(-1) ?? null;
  const isEditable = job.status === "in_progress" && !!job.draftReportRef;
  const [report, parts] = await Promise.all([
    ref ? quiet(coreOp<ApiTechReport>("reports.get", { jobId, reportId: ref.reportId, reportVersion: ref.reportVersion })) : null,
    isEditable || job.status === "in_progress" ? quiet(coreAll<Part>("parts.list")) : [],
  ]);
  const u = isRefused(unit) ? null : unit;
  const r = report === null || isRefused(report) ? null : report;
  const a = job.assignment;
  const known = knownOf(i,
    [...timeInstants(job.timeOnSite), a?.scheduledEnd, a && warnAt(a.scheduledEnd), r?.signOff?.signedAt, ...(r?.measurements ?? []).map((m) => m.observedAt)],
    [a?.scheduledStart, a?.scheduledEnd, ...(r?.reviewHistory ?? []).map((h) => h.occurredAt)],
    a ? [[a.scheduledStart, a.scheduledEnd]] : []);
  return {
    now: now.toISOString(),
    zone: display.timeZone,
    job: {
      id: job.id, version: job.version, status: job.status, type: job.type, unitId: job.unitId, symptom: job.symptom, alertCount: job.alertIds.length, origin: job.origin,
      assignment: job.assignment, timeOnSite: job.timeOnSite, draftReportRef: job.draftReportRef, reportRefs: job.reportRefs,
      problemPhotos: (job.problemPhotos ?? []).filter((p) => p.status === "ready").map((p) => ({ id: p.id, name: p.name })), // IR437
    },
    unit: u ? { name: u.displayName, place: u.location.pathLabels.join(" › "), access: u.location.accessInstructions, model: `${u.capabilities.manufacturer} ${u.capabilities.model}`, scope: u.serviceScope } : null,
    unitRefused: isRefused(unit) ? unit.refused : null,
    unitName: u?.displayName ?? (listed && !isRefused(listed) ? listed.items[0]?.displayName ?? null : null),
    components: componentsFor(u?.serviceScope ?? ["indoor", "outdoor", "electrical"]),
    report: r,
    reportRefused: isRefused(report) ? report.refused : null,
    parts: isRefused(parts) ? [] : parts,
    known,
  };
}
