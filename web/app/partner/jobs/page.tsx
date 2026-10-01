"use client";

import Link from "next/link";
import { use, useState } from "react";
import { Badge, Btn, Card, EmptyState, ErrorState, ListRow, Page, PageHead, Tabs, cx } from "@/components/ui";
import { pjobs } from "@/lib/partner";

type T = "all" | "Offered" | "Active" | "Review" | "Completed";
export default function PartnerJobs({ searchParams }: { searchParams: Promise<{ status?: string }> }) {
  const { status } = use(searchParams);
  const [tab, setTab] = useState<T>(status === "offered" ? "Offered" : "all");
  const [state, setState] = useState<"ok" | "empty" | "error">("ok");
  const jobs = pjobs.filter((j) => tab === "all" || j.status === tab || (tab === "Active" && j.status === "Overdue"));
  const count = (t: T) => pjobs.filter((j) => t === "all" || j.status === t || (t === "Active" && j.status === "Overdue")).length;
  return (
    <Page>
      <PageHead title="Jobs" sub={`contractor-a only · ${pjobs.length} this week`} />
      <Tabs value={tab} onChange={setTab} tabs={(["all", "Offered", "Active", "Review", "Completed"] as T[]).map((t) => ({ id: t, label: t === "all" ? "All" : t, count: count(t) }))} />
      {state === "error" && <ErrorState title="Couldn’t refresh jobs (stale)" onRetry={() => setState("ok")}>Showing last loaded values. Retry to refresh.</ErrorState>}
      {state === "empty" ? <EmptyState title="No jobs yet">HQ offers will appear here.</EmptyState> : (
        <div className="flex flex-col gap-2">
          <div className="hidden grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_100px] gap-3 px-3 text-[11px] font-semibold uppercase tracking-wide text-muted @3xl:grid"><span>Job</span><span>Technician</span><span>Slot / deadline</span><span>Status</span></div>
          {jobs.map((j) => (
            <Link key={j.id} href={j.status === "Review" ? `/partner/jobs/${j.id}/review` : `/partner/jobs/${j.id}`} className="grid grid-cols-1 gap-2 rounded-xl border border-line bg-surface p-3 hover:bg-surface2/60 @3xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_100px] @3xl:items-center @3xl:gap-3">
              <div className="min-w-0"><b className="text-[13px]">{j.id} · {j.unit} · {j.cust}</b><div className="text-xs text-muted">{j.line}</div></div>
              <div className="text-xs"><b>{j.tech ?? "—"}</b><div className="text-muted">{j.techSub}</div></div>
              <div className="text-xs"><b>{j.slot}</b><div className="text-muted">{j.deadline}</div></div>
              <div><Badge tone={j.tone}>{j.label}</Badge></div>
            </Link>
          ))}
          {jobs.length === 0 && <p className="rounded-xl border border-line bg-surface p-4 text-center text-xs text-muted">No jobs in this filter.</p>}
          <p className="text-[11px] text-muted">Showing {jobs.length} of 10 · Sorted by status, then deadline · KPI counts on Overview use the same filter (own company only)</p>
        </div>
      )}
      <div className="flex justify-end gap-3 text-[11px] text-muted"><button className="underline" onClick={() => setState("empty")}>empty state</button><button className="underline" onClick={() => setState("error")}>stale state</button><button className="underline" onClick={() => setState("ok")}>normal</button></div>
    </Page>
  );
}
