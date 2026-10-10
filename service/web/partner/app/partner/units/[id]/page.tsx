// /partner/units/[id] (FR-P04, FR-P08, DD-P04): in API mode the Server Component reads the unit, its alerts, the
// company's jobs on it and its last 24 hours through the DAL (./_lib/load.ts); outside the accepted offer's access
// window the unit is not found (IR169) — or, with ?jobId=, that job's history snapshot is shown. The Phase 1A demo
// keeps the fixture view.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { apiMode } from "@ac/web/lib/dal";
import { MockPartnerUnit } from "./_components/mock-unit";
import { PartnerUnitDetail } from "./_components/unit-detail";
import { loadUnit } from "./_lib/load";

export default async function PartnerUnitPage({ params, searchParams }: PageProps<"/partner/units/[id]">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <MockPartnerUnit id={id} />;
  const sp = await searchParams;
  const jobId = typeof sp.jobId === "string" && sp.jobId ? sp.jobId : undefined;
  const live = await loadUnit(id, jobId);
  if (!live) notFound();
  return <PartnerUnitDetail live={live} />;
}
