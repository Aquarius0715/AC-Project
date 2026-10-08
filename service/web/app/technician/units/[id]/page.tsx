// /technician/units/[id] (FR-T02, FR-T04): in API mode the Server Component reads the unit (technician:assigned,
// IR169/IR170), its open alerts, the jobs on it and the last 7 days of temperature through the DAL. External
// technicians before their work window see "Not started yet" (IR49(b)); units outside their scope are not found.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { Card, Page } from "@/components/ui";
import { apiMode, coreNow, coreOp, CoreError } from "@/lib/dal";
import type { ApiUnitDetail } from "@/lib/units";
import { TechUnitView, type ApiAlert, type ApiJob, type ApiMeasurement } from "./_components/unit-view";

const minute = (d: Date) => d.toISOString().slice(0, 16) + ":00Z";

export default async function TechnicianUnitPage({ params }: PageProps<"/technician/units/[id]">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <TechUnitView id={id} />;
  let d: ApiUnitDetail;
  try {
    d = await coreOp<ApiUnitDetail>("units.get", { id });
  } catch (e) {
    if (e instanceof CoreError && e.error.messageKey === "errors.assignment_not_started") {
      return (
        <Page className="max-w-xl">
          <Card title="Not started yet" sub="You can see this AC from the start time of your assignment. Open the job to check the scheduled window." />
        </Page>
      );
    }
    if (e instanceof CoreError && (e.error.code === "NOT_FOUND" || e.error.code === "FORBIDDEN" || e.error.fieldErrors.id)) notFound();
    throw e;
  }
  const now = await coreNow();
  const [alerts, jobs, series] = await Promise.all([
    coreOp<{ items: ApiAlert[] }>("alerts.list", { limit: 20, filters: { unitId: id } }),
    coreOp<{ items: ApiJob[] }>("jobs.list", { limit: 20, filters: { unitId: id } }),
    coreOp<{ items: ApiMeasurement[] }>("telemetry.series", {
      from: minute(new Date(now.getTime() - 7 * 86400e3)), to: minute(now), unitIds: [id], metric: "temperature",
      query: { limit: 100, sort: { field: "observedAt", direction: "desc" } },
    }),
  ]);
  return (
    <TechUnitView
      id={id}
      data={{
        d,
        alerts: alerts.items.filter((a) => a.status !== "resolved"),
        jobs: jobs.items.filter((j) => j.projection === "summary"),
        series: series.items,
        now: now.toISOString(),
      }}
    />
  );
}
