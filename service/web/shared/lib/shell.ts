// The shell of an app in API mode (IR241): who is signed in (session.get display and organization names) and the
// sidebar badges — the customer's unresolved critical / warning alerts (summaries.get alertCount, IR51), the
// contractor's offers waiting for an answer (summaries.get offerCount), HQ's unresolved critical / warning alerts
// (alerts.list totals) and everyone's unread notifications (notifications.list unreadOnly total, IR102). A count that
// cannot be read shows no badge; and the user's display language and time zone (preferences.get, IR258). Read by the role layouts on
// every render (refresh() after a write reads them again); connection() keeps those layouts from being prerendered at
// build time, when there is no API (IR248).
import "server-only";
import { connection } from "next/server";
import { apiMode, coreDisplay, coreIdentity, coreOp } from "@ac/web/lib/dal";
import type { Role, ShellLive } from "@ac/web/lib/nav";
import type { OpCall } from "@ac/web/lib/opTypes";
import { orNull } from "@ac/web/lib/promises";

const total = async (...call: OpCall) => { const p = await coreOp<{ total: number }>(...call); return p.total; };
/** badge is a sidebar count for `href`, or null when the count cannot be read. */
const badge = async (href: string, count: Promise<number>): Promise<[string, number | null]> => [href, await orNull(count)];
const alertCount = async () => {
  const summary = await coreOp<{ counts: { alertCount: number } }>("summaries.get", { kind: "customer", filters: {} });
  return summary.counts.alertCount;
};
const offerCount = async () => {
  const summary = await coreOp<{ counts: { offerCount: number } }>("summaries.get", { kind: "partner", filters: {} });
  return summary.counts.offerCount;
};
const openAlerts = async () => {
  const parts = (["critical", "warning"] as const).flatMap((severity) => (["open", "acknowledged"] as const).map((status) => total("alerts.list", { filters: { severity, status }, limit: 1 })));
  const totals = await Promise.all(parts);
  return totals.reduce((a, b) => a + b, 0);
};
const identityOrBlank = async () => (await orNull(coreIdentity())) ?? { displayName: "", organizationName: "" };

export async function loadShell(role: Role): Promise<ShellLive | undefined> {
  await connection();
  if (!apiMode()) return undefined;
  const counts: Promise<[string, number | null]>[] = [badge("/notifications", total("notifications.list", { filters: { unreadOnly: true }, limit: 1 }))];
  switch (role) {
    case "client": { counts.push(badge("/customer/alerts", alertCount())); break; }
    case "contractor": { counts.push(badge("/partner/jobs", offerCount())); break; }
    case "admin": { counts.push(badge("/admin/alerts", openAlerts())); break; }
    case "technician": { break; } // the technician shell counts the notifications only
  }
  const [who, display, ...badges] = await Promise.all([identityOrBlank(), coreDisplay(), ...counts]);
  return { user: who.displayName, organization: who.organizationName, badges: Object.fromEntries(badges.filter((b): b is [string, number] => b[1] !== null)), display };
}
