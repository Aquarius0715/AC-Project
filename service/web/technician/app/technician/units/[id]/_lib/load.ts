// The reads of the technician's unit screen (FR-T02, FR-T03, DD-T02, DD-T03, SCR-T02) through the DAL: units.get
// (technician:assigned — an external technician inside the work window, IR49(b) / IR169), the unit's alerts, the
// technician's jobs on it (a lookup: the job diagnostic control works for and the maintenance history), the chosen
// metric over the period (telemetry.series) and, on the monitoring tab, the other metrics and the events of the period
// from the unit's device (devices.list, devices.events). Before the work window starts the page says when it opens
// (IR76, jobs.get of the URL's job). Texts in the user's display language; every time is formatted here (IR283).
import "server-only";
import { coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showTime } from "@ac/web/lib/i18n";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiDeviceEventFull } from "@ac/web/lib/techDevices";
import {
  componentCards, controlJob, jobRows, liveTiles, metricOf, openAlerts, otherMetrics, PERIODS, periodOf, periodRange, registerRows, seriesChart, stoppedBanner,
  windowEvents, type ApiTechUnit, type Point,
} from "@ac/web/lib/techUnit";

type Page<T> = { items: T[]; nextCursor?: string | null };
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});
const gone = (e: unknown) => e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || !!e.error.fieldErrors.id);

/** The readings of a metric in the period, newest first: up to `pages` pages of 100 (telemetry.series pages its query). */
async function series(unitId: string, metric: string, from: number, to: number, pages: number): Promise<Point[]> {
  const out: Point[] = [];
  let cursor: string | null | undefined;
  for (let page = 0; page < pages; page++) {
    const r: Page<Point> = await coreOp<Page<Point>>("telemetry.series", {
      from: new Date(from).toISOString(), to: new Date(to + 1000).toISOString(), unitIds: [unitId], metric, query: { limit: 100, sort: { field: "observedAt", direction: "desc" }, ...(cursor ? { cursor } : {}) },
    });
    out.push(...r.items);
    cursor = r.nextCursor;
    if (!cursor) break;
  }
  return out;
}

export async function loadTechUnit(id: string, sp: { jobId?: string; metric?: string; period?: string }) {
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display);
  const { t } = i;
  const nowMs = now.getTime();
  const d = await coreOp<ApiTechUnit>("units.get", { id }).catch(async (e) => {
    if (e instanceof CoreError && e.error.messageKey === "errors.assignment_not_started") return "not_started" as const;
    if (gone(e)) return null;
    throw e;
  });
  if (d === null) return null; // not in the technician's assignments: not found (IR169)
  if (d === "not_started") { // IR76: the start time comes from the URL's job; without it only the general sentence
    const job = sp.jobId ? await optional(coreOp<{ assignment: { scheduledStart: string } | null }>("jobs.get", { jobId: sp.jobId }), null) : null;
    const start = job?.assignment?.scheduledStart ?? null;
    return {
      kind: "not_started" as const, title: t("Not started yet"),
      text: start ? t("You can see this AC from {time}, when your work window starts.", { time: showTime(start, display) }) : t("Available from the work start time of your assigned job."),
      job: start && sp.jobId ? { href: `/technician/jobs/${sp.jobId}`, label: t("Open the job →") } : null,
    };
  }
  const tab = sp.metric ? "monitoring" as const : "register" as const;
  const metric = metricOf(d, sp.metric), period = periodOf(sp.period);
  const { from, to } = periodRange(period, nowMs);
  const unitOf = (m: string) => d.latestMeasurements.find((x) => x.metric === m)?.unit ?? "";
  const others = tab === "monitoring" ? d.latestMeasurements.filter((m) => m.metric !== metric).slice(0, 3).map((m) => m.metric) : [];
  const [alerts, jobs, points, more, devices] = await Promise.all([
    optional(coreOp<Page<ApiAlert>>("alerts.list", { limit: 50, filters: { unitId: id } }).then((r) => r.items), [] as ApiAlert[]),
    optional(coreOp<Page<ApiTechJobRow>>("jobs.list", { limit: 20, filters: { unitId: id } }).then((r) => r.items), [] as ApiTechJobRow[]),
    optional(series(id, metric, from, to, 4), [] as Point[]),
    Promise.all(others.map((m) => optional(series(id, m, from, to, 1), [] as Point[]))),
    tab === "monitoring" ? optional(coreOp<Page<{ id: string }>>("devices.list", { limit: 5, filters: { unitId: id } }).then((r) => r.items), []) : Promise.resolve([]),
  ]);
  const events = (await Promise.all(devices.map((x) => optional(coreOp<Page<ApiDeviceEventFull>>("devices.events", {
    id: x.id, query: { limit: 50, filters: { from: new Date(from).toISOString(), to: new Date(to + 1000).toISOString() } },
  }).then((r) => r.items), [] as ApiDeviceEventFull[])))).flat();
  const control = controlJob(jobs, sp.jobId);
  const job = control ?? sp.jobId ?? null; // the job the alert evidence opens with
  const back = sp.jobId ? `/technician/jobs/${sp.jobId}` : "/technician";
  return {
    kind: "live" as const, id, short: id.slice(0, 8), name: d.displayName, back, tab, metric, period, connection: d.connection,
    tabs: [{ id: "register" as const, label: t("Register") }, { id: "monitoring" as const, label: t("Monitoring") }],
    periods: PERIODS.map((p) => ({ id: p, label: t(p) })),
    stopped: stoppedBanner(d, nowMs, i),
    register: registerRows(d, i), components: componentCards(d, i), componentCount: d.components.length,
    tiles: liveTiles(d, nowMs, i), chart: seriesChart(metric, unitOf(metric), points, period, nowMs, i),
    others: otherMetrics(d, metric, Object.fromEntries(others.map((m, k) => [m, more[k]])), period, nowMs, i),
    events: windowEvents(alerts, events, period, nowMs, i),
    history: jobRows(jobs, i), alerts: openAlerts(alerts, nowMs, i),
    alertsHref: `/technician/units/${id}/alerts${job ? `?jobId=${job}` : ""}`,
    controlHref: control ? `/technician/units/${id}/control?jobId=${control}` : null,
  };
}

export type UnitLive = Extract<NonNullable<Awaited<ReturnType<typeof loadTechUnit>>>, { kind: "live" }>;
