// /admin/billing/contracts (FR-A07, SCR-A07): in API mode a Server Component reads the contracts for the URL scope
// (customerId → unitId, plan-type chip kind), every current contract (to label units already on one and to count the
// plans of the scope, IR322), customers, properties and units through the DAL; the editor saves with a Server Action
// (actions.ts). Texts in the display language; contract periods are Kuala Lumpur days (IR300). The Phase 1A demo keeps
// the fixture rows.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreAllPage } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import type { ApiCustomer, ApiProperty } from "@ac/web/lib/billing";
import { contractRows, unitOptions, type ApiContractFull, type ApiUnitBrief, type PlanType } from "@ac/web/lib/contracts";
import { withAccess } from "@ac/web/lib/withAccess";
import { ContractsView } from "./_components/contracts-view";
import { queryReader } from "@ac/web/lib/searchParams";

const CONTRACTS_LIST = "contracts.list";

const plans: PlanType[] = ["rto", "general", "energy", "environment"];

// a role without the page's read permission sees which one it needs, not a failure (IR582)
export default async function AdminContractsPage(props: PageProps<"/admin/billing/contracts">) {
  return withAccess("Contracts", ["contract.read", "asset.read"], () => AdminContractsPageContent(props));
}

async function AdminContractsPageContent({ searchParams }: Readonly<PageProps<"/admin/billing/contracts">>) {
  await connection();
  if (!apiMode()) return <ContractsView />;
  const sp = await searchParams;
  const one = queryReader(sp);
  const kind = plans.find((p) => p === one("kind"));
  const scope = { customerId: one("customerId"), unitId: one("unitId"), kind };
  const where = { ...(scope.customerId && { customerId: scope.customerId }), ...(scope.unitId && { unitId: scope.unitId }) };
  const [display, all, inScope, scoped, customers, properties, units] = await Promise.all([
    coreDisplay(),
    coreAll<ApiContractFull>(CONTRACTS_LIST),
    Object.keys(where).length > 0 ? coreAll<ApiContractFull>(CONTRACTS_LIST, { filters: where }) : null, // the plan counts of the scope
    kind ? coreAll<ApiContractFull>(CONTRACTS_LIST, { filters: { ...where, kind } }) : null,
    coreAllPage<ApiCustomer>("customers.list"),
    coreAllPage<ApiProperty>("properties.list"),
    coreAllPage<ApiUnitBrief>("units.list"),
  ]);
  const ofScope = inScope ?? all;
  const countOf = (plan: PlanType) => ofScope.filter((k) => k.planType === plan).length;
  const counts: Record<PlanType | "all", number> = { all: ofScope.length, rto: countOf("rto"), general: countOf("general"), energy: countOf("energy"), environment: countOf("environment") };
  const rows = contractRows(scoped ?? ofScope, customers.items, i18nOf(display));
  return (
    <ContractsView
      live={{
        scope, rows, counts, selectedId: rows.find((r) => r.id === one("contractId"))?.id ?? rows[0]?.id ?? null,
        customers: customers.items.filter((c) => c.status === "active").map((c) => ({ id: c.id, name: c.name })),
        units: unitOptions(units.items, customers.items, properties.items, all),
      }}
    />
  );
}
