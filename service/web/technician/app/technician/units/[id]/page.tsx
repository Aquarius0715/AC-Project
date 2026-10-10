// /technician/units/[id] (FR-T02, FR-T03, SCR-T02): in API mode the Server Component reads the unit, its alerts, the
// technician's jobs on it, the chosen metric over the period and the period's events through the DAL (?jobId= the
// job the page is opened for, ?metric= the Monitoring tab's metric, ?period=1h|7d). Before the work window an external
// technician sees when it opens (IR76); a unit outside the assignments is not found (IR169). The Phase 1A demo keeps
// the fixture.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { Card, LinkBtn, Page } from "@ac/web/components/ui";
import { apiMode } from "@ac/web/lib/dal";
import { TechUnitDemo } from "./_components/unit-demo";
import { TechUnitView } from "./_components/unit-view";
import { loadTechUnit } from "./_lib/load";

export default async function TechnicianUnitPage({ params, searchParams }: PageProps<"/technician/units/[id]">) {
  const { id } = await params;
  const sp = await searchParams;
  await connection();
  if (!apiMode()) return <TechUnitDemo id={id} />;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  const live = await loadTechUnit(id, { jobId: one(sp.jobId), metric: one(sp.metric), period: one(sp.period) });
  if (!live) notFound();
  if (live.kind === "not_started") {
    return (
      <Page className="max-w-xl">
        <Card title={live.title} sub={live.text}>{live.job && <LinkBtn href={live.job.href} size="sm" variant="primary">{live.job.label}</LinkBtn>}</Card>
      </Page>
    );
  }
  return <TechUnitView live={live} />;
}
