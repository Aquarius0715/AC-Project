"use client";

import Link from "next/link";
import { Badge, Banner, Card, EmptyState, Kpi, LinkBtn, Page, Select, SeverityBadge, TextLink, cx } from "@ac/web/components/ui";
import { OriginBadge } from "@ac/web/components/JobBits";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { klTime } from "@ac/web/lib/devices";
import type { OverviewLive } from "../_lib/load";

/** The technician overview (FR-T01, Figma Technician 01-1…01-5) from the Core API. */
export function OverviewView({ live }: { live: OverviewLive }) {
  const patch = useUrlPatch();
  const day = klTime(live.now).slice(0, 10);
  const weekday = new Date(`${day}T00:00:00Z`).toLocaleDateString("en-GB", { weekday: "short", timeZone: "UTC" });
  return (
    <Page>
      <div className="grid-fluid" style={{ ["--min" as string]: "180px" }}>
        {live.kpis.map((k) => <Kpi key={k.label} label={k.label} value={k.value} sub={k.sub} tone={k.tone} />)}
      </div>
      {live.pending.length === 1 && (
        <Banner tone="warn" icon="✉" action={<LinkBtn size="sm" variant="primary" href={`/technician/jobs/${live.pending[0].id}`}>Review & accept</LinkBtn>}>
          <b>New assignment to accept — {live.pending[0].short} · {live.pending[0].title} · {live.pending[0].slotText}</b>. The visit time was agreed with the client; accept it or say you can’t make it.
        </Banner>
      )}
      {live.pending.length > 1 && (
        <Banner tone="warn" icon="✉" action={<LinkBtn size="sm" variant="primary" href={`/technician/jobs/${live.pending[0].id}`}>Review the first</LinkBtn>}>
          <b>{live.pending.length} new assignments to accept</b> — {live.pending.map((j, i) => <span key={j.id}>{i ? ", " : ""}<Link className="font-semibold underline" href={`/technician/jobs/${j.id}`}>{j.short} · {j.slotText}</Link></span>)}. The visit times were agreed with the client.
        </Banner>
      )}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="tablist" className="flex flex-wrap gap-2">
          {live.tabs.map((t) => <button key={t.id} role="tab" aria-selected={live.tab === t.id} onClick={() => patch({ tab: t.id === "today" ? null : t.id })} className={cx("rounded-control border px-3 py-1.5 text-[13px] font-semibold", live.tab === t.id ? "border-primary bg-primary text-white" : "border-line bg-surface text-ink hover:bg-surface2")}>{t.label} ({t.count})</button>)}
        </div>
        <Select aria-label="Sort" className="w-auto" value={live.sort} onChange={(e) => patch({ sort: e.target.value === "severity" ? null : e.target.value })}>{live.sorts.map((s) => <option key={s.id} value={s.id}>Sort: {s.text}</option>)}</Select>
      </div>
      <Card title={live.tab === "today" ? "Today — sorted by severity, deadline, progress" : `All assigned — sorted by ${live.sorts.find((s) => s.id === live.sort)!.text.replace(/ [↑↓]$/, "").toLowerCase()}`}>
        {live.rows.length === 0 ? (
          <EmptyState title={live.tab === "today" ? "No assigned jobs today" : "No assigned jobs"} action={live.tab === "today" && live.tabs[1].count ? <button type="button" className="text-sm font-semibold text-primary" onClick={() => patch({ tab: "all" })}>All assigned ({live.tabs[1].count}) →</button> : undefined}>
            Jobs assigned to you appear here. Past assignments stay in their job history.
          </EmptyState>
        ) : (
          <div className="flex flex-col">
            {live.rows.map((j) => (
              <Link key={j.id} href={`/technician/jobs/${j.id}`} className="flex items-center justify-between gap-3 border-t border-line py-2.5 first:border-0 hover:bg-surface2/60">
                <span className="min-w-0"><span className="flex flex-wrap items-center gap-2"><b className="text-[13px]">{j.short} · {j.title}</b>{j.origin && <OriginBadge origin={j.origin === "periodic_plan" ? "plan" : "request"} />}</span><span className="block text-xs text-muted">{j.line}</span></span>
                <span className="flex shrink-0 items-center gap-2"><Badge tone={j.badge.tone}>{j.badge.text}</Badge><span aria-hidden className="text-muted">›</span></span>
              </Link>
            ))}
          </div>
        )}
      </Card>
      {live.tab === "today" && live.readOnly.map((r) => <Banner key={r.id}>{r.short} shows read-only until its work window opens at {klTime(r.opensAt!).slice(11)} — actions are disabled until then (IR76).</Banner>)}
      <div className="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)]">
        <Card title={`Today · ${weekday} ${day}`} sub="Sort by time">
          <div className="scroll-x">
            <div className="relative min-w-[480px]">
              <div className="flex justify-between text-[11px] text-muted">{live.timeline.hours.map((h) => <span key={h}>{h}</span>)}</div>
              <div className="relative mt-1 h-10 rounded-lg bg-surface2">
                {live.timeline.blocks.map((b) => <div key={b.id} className={cx("absolute top-2 h-6 truncate rounded px-2 text-[11px] leading-6", b.current ? "bg-primary text-white" : "border border-dashed border-primary bg-primary-soft text-primary")} style={{ left: `${b.left}%`, width: `${b.width}%` }}>{b.short}</div>)}
                {live.timeline.nowPct !== null && <span className="absolute inset-y-0 w-0.5 bg-crit" style={{ left: `${live.timeline.nowPct}%` }} />}
              </div>
            </div>
          </div>
          <div className="mt-2 flex flex-col">
            {live.todayRows.length === 0 && <p className="text-[13px] text-muted">Nothing scheduled today.</p>}
            {live.todayRows.map((j) => <Link key={j.id} href={`/technician/jobs/${j.id}`} className="flex items-center justify-between gap-2 border-t border-line py-2 first:border-0"><span className="min-w-0"><b className="text-[13px]">{j.slotText} · {j.short} · {j.title}</b><span className="block truncate text-[11px] text-muted">{j.line}</span></span><Badge tone={j.badge.tone}>{j.badge.text}</Badge></Link>)}
          </div>
          <p className="mt-2 text-[11px] text-muted">Red line = now ({klTime(live.now).slice(11)}). Travel time is not planned by the system.</p>
        </Card>
        <Card title={`Alerts on my units (${live.alerts.length})`} action={live.alerts[0] && <TextLink href={live.alerts[0].href}>All →</TextLink>}>
          {live.alerts.length === 0 && <p className="text-[13px] text-muted">No open alerts on your units.</p>}
          {live.alerts.slice(0, 4).map((a) => <Link key={a.id} href={a.href} className="flex items-center justify-between gap-2 border-t border-line py-2 first:border-0"><span className="min-w-0"><b className="text-[13px]">{a.title}</b><span className="block truncate text-xs text-muted">{a.sub}</span></span><SeverityBadge s={a.severity} /></Link>)}
          <p className="mt-1 text-[11px] text-muted">Only units in your assignments. Finishing a job does not resolve its alert.</p>
        </Card>
        <Card title="My reports">
          {live.reports.length === 0 && <p className="text-[13px] text-muted">No drafts, returned or submitted reports.</p>}
          {live.reports.map((r) => <Link key={r.id} href={`/technician/jobs/${r.id}`} className="flex items-center justify-between gap-2 border-t border-line py-2 first:border-0"><span className="min-w-0"><b className="text-[13px]">{r.title}</b><span className="block text-xs text-muted">{r.sub}</span></span><Badge tone={r.badge.tone}>{r.badge.text}</Badge></Link>)}
        </Card>
      </div>
    </Page>
  );
}
