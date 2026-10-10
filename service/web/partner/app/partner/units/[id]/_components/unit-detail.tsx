"use client";

import Link from "next/link";
import { Badge, Banner, BarChart, Card, ConnBadge, LinkBtn, Page, SeverityBadge, SummaryList, UtilBar, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import type { UnitLive } from "../_lib/load";

/** The contractor's unit view (FR-P04, FR-P08, Figma Contractor Unit View): diagnosis only — the register, the job
 * context with its access window, past work, alert evidence, readings and the last 24 hours — or, after the
 * delegation, the job's history snapshot. Texts in the user's display language; the times come formatted from the
 * loader (IR280). */
export function PartnerUnitDetail({ live }: { live: UnitLive }) {
  const t = useT();
  if (live.kind === "snapshot") {
    return (
      <Page>
        <Banner tone="crit">{live.banner}</Banner>
        <Card title={live.title} className="max-w-xl">
          <SummaryList items={live.rows} />
          <p className="mt-2 text-[11px] text-muted">{t("Only your own acceptance / decline and work records are kept. Past access never re-opens customer data.")}</p>
          <div className="mt-3"><LinkBtn href="/partner/jobs" variant="primary" size="sm">{t("Back to Jobs")}</LinkBtn></div>
        </Card>
      </Page>
    );
  }
  const conn = live.register.connection;
  return (
    <Page>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted"><Link href={live.context ? `/partner/jobs/${live.context.id}` : "/partner/jobs"} className="font-semibold text-primary">‹</Link><b className="text-ink">{live.name}</b><span>· {live.id.slice(0, 8)}</span></div>
      <Banner>{live.banner}</Banner>
      <div className="split-rev">
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Unit register")}>
            <SummaryList items={[...live.register.rows, [t("Connection"), <ConnBadge key="c" s={conn} />], [t("Last seen"), live.register.lastSeen]]} />
            <p className="mt-2 rounded-xl bg-primary-soft/40 px-3 py-2 text-[11px] text-muted">{t("Read-only — no remote-control actions available to contractors.")}</p>
          </Card>
          <Card title={t("Job context")} action={live.context && <Link className="text-xs font-semibold text-primary" href={`/partner/jobs/${live.context.id}`}>{t("Job →")}</Link>}>
            {!live.context ? <p className="text-[13px] text-muted">{t("No current job of your company on this unit.")}</p> : (
              <>
                <SummaryList items={live.context.rows.map(([k, v], n): [string, React.ReactNode] => [k, n === 1 && live.context!.techMissing ? <span key="t" className="font-semibold text-crit">{v}</span> : v])} />
                {live.context.pct !== null && <div className="mt-2"><UtilBar pct={live.context.pct} tone={live.context.pct >= 80 ? "crit" : "primary"} /></div>}
                <p className="mt-2 text-[11px] text-muted">{live.context.note}</p>
              </>
            )}
          </Card>
          <Card title={t("Our past work on this unit")}>
            {live.past.length === 0 ? <p className="text-[13px] text-muted">{t("No past jobs.")}</p> : live.past.map((j) => (
              <div key={j.id} className="flex items-center justify-between gap-2 border-t border-line py-2 text-[13px] first:border-0"><span><b className="block">{j.title}</b><span className="text-[11px] text-muted">{j.sub}</span></span><Badge tone={j.badge.tone}>{j.badge.text}</Badge></div>
            ))}
            <p className="mt-1 text-[11px] text-muted">{t("Only your company’s own jobs are listed — other contractors’ work and customer billing are hidden.")}</p>
          </Card>
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <Card title={t("Alert evidence")}>
            {live.alerts.length === 0 ? <p className="text-[13px] text-muted">{t("No alerts on this unit.")}</p> : live.alerts.map((a) => (
              <div key={a.id} className={cx("border-t border-line py-2 first:border-0", a.resolved && "opacity-70")}><b className="text-[13px]">⚠ {a.title}</b> <SeverityBadge s={a.severity} /><div className="text-xs text-muted">{a.sub}</div></div>
            ))}
          </Card>
          <Card title={t("Readings (diagnosis only)")} sub={t("observed {time}", { time: live.readings.observed })}>
            <SummaryList items={live.readings.rows.length ? live.readings.rows.map((r): [string, React.ReactNode] => [r.label, <span key={r.label} className={cx("font-semibold", r.tone === "crit" && "text-crit", r.tone === "warn" && "text-warn")}>{r.value}</span>]) : [[t("Measurements"), t("No measurements yet")]]} />
          </Card>
          <Card title={t("Readings — last 24 h")} sub={live.window}>
            {live.charts.length === 0 ? <p className="text-[13px] text-muted">{t("No diagnosis readings for this unit.")}</p> : (
              <div className="flex flex-col gap-4">{live.charts.map((c) => (
                <div key={c.metric}>
                  <div className="mb-1 flex justify-between text-xs font-semibold"><span>{c.title}</span><span className="text-muted">{c.now}</span></div>
                  <BarChart labels={c.labels} series={[c.values]} colors={["#9ec0f5"]} height={110} />
                </div>
              ))}</div>
            )}
            <p className="mt-2 text-[11px] text-muted">{t("Diagnosis only — read-only values, no control.")}</p>
          </Card>
        </div>
      </div>
    </Page>
  );
}
