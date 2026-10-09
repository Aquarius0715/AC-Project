// /technician/units/[id]/control (FR-T10, SCR-T10): in API mode a Server Component reads the unit (units.get: observed
// state, capabilities, restriction, pending commands), the job of the URL jobId (opened from the unit or job page) with
// jobs.get (assignment window, version), the job's diagnosticRuns.list and its command history (commands.list, IR216). Sending a diagnostic command (commands.create) and starting a test run
// (diagnosticRuns.create) are Server Actions; the page refreshes while a command or run is open. Without
// control.diagnose the page says so. URL key: jobId. The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { Card, Page } from "@ac/web/components/ui";
import { apiMode, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { historyRows, observedText, runActive, runBanner, stateTiles, windowText, type ApiCommandRow, type ApiRun } from "@ac/web/lib/techControl";
import { ControlDemo } from "./_components/control-demo";
import { ControlView, type ControlLive } from "./_components/control-view";

type JobDetail = { id: string; version: number; status: string; assignment: { scheduledStart: string; scheduledEnd: string; status: string } | null };

export default async function TechnicianControlPage({ params, searchParams }: PageProps<"/technician/units/[id]/control">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <ControlDemo id={id} />;
  const sp = await searchParams;
  let d: ApiUnitDetail;
  try {
    d = await coreOp<ApiUnitDetail>("units.get", { id });
  } catch (e) {
    if (e instanceof CoreError && e.error.messageKey === "errors.assignment_not_started") {
      return <Page className="max-w-xl"><Card title="Not started yet" sub="Diagnostic control opens at the start of your work window. Open the job to check the scheduled time." /></Page>;
    }
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || e.error.fieldErrors.id)) notFound();
    throw e;
  }
  const [perms, now] = await Promise.all([corePermissions(), coreNow()]);
  const jobId = typeof sp.jobId === "string" && sp.jobId ? sp.jobId : null; // the job the commands belong to (IR94)
  const policy = d.effectiveControlPolicy.state === "restricted" ? d.effectiveControlPolicy.policy : null;
  const cap = d.capabilities;
  const unit: ControlLive["unit"] = {
    id: d.id, name: d.displayName, version: d.version, connection: d.connection, tiles: stateTiles(d), observed: observedText(d.observedState.observedAt), pending: d.pendingCommands.length,
    caps: cap, capabilityText: `Capability v${d.capabilityVersion} allows: ${[cap.modeControl && cap.modes.length ? `set_mode (${cap.modes.join("/")})` : null,
      cap.temperature ? `set_temperature ${cap.temperature.min}–${cap.temperature.max} °C` : null, cap.fanControl && cap.fanLevels.length ? `fan ${cap.fanLevels.join("/")}` : null, "test run 1–15 min"].filter(Boolean).join(", ")}.`,
    restriction: !policy ? "none" : policy.kind === "temperature_limit" ? `min ${policy.minimumCoolingSetpoint} °C` : "power off only",
    blocked: d.controlAvailability.state === "blocked" ? d.controlAvailability.reasonKey.replace(/^errors?\./, "").replace(/_/g, " ") : null,
  };
  if (!jobId) return <ControlView live={{ unit, job: null, run: null, banner: null, active: false, history: [], canDiagnose: perms.has("control.diagnose") }} />;
  let job: JobDetail;
  try {
    job = await coreOp<JobDetail>("jobs.get", { jobId });
  } catch (e) {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN")) notFound();
    throw e;
  }
  const [runs, cmds] = await Promise.all([
    coreOp<{ items: ApiRun[] }>("diagnosticRuns.list", { unitId: id, jobId, query: { limit: 10 } }),
    coreOp<{ items: ApiCommandRow[] }>("commands.list", { unitId: id, jobId, query: { limit: 20 } }),
  ]);
  const run = runs.items[0] ?? null;
  const a = job.assignment;
  const inWindow = !!a && a.status === "active" && Date.parse(a.scheduledStart) <= now.getTime() && now.getTime() < Date.parse(a.scheduledEnd);
  const live: ControlLive = {
    unit, run, banner: runBanner(run), active: runActive(run) || cmds.items.some((c) => c.status === "requested" || c.status === "sent"),
    job: { id: job.id, version: job.version, status: job.status, window: a ? windowText(a.scheduledStart, a.scheduledEnd) : "no assignment", inWindow },
    history: historyRows(cmds.items, runs.items), canDiagnose: perms.has("control.diagnose"),
  };
  return <ControlView live={live} />;
}
