"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, EmptyState, ErrorState, Page, PageHead, Select, Tabs } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { PJob, pjobs } from "@ac/web/lib/partner";
import { fmt, useJobs } from "@ac/web/lib/jobs";

type T = "all" | "Offered" | "Active" | "Review" | "Completed";
type Row = PJob & { origin: "request" | "plan" };

/** The Phase 1A demo job list (DATA_SOURCE=mock): the fixture jobs and the shared job store. */
export function JobsDemo({ status }: { status?: string }) {
  const live = useJobs().filter((j) => j.contractor === "contractor-a" && !pjobs.some((p) => p.id === j.id) && j.status !== "cancelled");
  const [tab, setTab] = useState<T>(status === "offered" ? "Offered" : "all");
  const [origin, setOrigin] = useState<"all" | "request" | "plan">("all");
  const [state, setState] = useState<"ok" | "empty" | "error">("ok");
  const liveRows: Row[] = live.map((j) => {
    const offered = j.status === "offered" || j.status === "time_proposed";
    const pend = j.partnerProposal?.status === "pending" || j.partnerProposal?.status === "sent_to_client";
    return {
      id: j.id, unit: j.unit, cust: j.customer, origin: j.origin,
      line: offered ? (pend ? `Time change proposed · waiting for the client` : `Offered by HQ · visit ${fmt(j.scheduled)} (fixed)`) : j.status === "accepted" ? "Accepted · assign a technician for the agreed time" : `Assigned ${j.technician} · ${fmt(j.scheduled)}`,
      tech: j.technician, techSub: j.technician ? (j.techAck?.status === "accepted" ? "accepted ✓" : j.techAck?.status === "cant_make" ? "can’t make it ⚠" : "awaiting acceptance") : "not assignable before acceptance",
      slot: fmt(j.scheduled), deadline: offered ? "Answer by 09-25 12:00" : "", status: offered ? "Offered" : "Active",
      label: offered ? (pend ? "Time proposed" : "Offered") : j.status === "accepted" ? "Unassigned" : "Assigned", tone: pend ? "warn" : "primary",
    };
  });
  const rows: Row[] = [...liveRows, ...pjobs.map((p) => ({ ...p, origin: (p.id === "job-p09" ? "plan" : "request") as Row["origin"] }))];
  const match = (j: Row, t: T) => t === "all" || j.status === t || (t === "Active" && j.status === "Overdue");
  const jobs = rows.filter((j) => match(j, tab) && (origin === "all" || j.origin === origin));
  const count = (t: T) => rows.filter((j) => match(j, t)).length;
  return (
    <Page>
      <PageHead title="Jobs" sub={`contractor-a only · ${rows.length} this week`} />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Tabs value={tab} onChange={setTab} tabs={(["all", "Offered", "Active", "Review", "Completed"] as T[]).map((t) => ({ id: t, label: t === "all" ? "All" : t, count: count(t) }))} />
        <Select aria-label="Origin" className="w-auto" value={origin} onChange={(e) => setOrigin(e.target.value as typeof origin)}><option value="all">Origin: All</option><option value="request">Origin: Client request</option><option value="plan">Origin: Periodic plan</option></Select>
      </div>
      {state === "error" && <ErrorState title="Couldn’t refresh jobs (stale)" onRetry={() => setState("ok")}>Showing last loaded values. Retry to refresh.</ErrorState>}
      {state === "empty" ? <EmptyState title="No jobs yet">HQ offers will appear here.</EmptyState> : (
        <div className="flex flex-col gap-2">
          <div className="hidden grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_110px] gap-3 px-3 text-[11px] font-semibold uppercase tracking-wide text-muted @3xl:grid"><span>Job</span><span>Technician</span><span>Visit / deadline</span><span>Status</span></div>
          {jobs.map((j) => (
            <Link key={j.id} href={j.status === "Review" ? `/partner/jobs/${j.id}/review` : j.label === "Unassigned" && liveRows.some((r) => r.id === j.id) ? `/partner/schedule?jobId=${j.id}` : `/partner/jobs/${j.id}`} className="grid grid-cols-1 gap-2 rounded-xl border border-line bg-surface p-3 hover:bg-surface2/60 @3xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_110px] @3xl:items-center @3xl:gap-3">
              <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><b className="text-[13px]">{j.id} · {j.unit} · {j.cust}</b><OriginBadge origin={j.origin} /></div><div className="text-xs text-muted">{j.line}</div></div>
              <div className="text-xs"><b>{j.tech ?? "—"}</b><div className="text-muted">{j.techSub}</div></div>
              <div className="text-xs"><b>{j.slot}</b><div className="text-muted">{j.deadline}</div></div>
              <div><Badge tone={j.tone}>{j.label}</Badge></div>
            </Link>
          ))}
          {jobs.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">No jobs in this filter.</p>}
          <p className="text-[11px] text-muted">Showing {jobs.length} of {rows.length} · Sorted by status, then deadline · Visit times are fixed when offered; a different time needs the client’s approval via HQ.</p>
        </div>
      )}
      <div className="flex justify-end gap-3 text-[11px] text-muted"><button className="underline" onClick={() => setState("empty")}>empty state</button><button className="underline" onClick={() => setState("error")}>stale state</button><button className="underline" onClick={() => setState("ok")}>normal</button></div>
    </Page>
  );
}
