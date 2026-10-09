"use client";

import Link from "next/link";
import { Badge, Banner, Card, cx, EmptyState, Kpi, LinkBtn, Page, Select, TextLink, UtilBar } from "@ac/web/components/ui";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import { BUCKETS, DAY_END, DAY_START, type ActionRow } from "@ac/web/lib/partnerOverview";
import type { OverviewLive } from "../_lib/load";

const badgeTone: Record<ActionRow["kind"], "primary" | "crit" | "warn"> = { offer: "primary", overdue: "crit", review: "primary", unassigned: "primary" };
const hours = Array.from({ length: (DAY_END - DAY_START) / 2 + 1 }, (_, i) => DAY_START + i * 2);

/** The contractor overview (FR-P01, Figma Contractor 01-1…01-4) from the Core API: KPI tiles that open the job list,
 * the period's progress by status, what needs the company's action, today's timeline, the team's capacity and the
 * recent job activity. The period (URL from/to) applies to every section, as on the job list. */
export function OverviewView({ live }: { live: OverviewLive }) {
  const patch = useUrlPatch();
  const { progress: pr } = live;
  const period = live.period.label === "Period" ? `${live.period.from.slice(5)} – ${live.period.to.slice(5)}` : live.period.label.toLowerCase();
  const unassigned = live.actions.filter((a) => a.kind === "unassigned").length;
  const anyBlock = live.timeline.some((r) => r.blocks.length);
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-end gap-3 text-xs text-muted">
        <span>Updated {live.updated} · your company’s jobs only</span>
        <Select aria-label="Period" className="w-auto" value={`${live.period.from}|${live.period.to}`} onChange={(e) => { const [from, to] = e.target.value.split("|"); patch({ from, to }); }}>
          {live.period.label === "Period" && <option value={`${live.period.from}|${live.period.to}`}>{live.period.from} – {live.period.to}</option>}
          {live.periods.map((p) => <option key={p.label} value={`${p.from}|${p.to}`}>{p.label} ({p.from.slice(5)} – {p.to.slice(5)})</option>)}
        </Select>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "170px" }}>
        {live.kpis.map((k) => <Kpi key={k.label} label={k.label} value={k.value} sub={k.sub} href={k.href} link={k.link} tone={k.tone} />)}
      </div>
      <Card title={`Job progress — ${period}`} action={<TextLink href="/partner/jobs">Open all jobs →</TextLink>}>
        <p className="mb-2 text-xs text-muted">{pr.total ? `${pr.total} job${pr.total === 1 ? "" : "s"} · ${pr.counts.completed} completed (${pr.completedPct}%) · counts use the same period as the Jobs list` : "No jobs yet — offers from HQ to your company will appear here and in Jobs."}</p>
        <div className="flex h-3 w-full overflow-hidden rounded-full bg-surface2" role="img" aria-label="Jobs by status">
          {pr.total > 0 && BUCKETS.map((b) => pr.counts[b.id] > 0 && <div key={b.id} className={cx("h-full", b.tone)} style={{ width: `${(pr.counts[b.id] / pr.total) * 100}%` }} title={`${b.label}: ${pr.counts[b.id]}`} />)}
        </div>
        <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
          {BUCKETS.map((b) => <li key={b.id} className="flex items-center gap-1.5"><span className={cx("inline-block h-2.5 w-2.5 rounded-full", b.tone)} />{b.label} <b className="text-ink">{pr.counts[b.id]}</b></li>)}
        </ul>
      </Card>
      <div className="split">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`Needs your action (${live.actions.length})`}>
            {live.actions.length === 0 ? <EmptyState title="Nothing needs your action">Offers, unassigned jobs and reports to review will be listed here.</EmptyState> : (
              <ul className="divide-y divide-line">
                {live.actions.map((a) => (
                  <li key={`${a.kind}-${a.href}`} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
                    <span className="flex min-w-0 items-center gap-3">
                      <Badge tone={badgeTone[a.kind]}>{a.badge}</Badge>
                      <span className="min-w-0"><b className="block truncate text-[13px]">{a.title}</b><span className="block text-xs text-muted">{a.detail}</span></span>
                    </span>
                    <LinkBtn size="sm" href={a.href}>{a.button}</LinkBtn>
                  </li>
                ))}
              </ul>
            )}
          </Card>
          <Card title={`Team capacity — ${period}`} action={<TextLink href="/partner/team">Team & capacity →</TextLink>}>
            {live.capacity.length === 0 ? <p className="text-[13px] text-muted">No technicians in your company yet.</p> : live.capacity.map((c) => (
              <div key={c.id} className="mb-3"><div className="mb-1 flex justify-between text-xs"><b>{c.name}</b><span className="text-muted">{c.text}</span></div><UtilBar pct={c.pct} tone={c.pct !== null && c.pct >= 90 ? "warn" : "primary"} /></div>
            ))}
            <p className="text-[11px] text-muted">Utilization = assigned hours ÷ available hours. Undefined hours show “—”.</p>
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={`Today · ${live.today.weekday} ${live.today.date}`} action={<TextLink href="/partner/schedule">Open schedule →</TextLink>}>
            <div className="scroll-x"><div className="min-w-[320px]">
              <div className="grid grid-cols-[104px_1fr] text-[11px] text-muted"><span /><div className="flex justify-between">{hours.map((h) => <span key={h}>{String(h).padStart(2, "0")}</span>)}</div></div>
              {live.timeline.length === 0 && <p className="py-2 text-[13px] text-muted">No technicians in your company yet.</p>}
              {live.timeline.map((r) => (
                <div key={r.id} className="grid grid-cols-[104px_1fr] items-center border-t border-line py-2 text-xs">
                  <span><b>{r.name}</b><span className="block text-[11px] text-muted">{r.sub}</span></span>
                  <div className="relative h-8 rounded-lg bg-surface2">
                    {r.blocks.map((b) => <div key={b.text} className="absolute top-1 h-6 truncate rounded bg-primary px-2 text-[11px] leading-6 text-white" style={{ left: `${b.left}%`, width: `${b.width}%` }}>{b.text}</div>)}
                    {!r.blocks.length && <span className="absolute inset-0 grid place-items-center text-[11px] text-muted">{r.off ? `Unavailable · ${r.off}` : "Free all day"}</span>}
                    {live.today.nowPct > 0 && live.today.nowPct < 100 && <span className="absolute inset-y-0 w-px bg-crit" style={{ left: `${live.today.nowPct}%` }} aria-hidden />}
                  </div>
                </div>
              ))}
            </div></div>
            <p className="mt-2 text-[11px] text-muted">Red line = now ({live.today.now}).{anyBlock ? " Blue = assigned work windows (members.capacity)." : " No assignments today."}</p>
            {unassigned > 0 && <div className="mt-2"><Banner tone="warn">{unassigned} accepted job{unassigned === 1 ? " still has" : "s still have"} no technician — assign {unassigned === 1 ? "it" : "them"} before the delegation period ends.</Banner></div>}
          </Card>
          <Card title="Recent activity" action={<TextLink href="/partner/history">Job history →</TextLink>}>
            {live.activity.length === 0 ? <p className="text-[13px] text-muted">No job activity in this period.</p> : (
              <ul className="flex flex-col gap-2 text-[13px]">
                {live.activity.map((a) => <li key={a.id} className="grid grid-cols-[86px_1fr] gap-2"><span className="text-xs text-muted">{a.time}</span><Link href={a.href} className="hover:text-primary">{a.text}</Link></li>)}
              </ul>
            )}
          </Card>
        </div>
      </div>
    </Page>
  );
}
