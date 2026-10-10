// /admin/devices (FR-A04, FR-A20, SCR-A04): in API mode a Server Component reads the capability register, units and
// devices; on the devices tab the selected device (devices.get) with its operations, calibrations and events; on the
// firmware tab the campaigns and the selected one (firmwareCampaigns.get). URL keys follow the screen catalog (tab,
// deviceId, campaignId). Writes are Server Actions (actions.ts). The Phase 1A demo keeps the fixture register. Texts and
// times of the first render are formatted here in the user's language and display time zone (IR295).
import { connection } from "next/server";
import { apiMode, coreDisplay, coreNow, coreOp, corePermissions, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import {
  campaignRows, deviceRows, modelRows,
  type ApiCalibration, type ApiCampaign, type ApiCapability, type ApiDevice, type ApiDeviceDetail, type ApiDeviceOperation, type ApiUnitLite,
} from "@ac/web/lib/devices";
import {
  calibrationText, deviceTiles, eventRows, firmwareCard, newerFirmware, newerVersions, openFaults, operationRows, sensorRows, techDeviceRows,
  type ApiAlertLite, type ApiDeviceEventFull,
} from "@ac/web/lib/techDevices";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { DevicesDemo } from "./_components/devices-demo";
import { DevicesView, type DevicesLive } from "./_components/devices-view";

type Page<T> = { items: T[] };
type Operation = ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string };
const absent = <T,>(p: Promise<T>) => p.catch((e) => { // an alert or unit HQ cannot read (archived, another tenant) reads as absent
  if (e instanceof CoreError && (e.error.code === "FORBIDDEN" || e.error.code === "NOT_FOUND")) return null;
  throw e;
});

export default async function AdminDevicesPage({ searchParams }: PageProps<"/admin/devices">) {
  await connection();
  if (!apiMode()) return <DevicesDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const tab = one("tab") === "devices" ? "devices" : one("tab") === "firmware" ? "firmware" : "models";
  const [now, perms, display, caps, units, devices] = await Promise.all([
    coreNow(), corePermissions(), coreDisplay(),
    coreOp<Page<ApiCapability>>("capabilities.list", { limit: 100 }),
    coreOp<Page<ApiUnitLite>>("units.list", { limit: 100 }),
    coreOp<Page<ApiDevice>>("devices.list", { limit: 100 }),
  ]);
  const i = i18nOf(display);
  const rows = deviceRows(devices.items, units.items, i.t);
  const live: DevicesLive = {
    tab, now: now.toISOString(), canAudit: perms.has("audit.read"), canWrite: perms.has("device.write"),
    models: modelRows(caps.items, units.items, devices.items, i), devices: rows,
    units: units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, label: u.displayName, modelId: u.modelId })),
  };
  if (tab === "devices") {
    // the list and the detail as Figma Admin 246:2 and the technician's device screen draw them: sorted by serial, the
    // state chips, tiles, the bound unit, the actions, sensors with Calibrate →, the operation and calibration
    // history and the events (IR319)
    const names = new Map(units.items.map((u) => [u.id, u.displayName]));
    live.rows = techDeviceRows(devices.items, names, now.getTime(), i.t);
    const id = live.rows.find((r) => r.id === one("deviceId"))?.id ?? live.rows[0]?.id;
    if (id) {
      const q = { limit: 20 };
      const [d, ops, cals, events] = await Promise.all([
        coreOp<ApiDeviceDetail & { activeOperation: Operation | null }>("devices.get", { id }),
        coreOp<Page<Operation>>("devices.operations", { deviceId: id, query: { ...q, sort: { field: "createdAt", direction: "desc" } } }),
        coreOp<Page<ApiCalibration>>("devices.calibrations", { deviceId: id, query: { ...q, sort: { field: "createdAt", direction: "desc" } } }),
        coreOp<Page<ApiDeviceEventFull>>("devices.events", { id, query: { limit: 50 } }),
      ]);
      const [unit, alerts] = await Promise.all([
        d.unitId ? absent(coreOp<ApiUnitDetail>("units.get", { id: d.unitId })) : null,
        Promise.all([...new Set(events.items.flatMap((e) => e.alertIds))].map((a) => absent(coreOp<ApiAlertLite>("alerts.get", { id: a })))),
      ]);
      const model = units.items.find((u) => u.id === d.unitId)?.modelId;
      const candidates = caps.items.find((k) => k.id === model)?.firmwareCandidates ?? [];
      live.device = {
        detail: d, row: live.rows.find((r) => r.id === id)!,
        tiles: deviceTiles(d, newerFirmware(d.firmwareVersion, candidates), now.getTime(), openFaults(events.items), i), sensors: sensorRows(d, i),
        unit: unit ? { name: unit.displayName, place: unit.location.pathLabels.join(" › "), model: `${unit.capabilities.manufacturer} ${unit.capabilities.model} v${unit.capabilityVersion}` } : null,
        firmware: newerVersions(d.firmwareVersion, candidates), firmwareCard: firmwareCard(d, ops.items, i),
        operations: operationRows(ops.items, d.firmwareVersion, i), calibrations: cals.items.map((c) => calibrationText(c, i)),
        events: eventRows(events.items, new Map(alerts.filter((a): a is ApiAlertLite => !!a).map((a) => [a.id, a])), now.getTime(), i),
      };
    }
  }
  if (tab === "firmware") {
    const campaigns = await coreOp<Page<ApiCampaign>>("firmwareCampaigns.list", { limit: 100, sort: { field: "createdAt", direction: "desc" } });
    live.campaigns = campaignRows(campaigns.items, caps.items, i);
    const id = live.campaigns.find((c) => c.id === one("campaignId"))?.id ?? live.campaigns[0]?.id;
    if (id) live.campaign = campaignRows([await coreOp<ApiCampaign>("firmwareCampaigns.get", { id })], caps.items, i)[0];
  }
  return <DevicesView live={live} />;
}
