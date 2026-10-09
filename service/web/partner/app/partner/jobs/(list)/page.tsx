// /partner/jobs (FR-P01, SCR-P09): in API mode a Server Component reads the company's jobs of the URL's tab, sort,
// period and page through the DAL. The Phase 1A demo keeps the fixture list.
import { connection } from "next/server";
import { apiMode } from "@ac/web/lib/dal";
import { JobsDemo } from "./_components/jobs-demo";
import { JobsView } from "./_components/jobs-view";
import { loadJobs } from "./_lib/load";

export default async function PartnerJobsPage({ searchParams }: PageProps<"/partner/jobs">) {
  await connection();
  const sp = await searchParams;
  const one = (k: string) => (typeof sp[k] === "string" && sp[k] ? (sp[k] as string) : undefined);
  if (!apiMode()) return <JobsDemo status={one("status")} />;
  // the overview's KPI links use status=…; the list's URL key is tab
  const tab = one("tab") ?? ({ offered: "offered", active: "active", submitted: "review", completed: "completed" } as Record<string, string>)[one("status") ?? ""];
  return <JobsView live={await loadJobs({ tab, sort: one("sort"), from: one("from"), to: one("to"), cursor: one("cursor"), page: one("page") })} />;
}
