// /customer/users (FR-C19, SCR-C19): in API mode a Server Component checks the session's client role (session.get) —
// members get the shared Page unavailable view (FORBIDDEN, IR112) — and reads clientUsers.list for the owner's customer.
// Inviting a member and resending an invite are Server Actions (actions.ts); role changes, disabling, password resets
// and removal stay with HQ (DD-A17). Texts and times in the user's display language and time zone (IR267). The Phase 1A
// demo keeps the local user store.
import { connection } from "next/server";
import { apiMode, coreAll, coreDisplay, corePrincipal } from "@ac/web/lib/dal";
import { i18nOf } from "@ac/web/lib/i18n";
import { clientUserRows, type ApiClientUser } from "@ac/web/lib/assets";
import { UsersDemo } from "./_components/users-demo";
import { UsersView } from "./_components/users-view";

export default async function CustomerUsersPage() {
  await connection();
  if (!apiMode()) return <UsersDemo />;
  const me = await corePrincipal();
  if (me.clientRole !== "owner") return <UsersView live={null} />;
  const [users, display] = await Promise.all([coreAll<ApiClientUser>("clientUsers.list"), coreDisplay()]);
  const i = i18nOf(display);
  const customerId = users.find((u) => u.membershipId === me.membershipId)?.customerId ?? users[0]?.customerId ?? "";
  return <UsersView live={{ customerId, rows: clientUserRows(users, (m) => (m === me.membershipId ? i.t("you") : "HQ"), me.membershipId, i), emails: users.map((u) => u.email) }} />;
}
