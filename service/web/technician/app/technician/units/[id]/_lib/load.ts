// The reads of the technician's unit screen (FR-T02, FR-T03, DD-T02, DD-T03, SCR-T02) through the DAL: units.get
// (technician:assigned — an external technician inside the work window, IR49(b) / IR169), the unit's alerts, the
// technician's jobs on it (a lookup: the job diagnostic control works for and the maintenance history), the chosen
// metric over the period (telemetry.series) and, on the monitoring tab, the other metrics and the events of the period
// from the unit's device (devices.list, devices.events). Before the work window starts the page says when it opens
// (IR76, jobs.get of the URL's job). Texts in the user's display language; every time is formatted here (IR283).
import "server-only";
import { coreDisplay, coreNow, coreOp, CoreError, orWhenRefused, coreItems } from "@ac/web/lib/dal";
import { i18nOf, showTime } from "@ac/web/lib/i18n";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { TechJobRead } from "@ac/web/lib/techJob";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiDeviceEventFull } from "@ac/web/lib/techDevices";
import type { OpInput } from "@ac/web/lib/opTypes";
import {
  componentCards, controlJob, jobRows, liveTiles, metricOf, openAlerts, otherMetrics, PERIODS, periodOf, periodRange, registerRows, seriesChart, stoppedBanner,
  windowEvents, type ApiTechUnit, type Period, type Point,
} from "@ac/web/lib/techUnit";

interface Page<T> { items: T[]; nextCursor?: string | null; total?: number }
/** telemetry.series: a page of readings, or with bucketMinutes the period's averages (AirSeries.buckets, IR605). */
type SeriesPage = Page<Point> & { buckets: { start: string; value: number | null; count: number }[] | null };
const optional = orWhenRefused;
const gone = (e: unknown) => e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || !!e.error.fieldErrors.id);

/** The readings of a metric in the period. A day and a week come whole, as 5- and 60-minute averages of their valid
readings (bucketMinutes, IR605, IR606); an hour comes as its readings, newest first, up to `pages` pages of 100
(telemetry.series pages its query), with how many the period holds, so a cut hour says what is not loaded (IR598). */
async function series(unitId: string, metric: OpInput<"telemetry.series">["metric"], from: number, to: number, period: Period, pages: number): Promise<{ items: Point[]; total: number; complete: boolean }> {
  const range = { from: new Date(from).toISOString(), to: new Date(to + 1000).toISOString(), unitIds: [unitId], metric };
  if (period !== "1h") {
    const r: SeriesPage = await coreOp<SeriesPage>("telemetry.series", { ...range, bucketMinutes: period === "24h" ? 5 : 60, query: {} });
    const items = (r.buckets ?? []).filter((b) => b.count > 0).map((b): Point => ({ value: b.value, observedAt: b.start }));
    return { items, total: r.total ?? 0, complete: true };
  }
  const out: Point[] = [];
  let total = 0;
  let cursor: string | null | undefined;
  for (let page = 0; page < pages; page++) {
    const r: SeriesPage = await coreOp<SeriesPage>("telemetry.series", {
      ...range, query: { limit: 100, sort: { field: "observedAt", direction: "desc" }, ...(cursor && { cursor }) },
    });
    out.push(...r.items);
    if (page === 0) total = r.total ?? 0;
    cursor = r.nextCursor;
    if (!cursor) break;
  }
  return { items: out, total: Math.max(total, out.length), complete: false };
}
const NONE = { items: [] as Point[], total: 0, complete: true };

export async function loadTechUnit(id: string, sp: { jobId?: string; metric?: string; period?: string }) {
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display);
  const { t } = i;
  const nowMs = now.getTime();
  const d = await coreOp<ApiTechUnit>("units.get", { id }).catch((error: unknown) => {
    if (error instanceof CoreError && error.error.messageKey === "errors.assignment_not_started") return "not_started" as const;
    if (gone(error)) return null;
    throw error;
  });
  if (d === null) return null; // not in the technician's assignments: not found (IR169)
  if (d === "not_started") { // IR76: the start time comes from the URL's job; without it only the general sentence
    const job = sp.jobId ? await optional(coreOp<TechJobRead<{ assignment: { scheduledStart: string } | null }>>("jobs.get", { jobId: sp.jobId }), null) : null;
    const start = job?.projection === "detail" ? job.assignment?.scheduledStart ?? null : null;
    return {
      kind: "not_started" as const, title: t("Not started yet"),
      text: start ? t("You can see this AC from {time}, when your work window starts.", { time: showTime(start, display) }) : t("Available from the work start time of your assigned job."),
      job: start && sp.jobId ? { href: `/technician/jobs/${sp.jobId}`, label: t("Open the job") } : null,
    };
  }
  const tab = sp.metric ? "monitoring" as const : "register" as const;
  const metric = metricOf(d, sp.metric), period = periodOf(sp.period);
  const { from, to } = periodRange(period, nowMs);
  const unitOf = (m: string) => d.latestMeasurements.find((x) => x.metric === m)?.unit ?? "";
  const others = tab === "monitoring" ? d.latestMeasurements.filter((m) => m.metric !== metric).slice(0, 3).map((m) => m.metric) : [];
  const [alerts, jobs, points, more, devices] = await Promise.all([
    optional(coreItems<ApiAlert>("alerts.list", { limit: 50, filters: { unitId: id } }), [] as ApiAlert[]),
    optional(coreItems<ApiTechJobRow>("jobs.list", { limit: 20, filters: { unitId: id } }), [] as ApiTechJobRow[]),
    optional(series(id, metric, from, to, period, 4), NONE),
    Promise.all(others.map((m) => optional(series(id, m, from, to, period, 1), NONE))),
    tab === "monitoring" ? optional(coreItems<{ id: string }>("devices.list", { limit: 5, filters: { unitId: id } }), []) : Promise.resolve([]),
  ]);
  const eventLists = await Promise.all(devices.map((x) => {
    return optional(coreItems<ApiDeviceEventFull>("devices.events", {
      id: x.id, query: { limit: 50, filters: { from: new Date(from).toISOString(), to: new Date(to + 1000).toISOString() } },
    }), [] as ApiDeviceEventFull[]);
  }));
  const events = eventLists.flat();
  const control = controlJob(jobs, sp.jobId);
  const job = control ?? sp.jobId ?? null; // the job the alert evidence opens with
  const back = sp.jobId ? `/technician/jobs/${sp.jobId}` : "/technician";
  const alertsHref = `/technician/units/${id}/alerts${job ? `?jobId=${job}` : ""}`;
  return {
    kind: "live" as const,
    short: id.slice(0, 8),
    name: d.displayName,
    connection: d.connection,
    tabs: [{ id: "register" as const, label: t("Register") }, { id: "monitoring" as const, label: t("Monitoring") }],
    periods: PERIODS.map((p) => ({ id: p, label: t(p) })),
    stopped: stoppedBanner(d, nowMs, i),
    register: registerRows(d, i),
    components: componentCards(d, i),
    componentCount: d.components.length,
    tiles: liveTiles(d, nowMs, i),
    chart: seriesChart(metric, unitOf(metric), points.items, period, nowMs, i, points.total, points.complete),
    others: otherMetrics(d, metric, Object.fromEntries(others.map((m, k) => [m, more[k].items])), period, nowMs, i,
      Object.fromEntries(others.map((m, k) => [m, more[k].complete ? 0 : more[k].total]))),
    events: windowEvents(alerts, events, period, nowMs, i),
    history: jobRows(jobs, i),
    alerts: openAlerts(alerts, nowMs, i).map((a) => ({ ...a, href: `${alertsHref}${job ? "&" : "?"}alertId=${a.id}` })),
    controlHref: control ? `/technician/units/${id}/control?jobId=${control}` : null,
    id,
    back,
    tab,
    metric,
    period,
    alertsHref,
  };
}

export type UnitLive = Extract<NonNullable<Awaited<ReturnType<typeof loadTechUnit>>>, { kind: "live" }>;
