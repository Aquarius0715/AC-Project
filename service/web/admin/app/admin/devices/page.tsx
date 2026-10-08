// /admin/devices (FR-A04, FR-A20, SCR-A04): in API mode a Server Component reads the capability register, units and
// devices; on the devices tab the selected device (devices.get) with its operations, calibrations and events; on the
// firmware tab the campaigns and the selected one (firmwareCampaigns.get). URL keys follow the screen catalog (tab,
// deviceId, campaignId). Writes are Server Actions (actions.ts). The Phase 1A demo keeps the fixture register.
import { connection } from "next/server";
import { apiMode, coreNow, coreOp, corePermissions } from "@ac/web/lib/dal";
import { deviceEventItem, type ApiDeviceEvent } from "@ac/web/lib/audit";
import {
  calibrationItem, campaignRows, deviceRows, modelRows, operationItem,
  type ApiCalibration, type ApiCampaign, type ApiCapability, type ApiDevice, type ApiDeviceDetail, type ApiDeviceOperation, type ApiUnitLite,
} from "@ac/web/lib/devices";
import { DevicesDemo } from "./_components/devices-demo";
import { DevicesView, type DevicesLive } from "./_components/devices-view";

type Page<T> = { items: T[] };

export default async function AdminDevicesPage({ searchParams }: PageProps<"/admin/devices">) {
  await connection();
  if (!apiMode()) return <DevicesDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const tab = one("tab") === "devices" ? "devices" : one("tab") === "firmware" ? "firmware" : "models";
  const [now, perms, caps, units, devices] = await Promise.all([
    coreNow(), corePermissions(),
    coreOp<Page<ApiCapability>>("capabilities.list", { limit: 100 }),
    coreOp<Page<ApiUnitLite>>("units.list", { limit: 100 }),
    coreOp<Page<ApiDevice>>("devices.list", { limit: 100 }),
  ]);
  const rows = deviceRows(devices.items, units.items);
  const live: DevicesLive = {
    tab, now: now.toISOString(), canAudit: perms.has("audit.read"), canWrite: perms.has("device.write"),
    models: modelRows(caps.items, units.items, devices.items), devices: rows,
    units: units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, label: u.displayName, modelId: u.modelId })),
  };
  if (tab === "devices") {
    const id = rows.find((r) => r.id === one("deviceId"))?.id ?? rows[0]?.id;
    if (id) {
      const q = { limit: 20 };
      const [d, ops, cals, events] = await Promise.all([
        coreOp<ApiDeviceDetail>("devices.get", { id }),
        coreOp<Page<ApiDeviceOperation>>("devices.operations", { deviceId: id, query: { ...q, sort: { field: "createdAt", direction: "desc" } } }),
        coreOp<Page<ApiCalibration>>("devices.calibrations", { deviceId: id, query: { ...q, sort: { field: "createdAt", direction: "desc" } } }),
        coreOp<Page<ApiDeviceEvent>>("devices.events", { id, query: q }),
      ]);
      const unit = units.items.find((u) => u.id === d.unitId);
      live.device = {
        detail: d, row: rows.find((r) => r.id === id)!, firmware: caps.items.find((k) => k.id === unit?.modelId)?.firmwareCandidates ?? [],
        operations: ops.items.map(operationItem), calibrations: cals.items.map(calibrationItem), events: events.items.map(deviceEventItem),
      };
    }
  }
  if (tab === "firmware") {
    const campaigns = await coreOp<Page<ApiCampaign>>("firmwareCampaigns.list", { limit: 100, sort: { field: "createdAt", direction: "desc" } });
    live.campaigns = campaignRows(campaigns.items, caps.items);
    const id = live.campaigns.find((c) => c.id === one("campaignId"))?.id ?? live.campaigns[0]?.id;
    if (id) live.campaign = campaignRows([await coreOp<ApiCampaign>("firmwareCampaigns.get", { id })], caps.items)[0];
  }
  return <DevicesView live={live} />;
}
