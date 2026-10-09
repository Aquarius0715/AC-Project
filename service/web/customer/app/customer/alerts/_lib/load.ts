// The reads of /customer/alerts (FR-C08, DD-C08, IR242) through the DAL: the customer's alerts (alerts.list, severity
// filter critical / warning — all omits it, IR74), the signed-in membership's notifications about them
// (notifications.list: their read state, DD-C08), the unresolved alert count (summaries.get alertCount, IR51) and the
// units with their places (units.list, properties.list, spaces.list). URL keys: tab (policies), severity, unreadOnly.
import "server-only";
import { coreAll, coreOp } from "@ac/web/lib/dal";
import { unitPlaces } from "@ac/web/lib/clientBilling";
import { inboxAlerts, type AlertNote, type ApiAlert } from "@ac/web/lib/alerts";

type Unit = { id: string; displayName: string; propertyId: string; spaceId: string | null };
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export async function loadAlerts(sp: Record<string, string | string[] | undefined>) {
  const tab = one(sp.tab) === "policies" ? "policies" as const : "alerts" as const;
  const severity = one(sp.severity) === "critical" || one(sp.severity) === "warning" ? one(sp.severity) as "critical" | "warning" : null;
  const unreadOnly = one(sp.unreadOnly) === "true";
  const [alerts, notes, summary, units, properties, spaces] = await Promise.all([
    coreAll<ApiAlert>("alerts.list", severity ? { filters: { severity } } : {}), coreAll<AlertNote>("notifications.list"),
    coreOp<{ counts: { alertCount: number } }>("summaries.get", { kind: "customer", filters: {} }),
    coreAll<Unit>("units.list"), coreAll<{ id: string; name: string }>("properties.list"), coreAll<{ id: string; name: string; parentSpaceId: string | null }>("spaces.list"),
  ]);
  const place = unitPlaces(units, properties, spaces);
  const all = inboxAlerts(alerts, notes, (id) => place.get(id));
  return {
    tab, severity, unreadOnly, alertCount: summary.counts.alertCount, total: all.length, unread: all.filter((a) => a.unread.length).length,
    rows: unreadOnly ? all.filter((a) => a.unread.length) : all,
  };
}

export type AlertsLive = Awaited<ReturnType<typeof loadAlerts>>;
