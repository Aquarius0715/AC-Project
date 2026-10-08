// /partner/team (FR-P team): in API mode the Server Component reads the company's technicians (members.list,
// IR172 displayName) and members.capacity for each day of the current week (Monday start, Core API business clock)
// through the DAL. The Phase 1A demo keeps the fixture technicians.
import { connection } from "next/server";
import { apiMode, coreNow, coreOp } from "@ac/web/lib/dal";
import { MockTeam } from "./_components/mock-team";
import { TeamView, type ApiCapacity, type ApiMember } from "./_components/team-view";

const day = 86400e3;

export default async function PartnerTeamPage() {
  await connection();
  if (!apiMode()) return <MockTeam />;
  const now = await coreNow();
  const monday = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()) - ((now.getUTCDay() + 6) % 7) * day;
  const dates = Array.from({ length: 7 }, (_, i) => new Date(monday + i * day).toISOString().slice(0, 10));
  const [members, ...week] = await Promise.all([
    coreOp<{ items: ApiMember[] }>("members.list", { limit: 100 }),
    ...dates.map((date) => coreOp<{ items: ApiCapacity[] }>("members.capacity", { date, query: { limit: 100 } })),
  ]);
  const capacity = Object.fromEntries(dates.map((d, i) => [d, (week[i] as { items: ApiCapacity[] }).items]));
  return <TeamView data={{ members: (members as { items: ApiMember[] }).items.filter((m) => m.role === "technician"), dates, capacity, now: now.toISOString() }} />;
}
