// The shell of an app in API mode (IR241): who is signed in (session.get display and organization names) and the
// sidebar badges — the customer's unresolved critical / warning alerts (summaries.get alertCount, IR51), the
// contractor's offers waiting for an answer (summaries.get offerCount), HQ's unresolved critical / warning alerts
// (alerts.list totals) and everyone's unread notifications (notifications.list unreadOnly total, IR102). A count that
// cannot be read shows no badge; and the user's display language (preferences.get, IR258). Read by the role layouts on
// every render (refresh() after a write reads them again); connection() keeps those layouts from being prerendered at
// build time, when there is no API (IR248).
import "server-only";
import { connection } from "next/server";
import { apiMode, coreDisplay, coreIdentity, coreOp } from "@ac/web/lib/dal";
import type { Role, ShellLive } from "@ac/web/lib/nav";

const total = (op: string, input: unknown) => coreOp<{ total: number }>(op, input).then((p) => p.total);
const quiet = (p: Promise<number>) => p.catch(() => null);

export async function loadShell(role: Role): Promise<ShellLive | undefined> {
  await connection();
  if (!apiMode()) return undefined;
  const counts: Promise<[string, number | null]>[] = [quiet(total("notifications.list", { filters: { unreadOnly: true }, limit: 1 })).then((n) => ["/notifications", n])];
  if (role === "client") counts.push(quiet(coreOp<{ counts: { alertCount: number } }>("summaries.get", { kind: "customer", filters: {} }).then((s) => s.counts.alertCount)).then((n) => ["/customer/alerts", n]));
  if (role === "contractor") counts.push(quiet(coreOp<{ counts: { offerCount: number } }>("summaries.get", { kind: "partner", filters: {} }).then((s) => s.counts.offerCount)).then((n) => ["/partner/jobs", n]));
  if (role === "admin") {
    const parts = (["critical", "warning"] as const).flatMap((severity) => (["open", "acknowledged"] as const).map((status) => total("alerts.list", { filters: { severity, status }, limit: 1 })));
    counts.push(quiet(Promise.all(parts).then((ns) => ns.reduce((a, b) => a + b, 0))).then((n) => ["/admin/alerts", n]));
  }
  const [who, display, ...badges] = await Promise.all([coreIdentity().catch(() => ({ displayName: "", organizationName: "" })), coreDisplay(), ...counts]);
  return { user: who.displayName, organization: who.organizationName, badges: Object.fromEntries(badges.filter((b): b is [string, number] => b[1] !== null)), locale: display.locale };
}
