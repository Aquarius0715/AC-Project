// /admin/billing/contracts (FR-A07, SCR-A07): in API mode a Server Component reads the contracts for the URL scope
// (customerId → unitId, plan-type chip kind), every current contract (to label units already on one), customers,
// properties and units through the DAL; the editor saves with a Server Action (actions.ts). Texts in the display
// language; contract periods are Kuala Lumpur days (IR300). The Phase 1A demo keeps the fixture rows.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreOp } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiCustomer, ApiProperty } from "@ac/web/lib/billing";
import { contractRows, unitOptions, type ApiContractFull, type ApiUnitBrief, type PlanType } from "@ac/web/lib/contracts";
import { ContractsView } from "./_components/contracts-view";

type Page<T> = { items: T[] };
const plans: PlanType[] = ["rto", "general", "energy", "environment"];

export default async function AdminContractsPage({ searchParams }: PageProps<"/admin/billing/contracts">) {
  await connection();
  if (!apiMode()) return <ContractsView />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const kind = plans.find((p) => p === one("kind"));
  const scope = { customerId: one("customerId"), unitId: one("unitId"), kind };
  const filters = { ...(scope.customerId && { customerId: scope.customerId }), ...(scope.unitId && { unitId: scope.unitId }), ...(kind && { kind }) };
  const [display, scoped, all, customers, properties, units] = await Promise.all([
    coreDisplay(),
    coreOp<Page<ApiContractFull>>("contracts.list", { limit: 100, filters }),
    coreOp<Page<ApiContractFull>>("contracts.list", { limit: 100 }),
    coreOp<Page<ApiCustomer>>("customers.list", { limit: 100 }),
    coreOp<Page<ApiProperty>>("properties.list", { limit: 100 }),
    coreOp<Page<ApiUnitBrief>>("units.list", { limit: 100 }),
  ]);
  const rows = contractRows(scoped.items, customers.items, i18nOf(display));
  return (
    <ContractsView
      live={{
        scope, rows, selectedId: rows.find((r) => r.id === one("contractId"))?.id ?? rows[0]?.id ?? null,
        customers: customers.items.filter((c) => c.status === "active").map((c) => ({ id: c.id, name: c.name })),
        units: unitOptions(units.items, customers.items, properties.items, all.items),
      }}
    />
  );
}
