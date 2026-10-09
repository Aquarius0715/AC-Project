"use client";

import Link from "next/link";
import { Badge, Banner, cx, EmptyState, Page, PageHead, Select, Tabs } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { SORTS, type TabId } from "@ac/web/lib/partnerJobs";
import type { JobsLive } from "../_lib/load";

const toneText = { warn: "text-warn", crit: "text-crit", ok: "text-ok", primary: "text-primary", muted: "text-muted" } as const;

/** The contractor job list (FR-P01, Figma Contractor 02-1…02-6) from the Core API: status tabs with counts, sort and
 * period in the URL (the overview's period, so the KPI counts match), one row per job and cursor paging. */
export function JobsView({ live }: { live: JobsLive }) {
  const patch = useUrlPatch();
  const reset = { cursor: null, page: null };
  const period = live.period.label === "Period" ? `${live.period.from.slice(5)} – ${live.period.to.slice(5)}` : live.period.label.toLowerCase();
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Tabs<TabId> value={live.tab} onChange={(t) => patch({ tab: t === "all" ? null : t, ...reset })} tabs={live.tabs.map((t) => ({ id: t.id, label: t.label, count: t.count }))} />
        <div className="flex flex-wrap gap-2">
          <Select aria-label="Sort" className="w-auto" value={live.sort} onChange={(e) => patch({ sort: e.target.value === "status:asc" ? null : e.target.value, ...reset })}>
            {SORTS.map((s) => <option key={s.id} value={s.id}>Sort: {s.label}</option>)}
          </Select>
          <Select aria-label="Period" className="w-auto" value={`${live.period.from}|${live.period.to}`} onChange={(e) => { const [from, to] = e.target.value.split("|"); patch({ from, to, ...reset }); }}>
            {live.period.label === "Period" && <option value={`${live.period.from}|${live.period.to}`}>Period: {live.period.from} – {live.period.to}</option>}
            {live.periods.map((p) => <option key={p.label} value={`${p.from}|${p.to}`}>Period: {p.label.toLowerCase()} ({p.from.slice(5)} – {p.to.slice(5)})</option>)}
          </Select>
        </div>
      </div>
      {live.restarted && <Banner tone="warn">The list changed since that page was loaded — showing the first page again.</Banner>}
      <section className="rounded-2xl border border-line bg-surface p-4">
        <PageHead title={`Jobs — your company only · ${live.tabs[0].count} ${period}`} />
        {live.rows.length === 0 ? (
          <EmptyState title={live.tabs[0].count === 0 ? "No jobs yet" : "No jobs in this tab"}>{live.tabs[0].count === 0 ? "Offers from HQ to your company will appear here." : "Choose another tab or period."}</EmptyState>
        ) : (
          <div className="flex flex-col">
            <div className="hidden grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_120px_16px] gap-3 border-b border-line px-1 pb-2 text-[11px] font-semibold uppercase tracking-wide text-muted @3xl:grid"><span>Job</span><span>Technician</span><span>Slot / deadline</span><span>Status</span><span /></div>
            {live.rows.map((j) => (
              <Link key={j.id} href={j.href} className="grid grid-cols-1 gap-2 border-b border-line px-1 py-3 last:border-0 hover:bg-surface2/60 @3xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_120px_16px] @3xl:items-center @3xl:gap-3">
                <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><b className="text-[13px]">{j.title}</b>{j.origin && <OriginBadge origin={j.origin === "periodic_plan" ? "plan" : "request"} />}</div><div className="text-xs text-muted">{j.line}</div></div>
                <div className="text-xs"><b className={cx(j.tech.tone && toneText[j.tech.tone])}>{j.tech.name}</b><div className={cx("text-muted", j.tech.tone === "warn" && j.tech.name !== "Unassigned" && "text-warn")}>{j.tech.sub}</div></div>
                <div className="text-xs"><b className={cx(j.slot.tone && toneText[j.slot.tone])}>{j.slot.main}</b><div className="text-muted">{j.slot.sub}</div></div>
                <div><Badge tone={j.badge.tone}>{j.badge.text}</Badge></div>
                <span aria-hidden className="hidden text-muted @3xl:block">›</span>
              </Link>
            ))}
          </div>
        )}
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-[11px] text-muted">
          <span>Showing {live.rows.length} of {live.total} · Sorted by {live.sortText} · KPI counts on Overview use the same period (your company only)</span>
          <span className="flex items-center gap-3">
            {live.paging.page > 1 && <button className="font-semibold text-primary" onClick={() => patch(reset)}>‹ First page</button>}
            <span>Page {live.paging.page} of {live.paging.of}</span>
            {live.nextCursor && <button className="font-semibold text-primary" onClick={() => patch({ cursor: live.nextCursor, page: String(live.paging.page + 1) })}>Next ›</button>}
          </span>
        </div>
      </section>
    </Page>
  );
}
