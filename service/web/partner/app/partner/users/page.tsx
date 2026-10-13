// /partner/users (FR-P11, DD-P11, SCR-P11): in API mode a Server Component reads partnerUsers.list for the URL's
// status and the signed-in membership (session.get), so the own row has no actions; the writes are Server Actions
// (actions.ts). A technician session gets the shared Page unavailable view (FORBIDDEN). Texts and times in the user's
// display language and time zone (IR267). The Phase 1A demo keeps a local store (partnerUsersDemo).
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, coreIdentity, coreNow, corePrincipal, CoreError } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { partnerUserRows, statusFilter, type ApiPartnerUser } from "@ac/web/lib/partnerUsers";
import { UsersDemo } from "./_components/users-demo";
import { UsersLive } from "./_components/users-live";

export default async function PartnerUsersPage({ searchParams }: PageProps<"/partner/users">) {
  await connection();
  if (!apiMode()) return <UsersDemo />;
  const sp = await searchParams;
  const status = statusFilter(Array.isArray(sp.status) ? sp.status[0] : sp.status);
  const [all, me, display, identity, now] = await Promise.all([
    coreAll<ApiPartnerUser>("partnerUsers.list").catch((e) => { if (e instanceof CoreError && e.error.code === "FORBIDDEN") return null; throw e; }),
    corePrincipal(), coreDisplay(), coreIdentity(), coreNow(),
  ]);
  if (!all) return <UsersLive live={null} />;
  const rows = partnerUserRows(all, me.membershipId, i18nOf(display), now);
  return <UsersLive live={{ company: identity.organizationName, status, rows: status ? rows.filter((r) => r.status === status) : rows, all: all.map((u) => ({ id: u.id, email: u.email })) }} />;
}
