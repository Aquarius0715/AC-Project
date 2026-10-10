// /admin/settings/access (FR-A03, SCR-A03): in API mode a Server Component reads members.list (HQ, contractor and
// technician memberships; clients are managed in Customers & units › Users) with the URL filters role and status,
// organizations.list, and the scope targets (customer organizations, properties, units); membershipId=new opens a
// blank form for another membership of an existing user. Saving and revoking are a Server Action (actions.ts). Texts in
// the display language; the valid periods are formatted here, in the display time zone (IR282, IR302). The Phase 1A
// demo keeps the fixture users.
import { connection } from "next/server";
import { apiMode, coreDisplay, coreNow, coreOp, corePermissions, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { memberRows, type ApiMember, type ApiOrganization } from "@ac/web/lib/members";
import { AccessDemo } from "./_components/access-demo";
import { AccessView } from "./_components/access-view";
import type { Role } from "@ac/web/lib/contracts.gen";

type Page<T> = { items: T[] };

export default async function AdminAccessPage({ searchParams }: PageProps<"/admin/settings/access">) {
  await connection();
  if (!apiMode()) return <AccessDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const role = one("role") === "admin" || one("role") === "contractor" || one("role") === "technician" ? one("role") : undefined;
  const status = one("status") === "active" || one("status") === "inactive" ? one("status") : undefined;
  const [now, perms, me, display, members, orgs, properties, units] = await Promise.all([
    coreNow(), corePermissions(), corePrincipal(), coreDisplay(),
    coreOp<Page<ApiMember>>("members.list", { limit: 100, filters: { ...(role && { role: role as Role }), ...(status === "active" && { activeOnly: true }) } }),
    coreOp<Page<ApiOrganization>>("organizations.list", { limit: 100 }),
    coreOp<Page<{ id: string; name: string }>>("properties.list", { limit: 100 }),
    coreOp<Page<{ id: string; displayName: string }>>("units.list", { limit: 100 }),
  ]);
  const rows = memberRows(members.items, orgs.items, now, i18nOf(display)).filter((r) => status !== "inactive" || !r.active);
  const users = [...new Map(members.items.filter((m) => m.role !== "client").map((m) => [m.userId, m.displayName])).entries()].map(([id, name]) => ({ id, name }));
  const selected = one("membershipId") === "new" ? "new" : rows.find((r) => r.id === one("membershipId"))?.id ?? rows[0]?.id ?? "new";
  return (
    <AccessView live={{
      now: now.toISOString(), canWrite: perms.has("identity.write"), selfUserId: me.userId, tenantId: me.tenantId, scope: { role, status }, rows, selected, users,
      orgs: orgs.items, properties: properties.items, units: units.items.map((u) => ({ id: u.id, name: u.displayName })),
    }} />
  );
}
