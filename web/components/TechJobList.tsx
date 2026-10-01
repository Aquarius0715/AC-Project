import Link from "next/link";
import { Badge, EmptyState } from "@/components/ui";
import { tjobs } from "@/lib/tech";

export function JobList({ jobs }: { jobs: typeof tjobs }) {
  if (!jobs.length) return <EmptyState title="No assigned jobs today">Jobs assigned to you appear here.</EmptyState>;
  return (
    <div className="flex flex-col gap-2">
      {jobs.map((j) => (
        <Link key={j.id} href={`/technician/jobs/${j.id}`} className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line bg-surface p-3 hover:bg-surface2/60">
          <div className="min-w-0"><b className="text-[13px]">{j.id} · {j.unit} · {j.cust}</b><div className="text-xs text-muted">{j.type} · window {j.win} · {j.note}</div><div className="text-xs">{j.progress}</div></div>
          <Badge tone={j.tone}>{j.status}</Badge>
        </Link>
      ))}
    </div>
  );
}
