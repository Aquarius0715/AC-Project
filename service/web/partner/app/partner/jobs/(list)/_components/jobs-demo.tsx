"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, EmptyState, ErrorState, Page, PageHead, Select, Tabs, Plain } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { PJob, pjobs } from "@ac/web/lib/partner";
import { fmt, useJobs } from "@ac/web/lib/jobs";
import { oneOf } from "@ac/web/lib/oneOf";
import { orDash } from "@ac/web/lib/text";

type Row = PJob & { origin: "request" | "plan" };
type StoreJob = ReturnType<typeof useJobs>[number];

/** A store job's line and status label: offered (or with the contractor's time proposal), accepted, or assigned. */
function stage(j: StoreJob, isOffered: boolean, isPend: boolean): { line: string; label: string } {
  if (isOffered && isPend) return { line: "Time change proposed · waiting for the client", label: "Time proposed" };
  if (isOffered) return { line: `Offered by HQ · visit ${fmt(j.scheduled)} (fixed)`, label: "Offered" };
  if (j.status === "accepted") return { line: "Accepted · assign a technician for the agreed time", label: "Unassigned" };
  return { line: `Assigned ${orDash(j.technician)} · ${fmt(j.scheduled)}`, label: "Assigned" };
}
/** The technician's answer under their name, or why nobody can be assigned yet. */
function techSub(j: StoreJob): string {
  if (!j.technician) return "not assignable before acceptance";
  if (j.techAck?.status === "accepted") return "accepted";
  if (j.techAck?.status === "cant_make") return "can’t make it";
  return "awaiting acceptance";
}
/** Where a row leads: the review, the schedule for a store job waiting for a technician, or the job. */
function jobHref(j: Row, isStoreJob: boolean): string {
  if (j.status === "Review") return `/partner/jobs/${j.id}/review`;
  if (isStoreJob && j.label === "Unassigned") return `/partner/schedule?jobId=${j.id}`;
  return `/partner/jobs/${j.id}`;
}

/** The Phase 1A demo job list (DATA_SOURCE=mock): the fixture jobs and the shared job store. */
export function JobsDemo({ status }: Readonly<{ status?: string }>) {
  type T = "all" | "Offered" | "Active" | "Review" | "Completed";
  const live = useJobs().filter((j) => j.contractor === "contractor-a" && pjobs.every((p) => p.id !== j.id) && j.status !== "cancelled");
  const [tab, setTab] = useState<T>(status === "offered" ? "Offered" : "all");
  const [origin, setOrigin] = useState<"all" | "request" | "plan">("all");
  const [state, setState] = useState<"ok" | "empty" | "error">("ok");
  const liveRows: Row[] = live.map((j) => {
    const isOffered = j.status === "offered" || j.status === "time_proposed";
    const isPend = j.partnerProposal?.status === "pending" || j.partnerProposal?.status === "sent_to_client";
    return {
      id: j.id, unit: j.unit, cust: j.customer, origin: j.origin, ...stage(j, isOffered, isPend),
      tech: j.technician, techSub: techSub(j),
      slot: fmt(j.scheduled), deadline: isOffered ? "Answer by 09-25 12:00" : "", status: isOffered ? "Offered" : "Active",
      tone: isPend ? "warn" : "primary",
    };
  });
  const rows: Row[] = [...liveRows, ...pjobs.map((p): Row => ({ ...p, origin: p.id === "job-p09" ? "plan" : "request" }))];
  const match = (j: Row, t: T) => t === "all" || j.status === t || (t === "Active" && j.status === "Overdue");
  const jobs = rows.filter((j) => match(j, tab) && (origin === "all" || j.origin === origin));
  const count = (t: T) => rows.filter((j) => match(j, t)).length;
  return (
    <Page>
      <PageHead title="Jobs" sub={`contractor-a only · ${String(rows.length)} this week`} />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Tabs value={tab} onChange={setTab} tabs={(["all", "Offered", "Active", "Review", "Completed"] as T[]).map((t) => ({ id: t, label: t === "all" ? "All" : t, count: count(t) }))} />
        <Select aria-label="Origin" className="w-auto" value={origin} onChange={(e) => { setOrigin(oneOf(["all", "request", "plan"] as const, e.target.value, origin)); }}><option value="all">Origin: All</option><option value="request">Origin: Client request</option><option value="plan">Origin: Periodic plan</option></Select>
      </div>
      {state === "error" && <ErrorState title="Couldn’t refresh jobs (stale)" onRetry={() => { setState("ok"); }}>Showing last loaded values. Retry to refresh.</ErrorState>}
      {state === "empty" ? <EmptyState title="No jobs yet">HQ offers will appear here.</EmptyState> : (
        <div className="flex flex-col gap-2">
          <div className="hidden grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_110px] gap-3 px-3 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground @3xl:grid"><span>Job</span><span>Technician</span><span>Visit / deadline</span><span>Status</span></div>
          {jobs.map((j) => (
            <Link key={j.id} href={jobHref(j, liveRows.some((r) => r.id === j.id))} className="grid grid-cols-1 gap-2 rounded-xl border border-line bg-surface p-3 hover:bg-surface2/60 @3xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_110px] @3xl:items-center @3xl:gap-3">
              <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><b className="text-[13px]">{j.id} · {j.unit} · {j.cust}</b><OriginBadge origin={j.origin} /></div><div className="text-xs text-muted-foreground">{j.line}</div></div>
              <div className="text-xs"><b>{j.tech ?? "—"}</b><div className="text-muted-foreground">{j.techSub}</div></div>
              <div className="text-xs"><b>{j.slot}</b><div className="text-muted-foreground">{j.deadline}</div></div>
              <div><Badge tone={j.tone}>{j.label}</Badge></div>
            </Link>
          ))}
          {jobs.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted-foreground">No jobs in this filter.</p>}
          <p className="text-[11px] text-muted-foreground">Showing {jobs.length} of {rows.length} · Sorted by status, then deadline · Visit times are fixed when offered; a different time needs the client’s approval via HQ.</p>
        </div>
      )}
      <div className="flex justify-end gap-3 text-[11px] text-muted-foreground"><Plain className="underline" onClick={() => { setState("empty"); }}>empty state</Plain><Plain className="underline" onClick={() => { setState("error"); }}>stale state</Plain><Plain className="underline" onClick={() => { setState("ok"); }}>normal</Plain></div>
    </Page>
  );
}
