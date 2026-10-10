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
import { componentsFor, historyCard, knownOf, timeInstants, type ApiTechHistory, type ApiTechReport, type TimeOnSite } from "@ac/web/lib/techJob";
import type { WorkspaceLive } from "../_components/workspace-view";

/** jobs.get for the technician: the detail while the job is theirs, the snapshot after the window (IR124); an offer
 * projection is a contractor's and reads as not found here. */
type JobRead = {
  projection: "detail"; id: string; version: number; status: string; type: string; unitId: string; symptom: string; alertIds: string[];
  origin: string; dueAt: string | null; timeOnSite: TimeOnSite | null; draftReportRef: { reportId: string; reportVersion: number } | null;
  reportRefs: { reportId: string; reportVersion: number }[];
  assignment: { scheduledStart: string; scheduledEnd: string; status: string; acknowledgement: "pending" | "accepted" | "cant_make"; cantMakeReason: string | null } | null;
} | ApiTechHistory | { projection: "offer" };
type Part = { code: string; name: string; vanStockQuantity: number | null };
type Refused = { refused: string }; // FORBIDDEN / NOT_FOUND with the reason (e.g. errors.assignment_not_started)
const quiet = <T,>(p: Promise<T>): Promise<T | Refused> => p.catch((e) => {
  if (e instanceof CoreError && (e.error.code === "FORBIDDEN" || e.error.code === "NOT_FOUND")) return { refused: e.error.messageKey };
  throw e;
});
const isRefused = (x: unknown): x is Refused => !!x && typeof x === "object" && "refused" in x;

export async function loadWorkspace(jobId: string): Promise<WorkspaceLive | { kind: "history"; card: ReturnType<typeof historyCard> } | "not_found"> {
  const job = await coreOp<JobRead>("jobs.get", { jobId }).catch((e) => {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || e.error.fieldErrors.jobId)) return null;
    throw e;
  });
  const display = await coreDisplay();
  const i = i18nOf(display);
  if (job?.projection === "history") return { kind: "history", card: historyCard(job, i) }; // window ended: completed (IR234) or reassigned
  if (!job || job.projection !== "detail") return "not_found";
  const [now, unit] = await Promise.all([coreNow(), quiet(coreOp<ApiUnitDetail & { serviceScope?: string[] }>("units.get", { id: job.unitId }))]);
  const ref = job.draftReportRef ?? job.reportRefs[job.reportRefs.length - 1] ?? null;
  const editable = job.status === "in_progress" && !!job.draftReportRef;
  const [report, parts] = await Promise.all([
    ref ? quiet(coreOp<ApiTechReport>("reports.get", { jobId, reportId: ref.reportId, reportVersion: ref.reportVersion })) : null,
    editable || job.status === "in_progress" ? quiet(coreAll<Part>("parts.list")) : [],
  ]);
  const u = isRefused(unit) ? null : unit;
  const r = report === null || isRefused(report) ? null : report;
  const a = job.assignment;
  const known = knownOf(i,
    [...timeInstants(job.timeOnSite), a?.scheduledEnd, r?.signOff?.signedAt, ...(r?.measurements ?? []).map((m) => m.observedAt)],
    [a?.scheduledStart, a?.scheduledEnd, ...(r?.reviewHistory ?? []).map((h) => h.occurredAt)],
    a ? [[a.scheduledStart, a.scheduledEnd]] : []);
  return {
    now: now.toISOString(), known, zone: display.timeZone,
    job: {
      id: job.id, version: job.version, status: job.status, type: job.type, unitId: job.unitId, symptom: job.symptom, alertCount: job.alertIds.length, origin: job.origin,
      assignment: job.assignment, timeOnSite: job.timeOnSite, draftReportRef: job.draftReportRef, reportRefs: job.reportRefs,
    },
    unit: u ? { name: u.displayName, place: u.location.pathLabels.join(" › "), access: u.location.accessInstructions, model: `${u.capabilities.manufacturer} ${u.capabilities.model}`, scope: u.serviceScope ?? ["indoor", "outdoor", "electrical"] } : null,
    unitRefused: isRefused(unit) ? unit.refused : null,
    components: componentsFor(u?.serviceScope ?? ["indoor", "outdoor", "electrical"]),
    report: r, reportRefused: isRefused(report) ? report.refused : null,
    parts: isRefused(parts) ? [] : parts,
  };
}
