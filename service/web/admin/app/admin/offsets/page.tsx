// /admin/offsets (FR-A15, SCR-A15): in API mode a Server Component reads offset records (offsets.list) for the URL
// filters customerId, status and created (the last 7, 30 or 90 days of the business clock, or all; 30 by default,
// Figma Admin 106:9, IR324), customers and units; the recordId detail comes from the list item (attempts, references,
// event history), and a recordId the list does not hold says so. Quotes, requests and the
// simulated purchase / retirement / failure / retry are Server Actions (actions.ts). Texts in the display language; the
// attempt and event times are formatted here, in the display time zone (IR282, IR298). The Phase 1A demo keeps fixtures.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreNow, coreOp, corePermissions } from "@ac/web/lib/dal";
import type { ApiCustomer } from "@ac/web/lib/billing";
import { i18nOf } from "@ac/web/lib/i18n";
import { recordRows, type ApiOffsetRecord } from "@ac/web/lib/offsets";
import { OffsetsDemo } from "./_components/offsets-demo";
import { OffsetsView } from "./_components/offsets-view";

type Page<T> = { items: T[] };
type Unit = { id: string; displayName: string; customerOrgId: string; archived: boolean };
const STATES = ["demo_requested", "demo_purchased", "demo_retired", "failed"] as const;
const CREATED = { "7d": 7, "30d": 30, "90d": 90, all: null } as const;

export default async function AdminOffsetsPage({ searchParams }: PageProps<"/admin/offsets">) {
  await connection();
  if (!apiMode()) return <OffsetsDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const tab = one("tab") === "market" ? "market" : "records";
  const customerId = one("customerId");
  const status = STATES.find((x) => x === one("status"));
  const created = (Object.keys(CREATED) as (keyof typeof CREATED)[]).find((k) => k === one("created")) ?? "30d";
  const now = await coreNow();
  const days = CREATED[created];
  const filters = { ...(customerId && { customerId }), ...(status && { status }), ...(days !== null && { from: new Date(now.getTime() - days * 86_400_000).toISOString() }) };
  const [perms, display, records, customers, units] = await Promise.all([
    corePermissions(), coreDisplay(),
    coreOp<Page<ApiOffsetRecord>>("offsets.list", { limit: 100, filters }),
    coreOp<Page<ApiCustomer>>("customers.list", { limit: 100 }),
    coreOp<Page<Unit>>("units.list", { limit: 100 }),
  ]);
  const customerOfOrg = new Map(customers.items.map((c) => [c.organizationId, c.id]));
  const names = customers.items.map((c) => ({ id: c.id, name: c.name }));
  const rows = recordRows(records.items, names, i18nOf(display));
  const wanted = one("recordId");
  const selectedId = wanted ? rows.find((r) => r.id === wanted)?.id ?? null : rows[0]?.id ?? null;
  return (
    <OffsetsView live={{
      tab, canWrite: perms.has("offset.write"), rows, selectedId, recordMissing: !!wanted && !selectedId, scope: { customerId, status, created }, customers: names,
      units: units.items.filter((u) => !u.archived).map((u) => ({ id: u.id, label: u.displayName, customerId: customerOfOrg.get(u.customerOrgId) ?? "" })),
    }} />
  );
}
