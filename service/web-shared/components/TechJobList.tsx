"use client";

import Link from "next/link";
import { Badge, EmptyState } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { tjobs } from "@ac/web/lib/tech";
import { fmt, useJobs } from "@ac/web/lib/jobs";

export type TechRow = (typeof tjobs)[number];

/** Seeded technician jobs plus jobs assigned through the shared store (e.g. job-c07). */
export function useTechJobs(today = false): TechRow[] {
  const live = useJobs().filter((j) => j.technician === "tech-external-a" && !tjobs.some((t) => t.id === j.id) && (j.status === "assigned" || j.status === "in_progress"));
  const rows: TechRow[] = live.map((j) => ({
    id: j.id, unit: j.unit, cust: j.customer, type: j.type, origin: j.origin, win: fmt(j.scheduled), today: false,
    note: j.techAck?.status === "pending" ? "new assignment — accept it" : j.techAck?.status === "cant_make" ? "you said you can’t make it" : `assigned by ${j.contractor ?? "HQ"}`,
    status: j.techAck?.status === "pending" ? "Not accepted" : j.techAck?.status === "cant_make" ? "Can’t make it" : "Assigned",
    progress: j.techAck?.status === "accepted" ? "accepted · opens on the day (read-only until then)" : "visit time agreed with the client",
    tone: (j.techAck?.status === "pending" ? "warn" : "primary") as TechRow["tone"],
  }));
  return [...rows, ...tjobs.filter((t) => !today || t.today)];
}

export function JobList({ jobs }: { jobs: TechRow[] }) {
  if (!jobs.length) return <EmptyState title="No assigned jobs today">Jobs assigned to you appear here.</EmptyState>;
  return (
    <div className="flex flex-col gap-2">
      {jobs.map((j) => (
        <Link key={j.id} href={`/technician/jobs/${j.id}`} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line bg-surface p-3 hover:bg-surface2/60">
          <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><b className="text-[13px]">{j.id} · {j.unit} · {j.cust}</b><OriginBadge origin={j.origin} /></div><div className="text-xs text-muted">{j.type} · window {j.win} · {j.note}</div><div className="text-xs">{j.progress}</div></div>
          <Badge tone={j.tone}>{j.status}</Badge>
        </Link>
      ))}
    </div>
  );
}
