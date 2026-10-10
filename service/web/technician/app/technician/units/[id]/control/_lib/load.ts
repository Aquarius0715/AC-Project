// The reads of technician diagnostic control (FR-T10, DD-T10, SCR-T10) through the DAL: the unit (units.get: observed
// state, capabilities, restriction, pending commands), the job of the URL jobId (opened from the unit or job page) with
// jobs.get (assignment window, version), the job's diagnosticRuns.list and its command history (commands.list, IR216)
// and whether the user holds control.diagnose. Before the work window the page says when it opens (IR76). Texts in
// the user's display language; every time is formatted here (IR286).
import "server-only";
import { coreDisplay, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import { i18nOf, showTime } from "@ac/web/lib/i18n";
import { actionOptions } from "@ac/web/lib/clientAutomations";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import {
  blockedText, capabilityText, connectionText, historyRows, observedText, restrictionText, runActive, runBanner, stateTiles, windowText, type ApiCommandRow, type ApiRun,
} from "@ac/web/lib/techControl";

type JobDetail = { id: string; version: number; status: string; assignment: { scheduledStart: string; scheduledEnd: string; status: string } | null };
const gone = (e: unknown) => e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || !!e.error.fieldErrors.id || !!e.error.fieldErrors.jobId);

export async function loadControl(unitId: string, jobId: string | null) {
  const [now, display, perms] = await Promise.all([coreNow(), coreDisplay(), corePermissions()]);
  const i = i18nOf(display);
  const { t } = i;
  const d = await coreOp<ApiUnitDetail>("units.get", { id: unitId }).catch((e) => {
    if (e instanceof CoreError && e.error.messageKey === "errors.assignment_not_started") return "not_started" as const;
    if (gone(e)) return null;
    throw e;
  });
  const unitHref = `/technician/units/${unitId}${jobId ? `?jobId=${jobId}` : ""}`;
  if (d === null) return null; // not in the technician's assignments: not found (IR169)
  if (d === "not_started") { // IR76: the start time from the URL's job
    const job = jobId ? await coreOp<JobDetail>("jobs.get", { jobId }).catch(() => null) : null;
    const start = job?.assignment?.scheduledStart ?? null;
    return {
      kind: "not_started" as const, title: t("Not started yet"), back: t("← Unit"), unitHref,
      text: start ? t("Diagnostic control opens at {time}, when your work window starts.", { time: showTime(start, display) }) : t("Available from the work start time of your assigned job."),
    };
  }
  const unit = {
    id: d.id, name: d.displayName, version: d.version, connection: d.connection, tiles: stateTiles(d, t), observed: observedText(d.observedState.observedAt, i), pending: d.pendingCommands.length,
    options: actionOptions(d.capabilities, t), capabilityText: `${capabilityText(d, t)} ${connectionText(d.connection, t)}`, restriction: restrictionText(d, t), blocked: blockedText(d, t), current: stateTiles(d, t).filter((x, k) => k < 4).map((x) => x.value).join(" · "),
  };
  const base = { kind: "live" as const, unitHref, unit, canDiagnose: perms.has("control.diagnose") };
  if (!jobId) return { ...base, job: null, run: null, banner: null, active: false, history: [] };
  const job = await coreOp<JobDetail>("jobs.get", { jobId }).catch((e) => { if (gone(e)) return null; throw e; });
  if (!job) return null;
  const [runs, cmds] = await Promise.all([
    coreOp<{ items: ApiRun[] }>("diagnosticRuns.list", { unitId, jobId, query: { limit: 10 } }),
    coreOp<{ items: ApiCommandRow[] }>("commands.list", { unitId, jobId, query: { limit: 20 } }),
  ]);
  const run = runs.items[0] ?? null;
  const a = job.assignment;
  const inWindow = !!a && a.status === "active" && Date.parse(a.scheduledStart) <= now.getTime() && now.getTime() < Date.parse(a.scheduledEnd);
  return {
    ...base, run, banner: runBanner(run, i), active: runActive(run) || cmds.items.some((c) => c.status === "requested" || c.status === "sent"),
    job: { id: job.id, version: job.version, status: job.status, window: a ? windowText(a.scheduledStart, a.scheduledEnd, i) : t("no assignment"), inWindow },
    history: historyRows(cmds.items, runs.items, i),
  };
}

export type ControlLive = Extract<NonNullable<Awaited<ReturnType<typeof loadControl>>>, { kind: "live" }>;
