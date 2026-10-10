// /technician/units/[id]/control (FR-T10, SCR-T10): in API mode a Server Component reads the unit, the job of the URL
// jobId (opened from the unit or job page), the job's test runs and its command history through the DAL. Sending a
// diagnostic command (commands.create) and starting a test run (diagnosticRuns.create) are Server Actions; the page
// refreshes while a command or run is open. Without control.diagnose the page says so; before the work window it says
// when control opens (IR76). URL key: jobId. The Phase 1A demo keeps the fixtures.
import { connection } from "next/server";
import { notFound } from "next/navigation";
import { Card, LinkBtn, Page } from "@ac/web/components/ui";
import { apiMode } from "@ac/web/lib/dal";
import { ControlDemo } from "./_components/control-demo";
import { ControlView } from "./_components/control-view";
import { loadControl } from "./_lib/load";

export default async function TechnicianControlPage({ params, searchParams }: PageProps<"/technician/units/[id]/control">) {
  const { id } = await params;
  await connection();
  if (!apiMode()) return <ControlDemo id={id} />;
  const sp = await searchParams;
  const jobId = typeof sp.jobId === "string" && sp.jobId ? sp.jobId : null; // the job the commands belong to (IR94)
  const live = await loadControl(id, jobId);
  if (!live) notFound();
  if (live.kind === "not_started") {
    return <Page className="max-w-xl"><Card title={live.title} sub={live.text}><LinkBtn href={live.unitHref} size="sm">{live.back}</LinkBtn></Card></Page>;
  }
  return <ControlView live={live} />;
}
