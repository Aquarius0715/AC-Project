// /admin/audit (FR-A16): in API mode a Server Component reads audit.list for the URL filters (period, correlation ID,
// result) and, on the device events tab, devices.events of the selected device through the DAL. Texts in the display
// language; the entries' times are formatted here, in the display time zone, whose days the period filter takes too
// (IR282, IR304). The Phase 1A demo keeps the fixture log.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreNow, coreOp } from "@ac/web/lib/dal";
import { auditFilters, auditQuery, auditRow, deviceEventItem, periodError, type ApiAudit, type ApiDeviceEvent } from "@ac/web/lib/audit";
import { i18nOf } from "@ac/web/lib/i18n";
import { AuditView } from "./_components/audit-view";

type Device = { id: string; serial: string };

export default async function AdminAuditPage({ searchParams }: PageProps<"/admin/audit">) {
  await connection();
  if (!apiMode()) return <AuditView />;
  const sp = await searchParams;
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display), zone = display.timeZone;
  const filters = auditFilters(sp, now, zone);
  const ok = !periodError(filters);
  const page = ok ? await coreOp<{ items: ApiAudit[]; total: number }>("audit.list", auditQuery(filters, zone)) : { items: [], total: 0 };
  let device: { label: string; items: ReturnType<typeof deviceEventItem>[] } | undefined;
  if (sp.tab === "devices") {
    const devices = await coreOp<{ items: Device[] }>("devices.list", { limit: 100 });
    const d = devices.items.find((x) => x.id === sp.deviceId) ?? devices.items[0];
    const events = d ? await coreOp<{ items: ApiDeviceEvent[] }>("devices.events", { id: d.id, query: { limit: 50 } }) : { items: [] };
    device = { label: d ? i.t("{serial} · connection, power and tamper are separate", { serial: d.serial }) : i.t("No devices"), items: events.items.map((e) => deviceEventItem(e, i, now.getTime())) };
  }
  const tab = sp.tab === "devices" ? "devices" : "log";
  return <AuditView live={{ rows: page.items.map((a) => auditRow(a, i, now.getTime())), total: page.total, filters, tab, device }} />;
}
