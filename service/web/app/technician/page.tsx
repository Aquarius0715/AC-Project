"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Banner, Card, EmptyState, Kpi, LinkBtn, Page, SeverityBadge, Tabs, TextLink, UtilBar } from "@/components/ui";
import { fmt, useJobs } from "@/lib/jobs";
import { JobList, useTechJobs } from "@/components/TechJobList";

export default function TechOverview({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const sp = use(searchParams);
  const [tab, setTab] = useState<"today" | "all">(sp.tab === "all" ? "all" : "today");
  const all = useTechJobs();
  const jobs = tab === "today" ? all.filter((j) => j.today) : all;
  const pending = useJobs().filter((j) => j.technician === "tech-external-a" && j.techAck?.status === "pending");
  return (
    <Page>
      <div className="grid-fluid" style={{ ["--min"as string]: "180px" }}>
        <Kpi label="Assigned units" value={2} sub="in your assignments" /><Kpi label="Not started" value={1} sub="job-t07" /><Kpi label="Overdue" value={0} tone="ok" sub="No overdue jobs" />
      </div>
      {pending.map((j) => <Banner key={j.id} tone="warn" icon="✉" action={<LinkBtn size="sm" variant="primary" href={`/technician/jobs/${j.id}`}>Review & accept</LinkBtn>}><b>New assignment to accept — {j.id} · {j.unit} · {fmt(j.scheduled)}</b> · assigned by {j.contractor ?? "HQ"}. The visit time was agreed with the client.</Banner>)}
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Tabs value={tab} onChange={setTab} tabs={[{ id: "today", label: "Today", count: 2 }, { id: "all", label: "All assigned", count: all.length }]} />
          <Card title={tab === "today" ? "Today — sorted by severity, deadline, progress" : "All assigned jobs"}>
            <JobList jobs={jobs} />
            {tab === "today" && <p className="mt-3 text-xs text-muted">job-t07 shows read-only until its work window opens at 14:00 — actions are disabled until then (IR76).</p>}
          </Card>
          <Card title="Today · Mon 2026-09-21" sub="Sort by time">
            <div className="scroll-x"><div className="relative min-w-[480px]"><div className="grid grid-cols-5 text-[11px] text-muted">{["08:00", "10:00", "12:00", "14:00", "16:00"].map((h) => <span key={h}>{h}</span>)}</div><div className="relative mt-1 h-16 rounded-lg bg-surface2"><div className="absolute top-1 h-6 rounded bg-primary px-2 text-[11px] leading-6 text-white" style={{ left: "25%", width: "25%" }}>10–12 job-contractor-a</div><div className="absolute bottom-1 h-6 rounded bg-primary-soft px-2 text-[11px] leading-6 text-primary" style={{ left: "75%", width: "25%" }}>14–16 job-t07</div><span className="absolute inset-y-0 w-px bg-crit" style={{ left: "46%" }} /></div></div></div>
            <p className="mt-2 text-[11px] text-muted">Red line = now (11:45). Travel time is not planned by the system.</p>
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Alerts on my units (2)" action={<TextLink href="/technician/units/unit-online-rto/alerts?jobId=job-t07">All →</TextLink>}>
            {[["Bedroom too hot", "unit-online-rto · 30.4 °C · 09:12", "warning", "/technician/units/unit-online-rto/alerts"], ["Filter pressure drop", "unit-t07-sr · suspected · 08:40", "normal", "/technician/units/unit-online-rto/alerts"]].map(([t, d, s, h]) => <Link key={t} href={h} className="flex items-center justify-between gap-2 border-t border-line py-2 first:border-0"><span><b className="text-[13px]">{t}</b><span className="block text-xs text-muted">{d}</span></span><SeverityBadge s={s as "warning"} /></Link>)}
            <p className="mt-1 text-[11px] text-muted">Only units in your assignments. Finishing a job does not resolve its alert.</p>
          </Card>
          <Card title="My reports">
            <div className="flex flex-col gap-3 text-[13px]"><div><b>job-contractor-a · draft v2</b><div className="mb-1 text-xs text-muted">7 / 18 · Indoor 7/8 · Outdoor 0/5 · Electrical 0/5</div><UtilBar pct={39} /></div><div><b>job-p05 · returned</b><div className="text-xs text-muted">“Photo evidence missing” — resume as v2</div></div><div><b>job-p09 · v1 submitted</b><div className="text-xs text-muted">awaiting quality review · read-only</div></div></div>
          </Card>
        </div>
      </div>
    </Page>
  );
}
