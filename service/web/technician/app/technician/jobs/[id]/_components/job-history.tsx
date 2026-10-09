import { Card, LinkBtn, Page, SummaryList } from "@ac/web/components/ui";
import { klTime } from "@ac/web/lib/devices";
import { typeLabel } from "@ac/web/lib/partnerJobDetail";

/** JobHistorySnapshot of service-contracts.ts as the technician reads it (IR124). */
export type ApiTechHistory = {
  projection: "history"; jobId: string; type: string; status: string; asOf: string; completedAt: string | null;
  redactedReportSummary: { hasReport: boolean; acceptance: "accepted" | "not_accepted" };
};

/** A job whose viewing window ended (IR49): completed — the assignment ended with it (IR234) — or reassigned / cancelled,
 * read as the snapshot frozen when it ended (IR124), so a notification link still opens something. */
export function JobHistory({ h }: { h: ApiTechHistory }) {
  const done = h.status === "completed";
  const report = !h.redactedReportSummary.hasReport ? "No report from your assignment" : h.redactedReportSummary.acceptance === "accepted" ? "Accepted in the quality review" : "Submitted — not accepted while you were assigned";
  return (
    <Page narrow>
      <Card title={`${h.jobId.slice(0, 8)} · ${typeLabel(h.type)}`} sub={done ? "Completed — your assignment ended with the job" : "Your assignment on this job has ended"}>
        <SummaryList items={[
          ["Status", done ? `Completed${h.completedAt ? ` ${klTime(h.completedAt).slice(5)}` : ""}` : `${h.status.replace(/_/g, " ")} when your assignment ended`],
          ["Your report", report],
          ["Assignment ended", klTime(h.asOf).slice(5)],
        ]} />
        <p className="mt-3 text-xs text-muted">{done ? "Your time is free for other jobs. The report, the unit and its devices stay with HQ and the customer." : "HQ or your coordinator changed the assignment, so the job details and the unit are no longer shown to you."}</p>
        <div className="mt-3"><LinkBtn href="/technician" size="sm">← Overview</LinkBtn></div>
      </Card>
    </Page>
  );
}
