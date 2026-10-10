// /admin/audit (FR-A16, SCR-A16): in API mode a Server Component reads audit.list for the URL filters (period, actor,
// target kind and ID, correlation ID, result, page size) and, for the selected entry (entryId, else the newest), the
// entries of its correlation ID — its trace and its related restriction, command, job and unit (IR320). The Actor filter
// names the users of members.list where the session may read them. The device events tab lists devices.list and reads
// devices.events only for the deviceId of the URL — there is no fallback device (IR33) — with the events' alerts
// (alerts.get, where readable) and the selected event (eventId, else the newest). Texts in the display language; the
// times are formatted here, in the display time zone, whose days the period filter takes too (IR282, IR304). The
// Phase 1A demo keeps the fixture log.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import {
  actorOptions, auditDeviceEvents, auditFilters, auditQuery, auditRow, correlationQuery, correlationTrace, periodError, periodInstants, relatedRecords, targetKinds,
  type ApiAudit, type ApiAuditDeviceEvent,
} from "@ac/web/lib/audit";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiAlertLite } from "@ac/web/lib/techDevices";
import { AuditView, type AuditLive } from "./_components/audit-view";

type Page<T> = { items: T[]; total: number };
type Device = { id: string; serial: string; unitId: string | null };
// a part the session may not read (FORBIDDEN, NOT_FOUND) is left out; UNAVAILABLE and TIMEOUT still fail the page
const optional = <T,>(p: Promise<T>, fallback: T): Promise<T> => p.catch((e) => {
  if (e instanceof CoreError && e.error.code !== "UNAVAILABLE" && e.error.code !== "TIMEOUT") return fallback;
  throw e;
});

export default async function AdminAuditPage({ searchParams }: PageProps<"/admin/audit">) {
  await connection();
  if (!apiMode()) return <AuditView />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display), zone = display.timeZone;
  const filters = auditFilters(sp, now, zone);
  const searched = !periodError(filters); // a reversed or too long period searches nothing (Figma Admin 338:602)
  const tab = one("tab") === "devices" ? "devices" : "log";
  const [page, members] = await Promise.all([
    searched ? coreOp<Page<ApiAudit>>("audit.list", auditQuery(filters, zone)) : Promise.resolve({ items: [], total: 0 }),
    optional(coreAll<{ userId: string; displayName: string }>("members.list"), []),
  ]);
  const rows = page.items.map((a) => auditRow(a, i, now.getTime()));
  const names = new Map(members.map((m) => [m.userId, m.displayName]));
  const live: AuditLive = {
    tab, rows, total: page.total, filters, searched, actors: actorOptions(members, rows, filters.actorId), kinds: targetKinds(rows, filters.targetKind),
    selected: null, entryMissing: false, trace: [], related: null,
  };
  if (tab === "log") {
    const wanted = one("entryId");
    const sel = wanted ? page.items.find((a) => a.id === wanted) : page.items[0];
    live.entryMissing = !!wanted && !sel;
    if (sel) {
      const group = await coreOp<Page<ApiAudit>>("audit.list", correlationQuery(sel));
      Object.assign(live, { selected: sel.id, trace: correlationTrace(group.items, i), related: relatedRecords(sel, group.items, i) });
    }
    return <AuditView live={live} />;
  }
  const [devices, units] = await Promise.all([
    coreAll<Device>("devices.list"),
    optional(coreAll<{ id: string; displayName: string }>("units.list"), []),
  ]);
  const unitName = new Map(units.map((u) => [u.id, u.displayName]));
  const label = (d: Device) => `${d.serial} · ${d.unitId ? unitName.get(d.unitId) ?? i.t("unit {id}", { id: d.unitId.slice(0, 8) }) : i.t("Unbound")}`;
  const wanted = one("deviceId");
  const d = wanted ? devices.find((x) => x.id === wanted) : undefined;
  live.device = {
    options: [...devices].sort((a, b) => a.serial.localeCompare(b.serial)).map((x) => ({ id: x.id, label: label(x) })),
    deviceId: d?.id ?? null, missing: !!wanted && !d, label: d ? label(d) : null, total: 0, events: [], selected: null,
  };
  if (d && searched) {
    const events = await coreOp<Page<ApiAuditDeviceEvent>>("devices.events", { id: d.id, query: { limit: 50, filters: periodInstants(filters, zone) } });
    const alerts = await Promise.all([...new Set(events.items.flatMap((e) => e.alertIds))].map((id) => optional(coreOp<ApiAlertLite>("alerts.get", { id }), null)));
    const rowsOf = auditDeviceEvents(events.items, new Map(alerts.filter((a): a is ApiAlertLite => !!a).map((a) => [a.id, a])), names, unitName, now.getTime(), i);
    Object.assign(live.device, { total: events.total, events: rowsOf, selected: rowsOf.find((r) => r.id === one("eventId"))?.id ?? rowsOf[0]?.id ?? null });
  }
  return <AuditView live={live} />;
}
