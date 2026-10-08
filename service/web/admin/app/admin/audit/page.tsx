// /admin/audit (FR-A16): in API mode a Server Component reads audit.list for the URL filters (period, correlation ID,
// result) and, on the device events tab, devices.events of the selected device through the DAL. The Phase 1A demo
// keeps the fixture log.
import { connection } from "next/server";
import { apiMode, coreNow, coreOp } from "@ac/web/lib/dal";
import { auditFilters, auditQuery, auditRow, deviceEventItem, periodError, type ApiAudit, type ApiDeviceEvent } from "@ac/web/lib/audit";
import { AuditView } from "./_components/audit-view";

type Device = { id: string; serial: string };

export default async function AdminAuditPage({ searchParams }: PageProps<"/admin/audit">) {
  await connection();
  if (!apiMode()) return <AuditView />;
  const sp = await searchParams;
  const filters = auditFilters(sp, await coreNow());
  const ok = !periodError(filters);
  const page = ok ? await coreOp<{ items: ApiAudit[]; total: number }>("audit.list", auditQuery(filters)) : { items: [], total: 0 };
  let device: { label: string; items: ReturnType<typeof deviceEventItem>[] } | undefined;
  if (sp.tab === "devices") {
    const devices = await coreOp<{ items: Device[] }>("devices.list", { limit: 100 });
    const d = devices.items.find((x) => x.id === sp.deviceId) ?? devices.items[0];
    const events = d ? await coreOp<{ items: ApiDeviceEvent[] }>("devices.events", { id: d.id, query: { limit: 50 } }) : { items: [] };
    device = { label: d ? `${d.serial} · connection, power and tamper are separate` : "No devices", items: events.items.map(deviceEventItem) };
  }
  const tab = sp.tab === "devices" ? "devices" : "log";
  return <AuditView live={{ rows: page.items.map(auditRow), total: page.total, filters, tab, device }} />;
}
