// The reads of /customer/alerts (FR-C08, DD-C08, IR242) through the DAL: the customer's alerts (alerts.list, severity
// filter critical / warning — all omits it, IR74), the signed-in membership's notifications about them
// (notifications.list: their read state, DD-C08), the unresolved alert count (summaries.get alertCount, IR51) and the
// units with their places (units.list, properties.list, spaces.list); for the Alert policies tab the default policy
// with this customer's rule settings and the customer's own policies (policies.list kind default_alert / alert,
// IR243) and the Preferences time zone (preferences.get). URL keys: tab (policies), severity, unreadOnly, unitId,
// policyId (new or an own policy: the editor). Texts and times in the user's display language and time zone (IR261).
import "server-only";
import { coreAll, coreDisplay, coreNow, coreOp, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { unitPlaces } from "@ac/web/lib/clientBilling";
import { inboxAlerts, type AlertNote, type ApiAlert } from "@ac/web/lib/alerts";
import { defaultRuleRows, policyCards, policyForm, type ApiAlertPolicy, type ApiDefaultPolicy } from "@ac/web/lib/customerPolicies";

type Unit = { id: string; displayName: string; propertyId: string; spaceId: string | null };
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export async function loadAlerts(sp: Record<string, string | string[] | undefined>) {
  const tab = one(sp.tab) === "policies" ? "policies" as const : "alerts" as const;
  const severity = one(sp.severity) === "critical" || one(sp.severity) === "warning" ? one(sp.severity) as "critical" | "warning" : null;
  const unreadOnly = one(sp.unreadOnly) === "true";
  const unitId = one(sp.unitId);
  const [now, me, alerts, notes, summary, units, properties, spaces, policies, defaults, prefs, display] = await Promise.all([
    coreNow(), corePrincipal(),
    coreAll<ApiAlert>("alerts.list", severity || unitId ? { filters: { ...(severity ? { severity } : {}), ...(unitId ? { unitId } : {}) } } : {}), coreAll<AlertNote>("notifications.list"),
    coreOp<{ counts: { alertCount: number } }>("summaries.get", { kind: "customer", filters: {} }),
    coreAll<Unit>("units.list"), coreAll<{ id: string; name: string }>("properties.list"), coreAll<{ id: string; name: string; parentSpaceId: string | null }>("spaces.list"),
    coreAll<ApiAlertPolicy>("policies.list", { filters: { kind: "alert" } }), coreAll<ApiDefaultPolicy>("policies.list", { filters: { kind: "default_alert" } }),
    coreOp<{ timezone: string }>("preferences.get", {}).catch(() => null), coreDisplay(),
  ]);
  const i = i18nOf(display);
  const place = unitPlaces(units, properties, spaces);
  const all = inboxAlerts(alerts, notes, (id) => place.get(id), i);
  const def = defaults[0] ?? null;
  const pid = one(sp.policyId);
  const editing = pid === "new" ? null : policies.find((p) => p.id === pid) ?? null;
  return {
    now: now.toISOString(), display, tab, severity, unreadOnly, unitId: units.some((u) => u.id === unitId) ? unitId! : null,
    units: units.map((u) => ({ id: u.id, name: u.displayName, place: place.get(u.id)?.place ?? "" })).sort((a, b) => a.name.localeCompare(b.name)),
    alertCount: summary.counts.alertCount, total: all.length, unread: all.filter((a) => a.unread.length).length,
    rows: unreadOnly ? all.filter((a) => a.unread.length) : all,
    policies: {
      count: policies.length + (def ? 1 : 0), owner: me.clientRole === "owner", customerId: me.customerId, membershipId: me.membershipId, timezone: prefs?.timezone ?? "Asia/Kuala_Lumpur",
      defaults: def ? { id: def.id, ...defaultRuleRows(def, me.customerId, me.membershipId, i) } : null,
      cards: policyCards(policies, (id) => place.get(id)?.name, i.t), list: policies,
      // the editor (policyId=new or an own policy's id, Figma 06f); an unknown id shows the list
      editor: pid === "new" ? { policy: null, form: policyForm(null), attached: [] as string[] } : editing ? { policy: editing, form: policyForm(editing), attached: editing.unitIds.map((id) => place.get(id)?.name ?? "AC") } : null,
    },
  };
}

export type AlertsLive = Awaited<ReturnType<typeof loadAlerts>>;
