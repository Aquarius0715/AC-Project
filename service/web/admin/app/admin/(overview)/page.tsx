// /admin dashboard (FR-A01, DD-A01, SCR-A01, IR244): in API mode a Server Component reads admin.summary for the URL
// scope (customerId, propertyId) and period (today / 7d / 30d / custom from–to, SR17) and renders every section from
// that one result; customers.list and properties.list only fill the filter bar (hidden without asset.read). Texts in the
// user's display language; the as-of time and the period are formatted here (IR289). The Phase 1A demo keeps the
// fixture KPIs.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { periodKinds, periodRange, type PeriodKind } from "@ac/web/lib/clientEnergy";
import { asOfText, type AdminSummary } from "@ac/web/lib/adminSummary";
import { NeedsAccess } from "@ac/web/components/NeedsAccess";
import { AdminOverviewDemo } from "./_components/overview-demo";
import { AdminOverviewView } from "./_components/overview-view";

const quiet = <T,>(p: Promise<T>) => p.catch((e) => { if (e instanceof CoreError && (e.error.code === "FORBIDDEN" || e.error.code === "NOT_FOUND")) return null; throw e; });

export default async function AdminOverviewPage({ searchParams }: PageProps<"/admin">) {
  await connection();
  if (!apiMode()) return <AdminOverviewDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const kind = (periodKinds.find((k) => k === one("period")) ?? "today") as PeriodKind;
  const [now, display] = await Promise.all([coreNow(), coreDisplay()]);
  const i = i18nOf(display);
  const period = periodRange(kind, now, { from: one("from"), to: one("to") }, i);
  const customers = await quiet(coreAll<{ id: string; name: string }>("customers.list"));
  const customerId = customers?.some((c) => c.id === one("customerId")) ? one("customerId")! : null;
  const properties = customerId ? await quiet(coreAll<{ id: string; name: string }>("properties.list", { filters: { customerId } })) : null;
  const propertyId = properties?.some((p) => p.id === one("propertyId")) ? one("propertyId")! : null;
  // what waits for HQ whatever its requested time — requests to book, offers declined or expired, follow-ups to classify
  // (DEC-90, IR611); a role without job.read sees no such tile
  const scope = { ...(customerId ? { customerId } : {}), ...(propertyId ? { propertyId } : {}) };
  const waitingPage = quiet(coreOp<{ total: number }>("jobs.list", { limit: 1, filters: { status: "requested", proposalPending: false, ...scope } }));
  let summary: AdminSummary | null = null;
  if (!period.error) {
    try {
      summary = await coreOp<AdminSummary>("admin.summary", { from: period.from, to: period.to, ...(customerId ? { customerId } : {}), ...(propertyId ? { propertyId } : {}) });
    } catch (e) {
      // a role without dashboard.read: what it needs, not a failure with Try again (IR581)
      if (e instanceof CoreError && e.error.code === "FORBIDDEN") return <NeedsAccess what="Overview" permission="dashboard.read" />;
      throw e;
    }
  }
  const waiting = (await waitingPage)?.total ?? null;
  return (
    <AdminOverviewView live={{
      period: { kind, from: period.from, to: period.to, error: period.error ?? null, label: period.label }, custom: { from: one("from") ?? "", to: one("to") ?? "" },
      customers, customerId, properties, propertyId, summary, asOf: summary ? asOfText(summary.asOf ?? now.toISOString(), period.label, i) : null, waiting,
    }} />
  );
}
