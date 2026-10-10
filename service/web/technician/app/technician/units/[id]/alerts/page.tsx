// /technician/units/[id]/alerts (FR-T07, SCR-T07): in API mode the Server Component reads the unit's alerts with the
// condition each was raised by, the unit and the URL's job through the DAL (?jobId=, ?alertId= the alert to open);
// acknowledge and resolve are Server Actions. Before the work window the page says when it opens (IR76). The Phase 1A
// demo keeps the fixture.
import { connection } from "next/server";
import { Card, LinkBtn, Page } from "@ac/web/components/ui";
import { apiMode } from "@ac/web/lib/dal";
import { AlertEvidenceView } from "./_components/alert-evidence";
import { MockAlertEvidence } from "./_components/mock-evidence";
import { loadAlerts } from "./_lib/load";

export default async function TechnicianAlertsPage({ params, searchParams }: PageProps<"/technician/units/[id]/alerts">) {
  const { id } = await params;
  const sp = await searchParams;
  await connection();
  if (!apiMode()) return <MockAlertEvidence id={id} />;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  const live = await loadAlerts(id, { jobId: one(sp.jobId), alertId: one(sp.alertId) });
  if (live.kind === "unavailable") {
    return (
      <Page className="max-w-xl">
        <Card title={live.title} sub={live.text}><LinkBtn href={live.unitHref} size="sm">{live.back}</LinkBtn></Card>
      </Page>
    );
  }
  return <AlertEvidenceView live={live} />;
}
