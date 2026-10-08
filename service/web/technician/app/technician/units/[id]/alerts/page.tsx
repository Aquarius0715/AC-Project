// /technician/units/[id]/alerts (FR-T07): in API mode the Server Component reads the unit's alerts through the DAL
// (equipment read, IR49(b)/IR170); acknowledge and resolve are Server Actions. The Phase 1A demo keeps the fixture.
import { connection } from "next/server";
import { Card, Page } from "@ac/web/components/ui";
import { apiMode, coreOp, CoreError } from "@ac/web/lib/dal";
import { AlertEvidenceView, type ApiAlert } from "./_components/alert-evidence";
import { MockAlertEvidence } from "./_components/mock-evidence";

const unavailable: Record<string, string> = {
  "errors.assignment_not_started": "Your assignment has not started yet.",
  "error.notFound": "This AC is not in your assignments.",
};

export default async function TechnicianAlertsPage({ params }: PageProps<"/technician/units/[id]/alerts">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <MockAlertEvidence id={id} />;
  let alerts: ApiAlert[];
  try {
    alerts = (await coreOp<{ items: ApiAlert[] }>("alerts.list", { limit: 50, filters: { unitId: id } })).items;
  } catch (e) {
    if (!(e instanceof CoreError) || e.status >= 500) throw e; // error.tsx
    return <Unavailable messageKey={e.error.messageKey} code={e.error.code} />;
  }
  return <AlertEvidenceView unitId={id} alerts={alerts} />;
}

function Unavailable({ messageKey, code }: { messageKey: string; code: string }) {
  return (
    <Page className="max-w-xl">
      <Card title="Alerts aren’t available" sub={unavailable[messageKey] ?? code} />
    </Page>
  );
}
