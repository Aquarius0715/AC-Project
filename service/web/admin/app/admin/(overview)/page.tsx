// /admin dashboard (FR-A01, DD-A01, SCR-A01, IR244): in API mode a Server Component reads admin.summary for the URL
// scope (customerId, propertyId) and period (today / 7d / 30d / custom from–to, SR17) and renders every section from
// that one result; customers.list and properties.list only fill the filter bar (hidden without asset.read). The Phase
// 1A demo keeps the fixture KPIs.
import { connection } from "next/server";
import { apiMode, coreAll, coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { periodKinds, periodRange, type PeriodKind } from "@ac/web/lib/clientEnergy";
import { type AdminSummary } from "@ac/web/lib/adminSummary";
import { AdminOverviewDemo } from "./_components/overview-demo";
import { AdminOverviewView } from "./_components/overview-view";

const quiet = <T,>(p: Promise<T>) => p.catch((e) => { if (e instanceof CoreError && (e.error.code === "FORBIDDEN" || e.error.code === "NOT_FOUND")) return null; throw e; });

export default async function AdminOverviewPage({ searchParams }: PageProps<"/admin">) {
  await connection();
  if (!apiMode()) return <AdminOverviewDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const kind = (periodKinds.find((k) => k === one("period")) ?? "today") as PeriodKind;
  const now = await coreNow();
  const period = periodRange(kind, now, { from: one("from"), to: one("to") });
  const customers = await quiet(coreAll<{ id: string; name: string }>("customers.list"));
  const customerId = customers?.some((c) => c.id === one("customerId")) ? one("customerId")! : null;
  const properties = customerId ? await quiet(coreAll<{ id: string; name: string }>("properties.list", { filters: { customerId } })) : null;
  const propertyId = properties?.some((p) => p.id === one("propertyId")) ? one("propertyId")! : null;
  const summary = period.error ? null : await coreOp<AdminSummary>("admin.summary", { from: period.from, to: period.to, ...(customerId ? { customerId } : {}), ...(propertyId ? { propertyId } : {}) });
  return (
    <AdminOverviewView live={{
      now: now.toISOString(), period: { kind, from: period.from, to: period.to, error: period.error ?? null, label: period.label }, custom: { from: one("from") ?? "", to: one("to") ?? "" },
      customers, customerId, properties, propertyId, summary,
    }} />
  );
}
