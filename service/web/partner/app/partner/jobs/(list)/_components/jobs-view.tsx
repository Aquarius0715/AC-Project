"use client";

import Link from "next/link";
import { Badge, Banner, cx, EmptyState, Page, PageHead, Select, Tabs } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { useT } from "@ac/web/components/I18n";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { TabId } from "@ac/web/lib/partnerJobs";
import type { JobsLive } from "../_lib/load";

const toneText = { warn: "text-warn", crit: "text-crit", ok: "text-ok", primary: "text-primary", muted: "text-muted" } as const;

/** The contractor job list (FR-P01, Figma Contractor 02-1…02-6) from the Core API: status tabs with counts, sort and
 * period in the URL (the overview's period, so the KPI counts match), one row per job and cursor paging. Texts in the
 * user's display language (IR271). */
export function JobsView({ live }: { live: JobsLive }) {
  const t = useT();
  const patch = useUrlPatch();
  const reset = { cursor: null, page: null };
  const all = live.tabs[0].count;
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Tabs<TabId> value={live.tab} onChange={(x) => patch({ tab: x === "all" ? null : x, ...reset })} tabs={live.tabs.map((x) => ({ id: x.id, label: x.label, count: x.count }))} />
        <div className="flex flex-wrap gap-2">
          <Select aria-label={t("Sort")} className="w-auto" value={live.sort} onChange={(e) => patch({ sort: e.target.value === "status:asc" ? null : e.target.value, ...reset })}>
            {live.sorts.map((s) => <option key={s.id} value={s.id}>{s.label}</option>)}
          </Select>
          <Select aria-label={t("Period")} className="w-auto" value={live.period.value} onChange={(e) => { const [from, to] = e.target.value.split("|"); patch({ from, to, ...reset }); }}>
            {live.period.custom && <option value={live.period.value}>{t("Period: {period}", { period: live.period.label })}</option>}
            {live.periods.map((p) => <option key={p.value} value={p.value}>{t("Period: {period}", { period: p.label })}</option>)}
          </Select>
        </div>
      </div>
      {live.restarted && <Banner tone="warn">{t("The list changed since that page was loaded — showing the first page again.")}</Banner>}
      <section className="rounded-2xl border border-line bg-surface p-4">
        <PageHead title={t("Jobs — your company only · {n} {period}", { n: all, period: live.period.text })} />
        {live.rows.length === 0 ? (
          <EmptyState title={t(all === 0 ? "No jobs yet" : "No jobs in this tab")}>{t(all === 0 ? "Offers from HQ to your company will appear here." : "Choose another tab or period.")}</EmptyState>
        ) : (
          <div className="flex flex-col">
            <div className="hidden grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_120px_16px] gap-3 border-b border-line px-1 pb-2 text-[11px] font-semibold uppercase tracking-wide text-muted @3xl:grid"><span>{t("Job")}</span><span>{t("Technician")}</span><span>{t("Slot / deadline")}</span><span>{t("Status")}</span><span /></div>
            {live.rows.map((j) => (
              <Link key={j.id} href={j.href} className="grid grid-cols-1 gap-2 border-b border-line px-1 py-3 last:border-0 hover:bg-surface2/60 @3xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_120px_16px] @3xl:items-center @3xl:gap-3">
                <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><b className="text-[13px]">{j.title}</b>{j.origin && <OriginBadge origin={j.origin === "periodic_plan" ? "plan" : "request"} />}</div><div className="text-xs text-muted">{j.line}</div></div>
                <div className="text-xs"><b className={cx(j.tech.tone && toneText[j.tech.tone])}>{j.tech.name}</b><div className={cx("text-muted", j.tech.subTone && toneText[j.tech.subTone])}>{j.tech.sub}</div></div>
                <div className="text-xs"><b className={cx(j.slot.tone && toneText[j.slot.tone])}>{j.slot.main}</b><div className="text-muted">{j.slot.sub}</div></div>
                <div><Badge tone={j.badge.tone}>{j.badge.text}</Badge></div>
                <span aria-hidden className="hidden text-muted @3xl:block">›</span>
              </Link>
            ))}
          </div>
        )}
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-[11px] text-muted">
          <span>{t("Showing {shown} of {total} · Sorted by {sort} · KPI counts on Overview use the same period (your company only)", { shown: live.rows.length, total: live.total, sort: live.sortText })}</span>
          <span className="flex items-center gap-3">
            {live.paging.page > 1 && <button className="font-semibold text-primary" onClick={() => patch(reset)}>{t("‹ First page")}</button>}
            <span>{t("Page {page} of {of}", { page: live.paging.page, of: live.paging.of })}</span>
            {live.nextCursor && <button className="font-semibold text-primary" onClick={() => patch({ cursor: live.nextCursor, page: String(live.paging.page + 1) })}>{t("Next ›")}</button>}
          </span>
        </div>
      </section>
    </Page>
  );
}
