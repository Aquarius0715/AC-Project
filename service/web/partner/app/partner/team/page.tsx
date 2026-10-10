// /partner/team (FR-P06, FR-P09, DD-P06, DD-P09, SCR-P06): in API mode a Server Component reads the company's
// technicians, the capacity of the chosen date's week, their booked jobs and the certificates through the DAL
// (./_lib/load.ts) for the URL's date, qualification, activeOnly, orgId, membershipId, status and expiringWithinDays;
// unavailable days, certificates and training requests are Server Actions (./actions.ts). The Phase 1A demo keeps
// the fixture technicians.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { MockTeam } from "./_components/mock-team";
import { TeamView } from "./_components/team-view";
import { loadTeam } from "./_lib/load";

export default async function PartnerTeamPage({ searchParams }: PageProps<"/partner/team">) {
  await connection();
  if (!apiMode()) return <MockTeam />;
  const sp = await searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  const live = await loadTeam({
    date: one(sp.date), qualification: one(sp.qualification), activeOnly: one(sp.activeOnly), orgId: one(sp.orgId),
    membershipId: one(sp.membershipId), status: one(sp.status), expiringWithinDays: one(sp.expiringWithinDays),
  });
  return <TeamView live={live} />;
}
