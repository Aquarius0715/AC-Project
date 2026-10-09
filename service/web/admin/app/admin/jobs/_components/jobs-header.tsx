"use client";

import Link from "next/link";
import { Select, cx } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";

type Scope = { customerId?: string; propertyId?: string; unitId?: string };
const keep = (q: Scope, extra: Record<string, string> = {}) => {
  const sp = new URLSearchParams({ ...Object.fromEntries(Object.entries(q).filter(([, v]) => v) as [string, string][]), ...extra });
  return sp.size ? `/admin/jobs?${sp}` : "/admin/jobs";
};

/** The tabs of /admin/jobs with their counts and the tab's create button (Figma 06-1, 283:2); the customer → property →
 * unit scope is shared, so the tab links keep it (DD-A06 item 5). */
export function JobsTabs({ tab, counts, q, action }: { tab: string; counts: { jobs: number; plans: number; contractors: number }; q: Scope; action?: React.ReactNode }) {
  const tabs = [{ id: "jobs", label: "Jobs", count: counts.jobs }, { id: "plans", label: "Plans", count: counts.plans }, { id: "contractors", label: "Contractors", count: counts.contractors }, { id: "sla", label: "SLA by customer" }];
  return (
    <div className="flex flex-wrap items-end justify-between gap-2 border-b border-line">
      <div role="tablist" className="flex flex-wrap gap-4">
        {tabs.map((t) => <Link key={t.id} role="tab" aria-selected={tab === t.id} href={keep(q, t.id === "jobs" ? {} : { tab: t.id })} className={cx("-mb-px border-b-2 px-1 pb-2 text-[13px] font-semibold", tab === t.id ? "border-primary text-primary" : "border-transparent text-muted hover:text-ink")}>{t.label}{t.count !== undefined && <span className="ml-1.5 rounded bg-surface2 px-1.5 text-[11px]">{t.count}</span>}</Link>)}
      </div>
      {action && <div className="mb-1.5">{action}</div>}
    </div>
  );
}

/** The scope selects; changing one clears the narrower ones and the selected job or plan (`clear`). */
export function ScopeBar({ scope, q, text, clear }: { scope: { customers: { id: string; name: string }[]; properties: { id: string; name: string }[]; units: { id: string; name: string }[] }; q: Scope; text: string; clear: "jobId" | "planId" }) {
  const patch = useUrlPatch();
  return (
    <div className="flex flex-wrap items-center gap-2 text-[13px]">
      <span className="text-muted">Scope</span>
      <Select aria-label="Customer" className="w-auto" value={q.customerId ?? ""} onChange={(e) => patch({ customerId: e.target.value || null, propertyId: null, unitId: null, [clear]: null })}><option value="">Customer: All</option>{scope.customers.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select>
      <Select aria-label="Property" className="w-auto" value={q.propertyId ?? ""} onChange={(e) => patch({ propertyId: e.target.value || null, unitId: null, [clear]: null })}><option value="">Property: All</option>{scope.properties.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</Select>
      <Select aria-label="Unit" className="w-auto" value={q.unitId ?? ""} onChange={(e) => patch({ unitId: e.target.value || null, [clear]: null })}><option value="">Unit: All</option>{scope.units.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</Select>
      <span className="ml-auto text-xs text-muted">{text}</span>
    </div>
  );
}
