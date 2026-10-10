"use client";

import Link from "next/link";
import { Badge, Banner, Card, ConnBadge, LineChart, LinkBtn, Page, SeverityBadge, SummaryList, Tabs, TextLink, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import type { Period, SeriesChart, Tile } from "@ac/web/lib/techUnit";
import type { UnitLive } from "../_lib/load";

/** The technician's unit (FR-T02, FR-T03, Figma Technician 02-10…02-13): the register with its components, the
 * maintenance history and open alerts, the latest values and one metric over a period — the metric and the period in
 * the URL (`metric` opens Monitoring, `period`). Texts in the user's display language; the times come formatted from
 * the loader (IR283). */
export function TechUnitView({ live }: { live: UnitLive }) {
  const t = useT();
  const patch = useUrlPatch();
  const setPeriod = (p: Period) => patch({ period: p === "24h" ? null : p });
  const periods = <Tabs value={live.period} onChange={setPeriod} tabs={live.periods} />;
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px]">
        <Link href={live.back} className="font-semibold text-primary" aria-label={t("Back")}>‹</Link><h1 className="text-base font-bold">{live.name}</h1><span className="text-muted">· {live.short}</span><ConnBadge s={live.connection} />
      </div>
      <div><Tabs value={live.tab} onChange={(v) => patch({ metric: v === "monitoring" ? live.metric : null })} tabs={live.tabs} /></div>
      {live.tab === "register" ? (
        <div className="split-rev">
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={t("Unit register")}>
              <SummaryList items={live.register.rows} />
              <p className="mt-2 rounded-xl bg-primary-soft/40 px-3 py-2 text-[11px] text-muted">ⓘ {live.register.note}</p>
              <div className="mt-3 flex flex-col gap-2">
                <LinkBtn href={live.alertsHref} size="sm">{t("Alert evidence →")}</LinkBtn>
                {live.controlHref ? <LinkBtn href={live.controlHref} size="sm">{t("Diagnostics →")}</LinkBtn> : <p className="text-[11px] text-muted">{t("Diagnostic control opens for a job you are assigned to on this AC (assigned or in progress).")}</p>}
              </div>
              {live.register.missing && <p className="mt-2 text-[11px] text-muted">{t("Missing values are never filled from similar models or today’s date (BR-T02).")}</p>}
            </Card>
            <Card title={t("Components ({n})", { n: live.componentCount })}>
              {live.components.length === 0 ? <p className="text-[13px] text-muted">{t("No components in this unit’s service scope.")}</p> : live.components.map((c) => (
                <div key={c.group} className="border-t border-line py-2 first:border-0"><b className="text-[13px]">{c.title}</b><div className="text-xs text-muted">{c.names}</div></div>
              ))}
              <p className="mt-1 text-[11px] text-muted">{t("Each component gets its result in the job workspace — normal is never preselected.")}</p>
            </Card>
          </div>
          <div className="flex min-w-0 flex-col gap-4">
            <Card title={t("Time-series monitoring")} action={periods}>
              {live.stopped && <div className="mb-3"><Banner tone="warn">{live.stopped}</Banner></div>}
              <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>{[live.tiles.temperature, live.tiles.power, live.tiles.connection].map((x) => <TileBox key={x.label} tile={x} />)}</div>
              <Series chart={live.chart} width={700} />
            </Card>
            <Card title={t("Maintenance history")}>
              {live.history.length === 0 ? <p className="text-[13px] text-muted">{t("No jobs of yours on this unit.")}</p> : live.history.map((j) => (
                <Link key={j.id} href={`/technician/jobs/${j.id}`} className="flex items-center justify-between gap-2 border-t border-line py-2 first:border-0 hover:bg-surface2/60">
                  <span className="min-w-0"><b className="block text-[13px]">{j.title}</b><span className="text-xs text-muted">{j.sub}</span></span><Badge tone={j.badge.tone}>{j.badge.text}</Badge>
                </Link>
              ))}
              <p className="mt-1 text-[11px] text-muted">{t("Only your own assignments on this unit are listed.")}</p>
            </Card>
            <Card title={t("Open alerts ({n})", { n: live.alerts.length })} action={<TextLink href={live.alertsHref}>{t("Evidence →")}</TextLink>}>
              {live.alerts.length === 0 ? <p className="text-[13px] text-muted">{t("No open alerts.")}</p> : live.alerts.map((a) => (
                <Link key={a.id} href={a.href} className="block border-t border-line py-2 first:border-0 hover:bg-surface2/60">
                  <div className="flex items-start justify-between gap-2"><span className="min-w-0"><b className="block text-[13px]">{a.title}</b><span className="text-xs text-muted">{a.sub}</span></span><SeverityBadge s={a.severity} /></div>
                  <p className="mt-1 text-[11px] text-muted">{a.ack}</p>
                </Link>
              ))}
            </Card>
          </div>
        </div>
      ) : (
        <>
          <Card title={live.chart.title} action={periods}>
            {live.stopped && <div className="mb-3"><Banner tone="warn">{live.stopped}</Banner></div>}
            <div className="grid-fluid" style={{ ["--min" as string]: "160px" }}>{[live.tiles.temperature, live.tiles.power, live.tiles.operation, live.tiles.connection].map((x) => <TileBox key={x.label} tile={x} />)}</div>
            <Series chart={live.chart} width={1000} />
          </Card>
          {live.others.length > 0 && (
            <div className="grid-fluid" style={{ ["--min" as string]: "260px" }}>
              {live.others.map((m) => (
                <Card key={m.metric} title={m.label} action={<b className={cx("text-[13px]", m.tone === "warn" && "text-warn")}>{m.value}</b>}>
                  {m.points.every((v) => v === null) ? <p className="text-xs text-muted">{t("No valid readings in this period.")}</p> : <LineChart points={m.points} height={56} width={500} />}
                  <div className="mt-1 flex justify-between gap-2 text-[11px] text-muted"><span>{m.sub}</span><button type="button" className="font-semibold text-primary" onClick={() => patch({ metric: m.metric })}>{t("Chart →")}</button></div>
                </Card>
              ))}
            </div>
          )}
          <Card title={t("Events in this period")}>
            {live.events.length === 0 ? <p className="text-[13px] text-muted">{t("No alert or device events in this period.")}</p> : live.events.map((e) => (
              <div key={e.id} className="flex items-start justify-between gap-2 border-t border-line py-2 first:border-0">
                <span className="min-w-0"><b className="block text-[13px]">{e.time} · {e.title}</b>{e.sub && <span className="text-xs text-muted">{e.sub}</span>}</span><Badge tone={e.badge.tone}>{e.badge.text}</Badge>
              </div>
            ))}
          </Card>
        </>
      )}
    </Page>
  );
}

function TileBox({ tile }: { tile: Tile }) {
  return (
    <div className="rounded-xl bg-surface2 p-3">
      <div className="text-[11px] text-muted">{tile.label}</div>
      <b className={cx("text-lg", tile.tone === "crit" && "text-crit", tile.tone === "warn" && "text-warn")}>{tile.value}</b>
      <div className="text-[11px] text-muted">{tile.sub}</div>
    </div>
  );
}

function Series({ chart, width }: { chart: SeriesChart; width: number }) {
  const t = useT();
  return (
    <div className="mt-3">
      {chart.empty ? <div className="rounded-xl border border-dashed border-line p-6 text-center text-[13px] text-muted">{t("No valid readings in this period.")}</div> : <LineChart points={chart.points} height={200} width={width} labels={chart.labels} />}
      <p className="mt-1 text-[11px] text-muted">{[chart.count, chart.gap, t("Gaps in missing data are not connected by lines. Reordered or duplicate demo events never roll values backward.")].filter(Boolean).join(" · ")}</p>
    </div>
  );
}
