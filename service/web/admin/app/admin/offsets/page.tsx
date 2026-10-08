// /admin/offsets (FR-A15, SCR-A15): in API mode a Server Component reads offset records (offsets.list), customers and
// units; the recordId detail comes from the list item (attempts, references, event history). Quotes, requests and the
// simulated purchase / retirement / failure / retry are Server Actions (actions.ts). The Phase 1A demo keeps fixtures.
import { connection } from "next/server";
import { apiMode, coreOp, corePermissions } from "@ac/web/lib/dal";
import type { ApiCustomer } from "@ac/web/lib/billing";
import { recordRows, type ApiOffsetRecord } from "@ac/web/lib/offsets";
import { OffsetsDemo } from "./_components/offsets-demo";
import { OffsetsView } from "./_components/offsets-view";

type Page<T> = { items: T[] };
type Unit = { id: string; displayName: string; customerOrgId: string; archived: boolean };

export default async function AdminOffsetsPage({ searchParams }: PageProps<"/admin/offsets">) {
  await connection();
  if (!apiMode()) return <OffsetsDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const tab = one("tab") === "market" ? "market" : "records";
  const [perms, records, customers, units] = await Promise.all([
    corePermissions(),
    coreOp<Page<ApiOffsetRecord>>("offsets.list", { limit: 100 }),
    coreOp<Page<ApiCustomer>>("customers.list", { limit: 100 }),
    coreOp<Page<Unit>>("units.list", { limit: 100 }),
  ]);
  const customerOfOrg = new Map(customers.items.map((c) => [c.organizationId, c.id]));
  const names = customers.items.map((c) => ({ id: c.id, name: c.name }));
  const rows = recordRows(records.items, names);
  return (
    <OffsetsView live={{
      tab, canWrite: perms.has("offset.write"), rows, selectedId: rows.find((r) => r.id === one("recordId"))?.id ?? rows[0]?.id ?? null, customers: names,
      units: units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, label: u.displayName, customerId: customerOfOrg.get(u.customerOrgId) ?? "" })),
    }} />
  );
}
