"use client";

import Link from "next/link";
import { Banner, Card, Kpi, Page, TextLink, UtilBar } from "@/components/ui";
import { fmt, useJobs } from "@/lib/jobs";

const hours = [8, 10, 12, 14, 16, 18];
export default function PartnerOverview() {
  const live = useJobs().filter((j) => j.contractor === "contractor-a" && j.id !== "job-p09");
  const actions: [string, string, string][] = [
    ...live.filter((j) => j.status === "offered" && j.partnerProposal?.status !== "pending").map((j): [string, string, string] => [`${j.id} · ${j.unit} · ${j.customer}`, `New offer · visit ${fmt(j.scheduled)} (fixed) — accept, decline or propose another time`, `/partner/jobs/${j.id}`]),
    ...live.filter((j) => j.status === "accepted").map((j): [string, string, string] => [`${j.id} · ${j.unit}`, `Accepted · assign a technician for ${fmt(j.scheduled)}`, `/partner/schedule?jobId=${j.id}`]),
    ...live.filter((j) => j.techAck?.status === "cant_make").map((j): [string, string, string] => [`${j.id} · ${j.unit}`, `${j.technician} can’t make ${fmt(j.scheduled)} — reassign`, `/partner/schedule?jobId=${j.id}`]),
  ];
  return (
    <Page>
      <div className="flex flex-wrap justify-between gap-2 text-xs text-muted"><span>Updated 09:30 · contractor-a only</span></div>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
        <Kpi label="Offers to answer" value={1 + live.filter((j) => j.status === "offered").length} sub="job-p09 · expires in 15 h" href="/partner/jobs?status=offered" link="Open offers →" />
        <Kpi label="Awaiting assignment" value={2} sub="Accepted, no technician yet" href="/partner/schedule" link="Assign →" tone="warn" />
        <Kpi label="In progress / scheduled" value={1} sub="tech-external-a on site 10:00" href="/partner/jobs" link="View jobs →" />
        <Kpi label="Reports to review" value={1} sub="job-p05 · submitted 09-20" href="/partner/jobs/job-p05/review" link="Review →" />
        <Kpi label="Overdue" value={1} tone="crit" sub="Work window ended (IR89)" href="/partner/jobs" link="View →" />
      </div>
      <Card title="Job progress — this week" action={<TextLink href="/partner/jobs">All jobs →</TextLink>}>
        <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
          {[["Offered", 1, "primary"], ["Accepted · unassigned", 2, "warn"], ["Assigned / in progress", 1, "primary"], ["Overdue", 1, "crit"], ["Submitted · in review", 1, "primary"], ["Completed", 4, "ok"]].map(([l, n]) => <div key={l as string} className="rounded-xl bg-surface2 p-3"><div className="text-[11px] text-muted">{l}</div><div className="text-xl font-bold">{n}</div></div>)}
        </div>
      </Card>
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`Needs your action (${actions.length + 5})`}>
            <ul className="divide-y divide-line">{[...actions, ["job-p09 · Lobby AC · customer-b", "Answer by 2026-09-22 01:00 (15 h left)", "/partner/jobs/job-p09"], ["job-p07 · Server room AC", "Work window ended 09-20 17:00", "/partner/schedule?jobId=job-p07"], ["job-p05 · Server room AC", "Report by tech-external-a · awaiting review", "/partner/jobs/job-p05/review"], ["job-p02 · Rooftop unit", "No technician · delegation ends 09-22 00:00", "/partner/schedule?jobId=job-p02"], ["job-p12 · Lobby AC", "No technician · delegation ends 09-24 18:00", "/partner/schedule?jobId=job-p12"]].map(([t, d, h]) => <li key={t}><Link href={h} className="flex items-center justify-between gap-2 py-2.5 hover:bg-surface2/50"><span><b className="text-[13px]">{t}</b><span className="block text-xs text-muted">{d}</span></span><span className="text-muted">›</span></Link></li>)}</ul>
          </Card>
          <Card title="Today · Mon 2026-09-21" action={<TextLink href="/partner/schedule">Schedule →</TextLink>}>
            <div className="scroll-x"><div className="min-w-[520px]">
              <div className="grid grid-cols-[120px_repeat(6,1fr)] text-[11px] text-muted"><span />{hours.map((h) => <span key={h}>{String(h).padStart(2, "0")}:00</span>)}</div>
              {[["tech-external-a", "Refrigerant handling", { s: 2, w: 1, t: "10–12" }], ["tech-external-a2", "General maintenance", null]].map(([n, q, b]) => (
                <div key={n as string} className="relative grid grid-cols-[120px_repeat(6,1fr)] items-center border-t border-line py-2 text-xs"><span><b>{n as string}</b><span className="block text-[11px] text-muted">{q as string}</span></span><div className="col-span-6 relative h-8 rounded-lg bg-surface2">{b ? <div className="absolute top-1 h-6 rounded bg-primary px-2 text-[11px] leading-6 text-white" style={{ left: "16.6%", width: "16.6%" }}>10–12</div> : <span className="absolute inset-0 grid place-items-center text-[11px] text-muted">Free all day</span>}<span className="absolute inset-y-0 w-px bg-crit" style={{ left: "29%" }} /></div></div>
              ))}
            </div></div>
            <p className="mt-2 text-[11px] text-muted">Red line = now (09:30). 10–12 = job-contractor-a · Bedroom AC (assigned, starts 10:00).</p>
            <div className="mt-2"><Banner tone="warn">2 accepted jobs still have no technician — assign them before their delegation period ends.</Banner></div>
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title="Team capacity — this week" action={<TextLink href="/partner/team">Team →</TextLink>}>
            {[["tech-external-a", 25, "10 h / 40 h · 25%"], ["tech-external-a2", 10, "4 h / 40 h · 10%"]].map(([n, p, l]) => <div key={n as string} className="mb-3"><div className="mb-1 flex justify-between text-xs"><b>{n}</b><span className="text-muted">{l}</span></div><UtilBar pct={p as number} /></div>)}
            <p className="text-[11px] text-muted">Utilization = assigned hours ÷ available hours. Undefined hours show “—”.</p>
          </Card>
          <Card title="Recent activity" action={<TextLink href="/partner/history">History →</TextLink>}>
            <ul className="flex flex-col gap-2 text-[13px]">{[["09:05", "tech-external-a checked in — job-contractor-a"], ["09-20 18:10", "Report submitted — job-p05 (tech-external-a)"], ["09-20 17:00", "Work window ended — job-p07"], ["09-20 11:00", "HQ offered job-p09 · Lobby AC"]].map(([t, d]) => <li key={d} className="grid grid-cols-[76px_1fr] gap-2"><span className="text-xs text-muted">{t}</span><span>{d}</span></li>)}</ul>
          </Card>
        </div>
      </div>
    </Page>
  );
}
