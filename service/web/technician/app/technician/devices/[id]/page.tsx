// /technician/devices/[id] (FR-T12, SCR-T12): in API mode the same device screen with the device's events — detection,
// evidence source, alerts and response notes kept apart (_lib/load.ts). Response notes (devices.addResponseNote) and
// alert acknowledgement (alerts.acknowledge) are Server Actions; acknowledging never changes the device state. A device
// outside the technician's scope is not found. URL key: tab. The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode, CoreError } from "@ac/web/lib/dal";
import { DevicesView } from "@ac/web/components/TechDevices";
import { loadDevices } from "../_lib/load";
import { TechDevicesView } from "../_components/devices-view";
import { recover } from "@ac/web/lib/promises";

export default async function TechnicianDeviceEventsPage({ params }: Readonly<PageProps<"/technician/devices/[id]">>) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <DevicesView initial={id} />;
  const live = await recover(loadDevices({ selected: id, events: true }), (error: unknown) => {
    if (error instanceof CoreError && (error.error.code === "NOT_FOUND" || error.error.code === "FORBIDDEN")) notFound();
    throw error;
  });
  if (live.device?.detail.id !== id) notFound(); // not one of the technician's devices
  return <TechDevicesView live={live} />;
}
