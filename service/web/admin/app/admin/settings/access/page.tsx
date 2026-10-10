// /admin/settings/access (FR-A03, SCR-A03): in API mode a Server Component reads members.list (HQ, contractor and
// technician memberships; clients are managed in Customers & units › Users) with the URL filters role, status and
// organizationId, organizations.list, and the scope targets (customer organizations, properties, units);
// membershipId=new opens a blank form for another membership of an existing user, and a membershipId the list does not
// hold says so instead of opening another one (IR321). Saving and revoking are a Server Action (actions.ts). Texts in
// the display language; the valid periods are formatted here, in the display time zone (IR282, IR302). The Phase 1A
// demo keeps the fixture users.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreNow, corePermissions, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { memberRows, type ApiMember, type ApiOrganization } from "@ac/web/lib/members";
import { AccessDemo } from "./_components/access-demo";
import { AccessView } from "./_components/access-view";
import type { Role } from "@ac/web/lib/contracts.gen";


export default async function AdminAccessPage({ searchParams }: PageProps<"/admin/settings/access">) {
  await connection();
  if (!apiMode()) return <AccessDemo />;
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  const role = one("role") === "admin" || one("role") === "contractor" || one("role") === "technician" ? one("role") : undefined;
  const status = one("status") === "active" || one("status") === "inactive" ? one("status") : undefined;
  const organizationId = one("organizationId");
  const filters = { ...(role && { role: role as Role }), ...(status === "active" && { activeOnly: true }), ...(organizationId && { organizationId }) };
  const filtered = Object.keys(filters).length > 0;
  const [now, perms, me, display, members, everyone, orgs, properties, units] = await Promise.all([
    coreNow(), corePermissions(), corePrincipal(), coreDisplay(),
    coreAll<ApiMember>("members.list", { filters }),
    filtered ? coreAll<ApiMember>("members.list") : Promise.resolve(null), // a new membership may be for any user, whatever the filters
    coreAll<ApiOrganization>("organizations.list"),
    coreAll<{ id: string; name: string }>("properties.list"),
    coreAll<{ id: string; displayName: string }>("units.list"),
  ]);
  const rows = memberRows(members, orgs, now, i18nOf(display)).filter((r) => status !== "inactive" || !r.active);
  const users = [...new Map((everyone ?? members).filter((m) => m.role !== "client").map((m) => [m.userId, m.displayName])).entries()].map(([id, name]) => ({ id, name }));
  const wanted = one("membershipId");
  const selected = wanted === "new" ? "new" : wanted ? rows.find((r) => r.id === wanted)?.id ?? "missing" : rows[0]?.id ?? "new";
  return (
    <AccessView live={{
      now: now.toISOString(), canWrite: perms.has("identity.write"), selfUserId: me.userId, tenantId: me.tenantId, scope: { role, status, organizationId }, rows, selected, users,
      orgs, properties, units: units.map((u) => ({ id: u.id, name: u.displayName })),
    }} />
  );
}
