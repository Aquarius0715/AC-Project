// /demo (FR-X05, DDC-07, SCR-X-demo, IR249; Figma Client 10e): in API mode a Server Component reads the Core API scenario
// clock (session.get meta.snapshotAt) and the devices the signed-in role can see (devices.list, units.list for the names).
// Only devices whose history it can read are offered (devices.events: the next sequence and the open connection fault a
// recovery names; a technician needs a current assignment, SR24); the changes are Server Actions (./actions.ts). The
// Phase 1A demo keeps its browser-only controls.
import { connection } from "next/server";
import { apiMode, coreAll, coreNow, coreOp } from "@ac/web/lib/dal";
import { clockText, deviceOptions, openFault, type ApiDevice, type ApiDeviceEvent } from "@ac/web/lib/demoPanel";
import { DemoMock } from "./_components/demo-mock";
import { DemoView } from "./_components/demo-view";

export default async function DemoPage() {
  await connection();
  if (!apiMode()) return <DemoMock />;
  const [now, devices, units] = await Promise.all([
    coreNow(),
    coreAll<ApiDevice>("devices.list").catch(() => null), // a role without device reads gets no device tile
    coreAll<{ id: string; displayName: string }>("units.list").catch(() => []),
  ]);
  const listed = (devices ?? []).slice(0, 20);
  const histories = await Promise.all(listed.map((d) => coreOp<{ items: ApiDeviceEvent[] }>("devices.events", { id: d.id, query: { limit: 100 } }).then((p) => p.items).catch(() => null)));
  const faults = new Map<string, string>();
  histories.forEach((h, i) => { const f = h && openFault(h); if (f) faults.set(listed[i].id, f.id); });
  const usable = listed.filter((_, i) => histories[i] !== null);
  return <DemoView live={{ clock: clockText(now.toISOString()), devices: devices && deviceOptions(usable, new Map(units.map((u) => [u.id, u.displayName])), faults), hidden: listed.length - usable.length }} />;
}
