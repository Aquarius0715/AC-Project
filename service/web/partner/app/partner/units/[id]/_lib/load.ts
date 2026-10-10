// The reads of the contractor's unit view (FR-P04, FR-P08, DD-P04) through the DAL: units.get inside the accepted
// offer's access window (contractor:accepted-valid-offer, IR169), its alerts, the company's jobs on it, the members
// for the technician's name and the last 24 hours of two diagnosis metrics (telemetry.series). Outside the window the
// unit reads as not found; with the URL's jobId the page then shows that job's history snapshot (Figma: delegation
// ended while open). Texts in the user's display language; every time is formatted here (IR280).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showSpan, showTime } from "@ac/web/lib/i18n";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import type { ApiMember } from "@ac/web/lib/partnerOverview";
import type { ApiPartnerJobDetail } from "@ac/web/lib/partnerJobDetail";
import { chart, chartMetrics, contextJob, evidenceRows, jobContext, pastWork, readingRows, registerRows, snapshotRows, type ApiUnitJob } from "@ac/web/lib/partnerUnit";
import type { OpInput } from "@ac/web/lib/opTypes";

type Page<T> = { items: T[]; nextCursor?: string | null };
type Point = { value: number | null; observedAt: string; quality?: string; unit?: string };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const gone = (e: unknown) => e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || !!e.error.fieldErrors.id);
const minute = (ms: number) => new Date(ms).toISOString().slice(0, 16) + ":00Z";

/** Up to 400 readings of a metric in the window, newest first (telemetry.series pages its query). */
async function series(unitId: string, metric: OpInput<"telemetry.series">["metric"], from: number, to: number): Promise<Point[]> {
  const out: Point[] = [];
  let cursor: string | null | undefined;
  for (let page = 0; page < 4; page++) {
    const r: Page<Point> = await coreOp<Page<Point>>("telemetry.series", { from: minute(from), to: minute(to), unitIds: [unitId], metric, query: { limit: 100, sort: { field: "observedAt", direction: "desc" }, ...(cursor ? { cursor } : {}) } });
    out.push(...r.items);
    cursor = r.nextCursor;
    if (!cursor) break;
  }
  return out;
}

export async function loadUnit(id: string, jobId?: string) {
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display);
  const { t } = i;
  const nowMs = now.getTime();
  const d = await coreOp<ApiUnitDetail>("units.get", { id }).catch((e) => { if (gone(e)) return null; throw e; });
  if (!d) {
    if (!jobId) return null; // not one of the company's units now: not found
    const h = await coreOp<ApiPartnerJobDetail>("jobs.get", { jobId }).catch((e) => { if (gone(e)) return null; throw e; });
    if (!h || h.projection !== "history") return null;
    return {
      kind: "snapshot" as const, id, jobId: h.jobId, title: t("Job history snapshot — {id}", { id: h.jobId.slice(0, 8) }), rows: snapshotRows(h, i),
      banner: t("Access to this unit ended {when} (delegation period). Reading it again was refused, and live values are not shown any more.", { when: showTime(h.asOf, display) }),
    };
  }
  const [alerts, jobs, members] = await Promise.all([
    optional(coreOp<Page<ApiAlert>>("alerts.list", { limit: 20, filters: { unitId: id } }).then((r) => r.items), [] as ApiAlert[]),
    optional(coreOp<Page<ApiUnitJob>>("jobs.list", { limit: 50, filters: { unitId: id } }).then((r) => r.items), [] as ApiUnitJob[]),
    optional(coreAll<ApiMember>("members.list"), [] as ApiMember[]),
  ]);
  const metrics = chartMetrics(d);
  const points = await Promise.all(metrics.map((m) => optional(series(id, m, nowMs - 86_400_000, nowMs), [] as Point[])));
  const job = contextJob(jobs, jobId);
  const names = new Map(members.map((m) => [m.id, m.displayName]));
  const unitOf = (m: string) => d.latestMeasurements.find((x) => x.metric === m)?.unit ?? "";
  return {
    kind: "live" as const, id, name: d.displayName,
    banner: job ? t("Diagnosis-scoped view for {job} — no billing, payments, or other contracts.", { job: job.id.slice(0, 8) }) : t("Diagnosis-scoped view for your accepted job — no billing, payments, or other contracts."),
    register: registerRows(d, i), context: job ? jobContext(job, names, nowMs, i) : null, past: pastWork(jobs, i), alerts: evidenceRows(alerts, i), readings: readingRows(d, i),
    charts: metrics.map((m, k) => chart(m, unitOf(m), points[k], nowMs, i)),
    window: showSpan(new Date(nowMs - 86_400_000).toISOString(), now.toISOString(), display), // the charts' last 24 hours
  };
}

export type UnitLive = NonNullable<Awaited<ReturnType<typeof loadUnit>>>;
