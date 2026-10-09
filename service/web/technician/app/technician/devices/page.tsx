// /technician/devices (FR-T11, SCR-T11): in API mode a Server Component reads the devices of the technician's assigned
// units with the selected device (deviceId, else the device of unitId, else the first), its unit, operations and
// calibrations (_lib/load.ts). Register, rebind, connection check, calibration and firmware update are Server Actions
// under the unit's assigned job (jobId, IR94). URL keys: deviceId, unitId, jobId, tab, sort. The Phase 1A demo keeps
// the fixtures.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { DevicesView } from "@ac/web/components/TechDevices";
import { loadDevices } from "./_lib/load";
import { TechDevicesView } from "./_components/devices-view";

export default async function TechnicianDevicesPage({ searchParams }: PageProps<"/technician/devices">) {
  await connection();
  if (!apiMode()) return <DevicesView />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  return <TechDevicesView live={await loadDevices({ selected: one("deviceId"), unitId: one("unitId"), jobId: one("jobId"), events: false })} />;
}
