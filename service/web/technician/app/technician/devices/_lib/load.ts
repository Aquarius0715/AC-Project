// The reads of the technician device screens (FR-T11 /technician/devices, FR-T12 /technician/devices/[id]) through the
// DAL: devices.list, units.list and jobs.list of the technician's scope, the selected device (devices.get) with its unit
// (units.get: place, capability sensors and firmware candidates), and either its operations and calibrations (T11) or
// its events with their alerts (T12). Register candidates are assigned units without a bound device. Texts in the
// user's display language; every time is formatted here (IR287).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { statusWord } from "@ac/web/lib/partnerJobDetail";
import type { ApiCalibration, ApiDevice, ApiDeviceDetail, ApiDeviceOperation, Metric } from "@ac/web/lib/devices";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import {
  calibrationText, deviceTiles, eventRows, firmwareCard, jobFor, newerFirmware, newerVersions, openFaults, operationRows, sensorRows, techDeviceRows,
  type ApiAlertLite, type ApiDeviceEventFull, type TechJob,
} from "@ac/web/lib/techDevices";
import type { TechDevicesLive } from "../_components/devices-view";

type UnitRow = { id: string; displayName: string; archived: boolean };
type JobRow = { projection: string; id?: string; unitId?: string; status?: string };
type Unit = ApiUnitDetail & { capabilities: ApiUnitDetail["capabilities"] & { firmwareCandidates?: string[] } };
const quiet = <T,>(p: Promise<T>) => p.catch((e) => { // a unit before the work window or outside the job scope reads as absent
  if (e instanceof CoreError && (e.error.code === "FORBIDDEN" || e.error.code === "NOT_FOUND")) return null;
  throw e;
});

export async function loadDevices(opts: { selected?: string; unitId?: string; jobId?: string; events: boolean }): Promise<TechDevicesLive> {
  const [now, display, perms, devices, units, jobs] = await Promise.all([
    coreNow(), coreDisplay(), corePermissions(), coreAll<ApiDevice>("devices.list"), coreAll<UnitRow>("units.list"), coreAll<JobRow>("jobs.list"),
  ]);
  const i = i18nOf(display);
  const { t } = i;
  const myJobs: TechJob[] = jobs.filter((j) => j.projection === "summary" && j.id && j.unitId).map((j) => ({ id: j.id!, unitId: j.unitId!, status: j.status ?? "" }));
  const names = new Map(units.map((u) => [u.id, u.displayName]));
  const rows = techDeviceRows(devices, names, now.getTime(), t);
  const bound = new Set(devices.map((d) => d.unitId).filter(Boolean));
  const free = units.filter((u) => !u.archived && !bound.has(u.id) && jobFor(u.id, myJobs));
  const freeDetails = await Promise.all(free.map((u) => quiet(coreOp<Unit>("units.get", { id: u.id }))));
  const live: TechDevicesLive = {
    rows, canMaintain: perms.has("device.maintain"), canAcknowledge: perms.has("alert.read"), events: opts.events, device: null, // acknowledging needs alert.read (IR284)
    register: free.flatMap((u, i) => {
      const d = freeDetails[i];
      return d ? [{ unitId: u.id, label: `${d.displayName} · ${d.location.pathLabels.join(" › ")}`, model: `${d.capabilities.manufacturer} ${d.capabilities.model}`,
        sensors: d.capabilities.sensors.map((s) => s.metric as Metric), jobs: myJobs.filter((j) => j.unitId === u.id).map((j) => ({ id: j.id, label: `${j.id.slice(0, 8)} · ${statusWord(j.status, t)}` })) }] : [];
    }),
    rebind: units.filter((u) => !u.archived && jobFor(u.id, myJobs)).map((u) => ({ unitId: u.id, label: u.displayName, jobId: jobFor(u.id, myJobs)! })),
  };
  const id = rows.find((r) => r.id === opts.selected)?.id ?? rows.find((r) => opts.unitId && r.unitId === opts.unitId)?.id ?? rows[0]?.id;
  if (!id) return live;
  const d = await coreOp<ApiDeviceDetail & { activeOperation: (ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string }) | null }>("devices.get", { id });
  const unit = d.unitId ? await quiet(coreOp<Unit>("units.get", { id: d.unitId })) : null;
  const jobId = (opts.jobId && myJobs.some((j) => j.id === opts.jobId && j.unitId === d.unitId) ? opts.jobId : null) ?? jobFor(d.unitId, myJobs);
  const candidates = unit?.capabilities.firmwareCandidates ?? [];
  const maintain = live.canMaintain && !!jobId;
  const [ops, cals, evs] = await Promise.all([
    maintain && !opts.events ? quiet(coreOp<{ items: (ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string })[] }>("devices.operations", { deviceId: id, query: { limit: 20, sort: { field: "createdAt", direction: "desc" } } })) : null,
    maintain && !opts.events ? quiet(coreOp<{ items: ApiCalibration[] }>("devices.calibrations", { deviceId: id, query: { limit: 20, sort: { field: "createdAt", direction: "desc" } } })) : null,
    opts.events ? quiet(coreOp<{ items: ApiDeviceEventFull[] }>("devices.events", { id, query: { limit: 50 } })) : null, // FORBIDDEN without an assignment on the unit

  ]);
  const alertIds = [...new Set((evs?.items ?? []).flatMap((e) => e.alertIds))];
  const alerts = new Map((await Promise.all(alertIds.map((a) => quiet(coreOp<ApiAlertLite>("alerts.get", { id: a }))))).filter((a): a is ApiAlertLite => !!a).map((a) => [a.id, a]));
  live.device = {
    detail: d, row: rows.find((r) => r.id === id)!, jobId, tiles: deviceTiles(d, newerFirmware(d.firmwareVersion, candidates), now.getTime(), evs ? openFaults(evs.items) : {}, i), sensors: sensorRows(d, i),
    unit: unit ? { name: unit.displayName, place: unit.location.pathLabels.join(" › "), model: `${unit.capabilities.manufacturer} ${unit.capabilities.model} v${unit.capabilityVersion}` } : null,
    firmware: newerVersions(d.firmwareVersion, candidates), firmwareCard: firmwareCard(d, ops?.items ?? [], i),
    operations: operationRows(ops?.items ?? [], d.firmwareVersion, i), calibrations: (cals?.items ?? []).map((c) => calibrationText(c, i)),
    events: evs ? eventRows(evs.items, alerts, now.getTime(), i) : null, eventsLocked: opts.events && !evs,
  };
  return live;
}
