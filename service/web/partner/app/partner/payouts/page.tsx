// /partner/payouts (FR-P10, SCR-P10): in API mode a Server Component reads the company's approved and paid
// statements, the selected one with its rate card and job labels, and the jobs still in review through the DAL;
// asking HQ about a line is a Server Action (payouts.query). URL keys: statementId, period, status. The Phase 1A
// demo keeps the fixture view.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { loadPayouts } from "./_lib/load";
import { PayoutsDemo } from "./_components/payouts-demo";
import { PayoutsView } from "./_components/payouts-view";

const str = (v: string | string[] | undefined) => (typeof v === "string" && v ? v : undefined);

export default async function PartnerPayoutsPage({ searchParams }: PageProps<"/partner/payouts">) {
  await connection();
  if (!apiMode()) return <PayoutsDemo />;
  const sp = await searchParams;
  const live = await loadPayouts({ statementId: str(sp.statementId), period: str(sp.period), status: str(sp.status) });
  return <PayoutsView live={live} />;
}
