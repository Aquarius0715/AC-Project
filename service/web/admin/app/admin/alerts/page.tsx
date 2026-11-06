// /admin/alerts (FR-A05): in API mode a Server Component reads alerts.list, policies.list and the names of units,
// customers and members through the DAL; acknowledging, resolving and saving a policy are Server Actions
// (actions.ts). The Phase 1A demo keeps the fixture rows. The rows' texts and times are formatted here in the user's
// language and display time zone, so the first render matches the browser's (IR292). The Alerts tab's scope (customer →
// property → unit), severity, status and selected alert live in the URL (DD-A05 item 5, Figma Admin 256:2, IR318); so do
// the Policies tab's selected policy (`policyId`) and the owner customer of a new one (`new`, DD-A05 item 8, IR345).
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, corePermissions, CoreError, orWhenForbidden, coreAllPage } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { adminAlertRows, policyRows, type AlertState, type ApiAdminAlert, type ApiCustomerName, type ApiMemberName, type ApiPolicy, type ApiUnitName, type Severity } from "@ac/web/lib/adminAlerts";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";

import { AdminAlertsView } from "./_components/alerts-view";

const ASSET_READ = "asset.read";

const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
// names the session may not read (no identity.read or job.read)
const optional = orWhenForbidden;
const SEVERITIES: Severity[] = ["critical", "warning", "normal"];
const STATES: AlertState[] = ["open", "acknowledged", "resolved"];

export default async function AdminAlertsPage({ searchParams }: Readonly<PageProps<"/admin/alerts">>) {
  interface Page<T> { items: T[] }
  await connection();
  if (!apiMode()) return <AdminAlertsView />;
  const sp = await searchParams;
  const severity = SEVERITIES.find((s) => s === one(sp.severity)) ?? null;
  const q = {
    customerId: one(sp.customerId),
    propertyId: one(sp.propertyId),
    unitId: one(sp.unitId),
    status: STATES.find((s) => s === one(sp.status)) ?? "open",
    alertId: one(sp.alertId) ?? null,
    severity,
  };
  const scope = { ...(q.customerId && { customerId: q.customerId }), ...(q.propertyId && { propertyId: q.propertyId }), ...(q.unitId && { unitId: q.unitId }) };
  const filters = { ...scope, ...(severity && { severity }) };
  // the policies in the same scope (DD-A05 item 8). The default policy is read on its own, without the scope: a property
  // or unit scope leaves it out though it runs on every unit (Figma 268:2), and a customer scope would keep only that
  // customer's rule settings, while its editor switches the rules for every customer (item 9)
  // a read the role may not make answers the permission it needs: the first one missing names the page's state, instead
  // of a failure with Try again (IR581)
  const need = <T,>(p: Promise<T>, permission: string): Promise<T | { missing: string }> => {
    return p.catch((error: unknown) => {
      if (error instanceof CoreError && error.error.code === "FORBIDDEN") return { missing: permission };
      throw error;
    });
  };
  const [now, perms, display, alertsRead, unitsRead, customersRead, propertiesRead, policiesRead, members, defaultsRead] = await Promise.all([
    coreNow(), corePermissions(), coreDisplay(),
    need(coreOp<Page<ApiAdminAlert>>("alerts.list", { limit: 100, filters }), "alert.read"), // every status in the scope: the tiles count them
    need(coreAllPage<ApiUnitName & { propertyId: string; archived?: boolean }>("units.list"), ASSET_READ),
    need(coreAllPage<ApiCustomerName & { status?: "active" | "inactive" }>("customers.list"), ASSET_READ),
    need(coreAll<{ id: string; name: string; customerOrgId: string }>("properties.list"), ASSET_READ),
    need(coreAllPage<ApiPolicy>("policies.list", { filters: scope }), "alert.policy.read"),
    optional(coreAllPage<ApiMemberName>("members.list"), { items: [], total: 0 }),
    need(coreOp<Page<ApiPolicy>>("policies.list", { limit: 1, filters: { kind: "default_alert" } }), "alert.policy.read"),
  ]);
  const isMissing = (r: unknown): r is { missing: string } => typeof r === "object" && r !== null && "missing" in r;
  const missing = [alertsRead, unitsRead, customersRead, propertiesRead, policiesRead, defaultsRead].find(isMissing);
  if (missing) return <NeedsAccess what="Alert policies" permission={missing.missing} />;
  const read = <T,>(r: T | { missing: string }): T => {
    if (isMissing(r)) throw new Error(`a read that is missing "${r.missing}" was shown the page`); // none is missing past the check above
    return r;
  };
  const [alerts, units, customers, properties, policies, defaults] = [read(alertsRead), read(unitsRead), read(customersRead), read(propertiesRead), read(policiesRead), read(defaultsRead)];
  const scoped = [...defaults.items, ...policies.items.filter((p) => p.kind !== "default_alert")];
  const i = i18nOf(display);
  const custOrg = customers.items.find((c) => c.id === q.customerId)?.organizationId;
  const custByOrg = new Map(customers.items.map((c) => [c.organizationId, c.id]));
  return (
    <AdminAlertsView
      live={{
        tab: sp.tab === "policies" ? "policies" : "alerts",
        scope: {
          customers: customers.items.map((c) => ({ id: c.id, name: c.name })),
          properties: properties.filter((p) => !custOrg || p.customerOrgId === custOrg).map((p) => ({ id: p.id, name: p.name })),
          units: units.items.filter((u) => (!custOrg || u.customerOrgId === custOrg) && (!q.propertyId || u.propertyId === q.propertyId)).map((u) => ({ id: u.id, name: u.displayName })),
        },
        scopeName: customers.items.find((c) => c.id === q.customerId)?.name ?? null,
        alerts: adminAlertRows(alerts.items, units.items, customers.items, scoped, now.getTime(), i),
        policies: policyRows(scoped, customers.items, members.items, i.t, units.items),
        pol: {
          policyId: one(sp.policyId) ?? null, creatingFor: one(sp.new) ?? null, from: one(sp.from) ?? null, canWrite: perms.has("alert.policy.write"), timezone: display.timeZone,
          customers: customers.items.filter((c) => c.status !== "inactive").map((c) => ({ id: c.id, name: c.name })),
          units: units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, name: u.displayName, customerId: custByOrg.get(u.customerOrgId) ?? null })),
        },
        q,
      }}
    />
  );
}
