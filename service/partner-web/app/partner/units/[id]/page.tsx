// /partner/units/[id] (FR-P04): in API mode the Server Component reads the unit, its alerts and the company's jobs
// on it through the DAL; outside the accepted Offer's access window the unit is not found (IR169). The Phase 1A demo
// keeps the fixture view.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode, coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { MockPartnerUnit } from "./_components/mock-unit";
import { PartnerUnitDetail, type ApiAlert, type ApiJob } from "./_components/unit-detail";

export default async function PartnerUnitPage({ params }: PageProps<"/partner/units/[id]">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <MockPartnerUnit id={id} />;
  let d: ApiUnitDetail;
  try {
    d = await coreOp<ApiUnitDetail>("units.get", { id });
  } catch (e) {
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.fieldErrors.id)) notFound();
    throw e;
  }
  const [alerts, jobs] = await Promise.all([
    coreOp<{ items: ApiAlert[] }>("alerts.list", { limit: 20, filters: { unitId: id } }),
    coreOp<{ items: ApiJob[] }>("jobs.list", { limit: 50, filters: { unitId: id } }),
  ]);
  return <PartnerUnitDetail d={d} alerts={alerts.items} jobs={jobs.items} />;
}
