"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Badge, Banner, Btn, Card, ConnBadge, cx, DataTable, EmptyState, Field, LineChart, LinkBtn, Modal, Page, Select, SummaryList, Tabs } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { useAction } from "@ac/web/lib/useAction";
import { useUrlPatch } from "@ac/web/lib/useUrlPatch";
import {
  airMetrics, airNumber, airPeriods, metricInfo, ventDurations, ventMethods, type AirMetric, type AirPeriod, type AirRow, type MetricCard, type VentMethod,
} from "@ac/web/lib/air";
import { logVentilation } from "../actions";

type Tone = MetricCard["feed"]["tone"];
export type AirLive = {
  rooms: { key: string; spaceId: string | null; label: string; units: { id: string; name: string }[] }[];
  roomKey: string; unitId: string; unitName: string; path: string; spaceId: string | null;
  connection: "online" | "offline" | "unknown" | "connecting" | "error"; updated: string; freshAir: boolean;
  cards: MetricCard[]; strip: { tone: "warn" | "primary"; title: string; text: string }; clean: string | null;
  allergen: { title: string; badge: { text: string; tone: Tone; icon: string }; text: string };
  metric: AirMetric; period: AirPeriod;
  chart: {
    title: string; sub: string; points: (number | null)[]; labels: string[]; guide: number | null; min: number; max: number; ticks: number[];
    hasSensor: boolean; readings: number; total: number; valid: boolean; gaps: boolean;
  };
  rows: AirRow[]; co2Now: string | null;
  history: { rows: { id: string; text: string; co2: string; by: string; when: string }[]; total: number } | null;
};

/** Air quality (FR-C07) in API mode: the room's sensor readings with read-time quality, the IR99 guidance, the IR98
 * allergen observation, the series of one metric and the manual ventilation log (IR110 — nothing is sent to the AC).
 * Texts in the user's display language (IR266). */
export function AirQualityView({ live }: { live: AirLive | null }) {
  const t = useT();
  const patch = useUrlPatch();
  const router = useRouter();
  const [view, setView] = useState<"chart" | "table">("chart");
  const [open, setOpen] = useState(false);
  if (!live) {
    return <Page><EmptyState title={t("No air conditioners yet")} action={<LinkBtn href="/customer/properties" size="sm">{t("Units & locations")}</LinkBtn>}>{t("Air quality is read from the sensors of your units. Once a unit is registered its readings appear here.")}</EmptyState></Page>;
  }
  const room = live.rooms.find((r) => r.key === live.roomKey)!;
  const c = live.chart;
  const info = metricInfo[live.metric];
  const label = t(info.label);
  const pickRoom = (key: string) => {
    const r = live.rooms.find((x) => x.key === key);
    if (r) patch({ spaceId: r.spaceId, unitId: r.units[0].id });
  };
  const noValid = c.readings === 0 ? t("No valid {metric} readings in this window — missing data is never drawn as a line.", { metric: label })
    : t(c.readings === 1 ? "No valid {metric} readings in this window (1 reading without a valid value) — missing data is never drawn as a line." : "No valid {metric} readings in this window ({n} readings without a valid value) — missing data is never drawn as a line.", { metric: label, n: c.readings });
  return (
    <Page>
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-line bg-surface px-4 py-3 text-xs">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted">{t("Viewing")}</span>
          <Select aria-label={t("Room")} className="w-auto py-1.5 font-semibold" value={live.roomKey} onChange={(e) => pickRoom(e.target.value)}>
            {live.rooms.map((r) => <option key={r.key} value={r.key}>{t("Room: {room}", { room: r.label })}</option>)}
          </Select>
          <Select aria-label={t("Sensor")} className="w-auto py-1.5 font-semibold" value={live.unitId} onChange={(e) => patch({ unitId: e.target.value })}>
            {room.units.map((u) => <option key={u.id} value={u.id}>{t("Sensor: {name}", { name: u.name })}</option>)}
          </Select>
        </div>
        <span className="flex items-center gap-2"><b>{t("Updated {time}", { time: live.updated })}</b><ConnBadge s={live.connection} /><Btn size="sm" variant="ghost" onClick={() => router.refresh()}>{t("↻ Refresh")}</Btn></span>
      </div>
      <div className="grid-fluid" style={{ ["--min" as string]: "210px" }}>
        {live.cards.map((m) => (
          <button key={m.metric} type="button" aria-pressed={m.metric === live.metric} onClick={() => patch({ metric: m.metric })}
            className={cx("flex min-w-0 flex-col gap-1 rounded-2xl border bg-surface p-4 text-left", m.metric === live.metric ? "border-primary ring-1 ring-primary" : "border-line hover:border-primary/50")}>
            <span className="flex items-center justify-between gap-2 text-xs text-muted">{m.label}<Badge tone={m.feed.tone} icon={m.feed.icon}>{m.feed.text}</Badge></span>
            <span className="text-[28px] font-bold leading-9">{m.value ?? "—"}{m.value !== null && <span className="ml-1 text-sm font-medium text-muted">{m.unit}</span>}</span>
            <span><Badge tone={m.status.tone} icon={m.status.icon}>{m.status.text}</Badge></span>
            <span className="text-[11px] text-muted">{m.sub}</span>
            <span className={cx("text-xs font-semibold", m.warn && "text-warn")}>{m.advice}</span>
          </button>
        ))}
      </div>
      <Card><div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px]"><b>{live.allergen.title}</b><Badge tone={live.allergen.badge.tone} icon={live.allergen.badge.icon}>{live.allergen.badge.text}</Badge><span className="text-xs text-muted">{live.allergen.text}</span></div></Card>
      {live.clean && <Banner tone="warn" action={<LinkBtn size="sm" href="/customer/maintenance">{t("Maintenance")}</LinkBtn>}>{live.clean}</Banner>}
      <Banner tone={live.strip.tone} action={<Btn size="sm" variant={live.strip.tone === "warn" ? "primary" : "secondary"} disabled={!live.spaceId} title={live.spaceId ? undefined : t("Logs belong to a room — this unit is not in one")} onClick={() => setOpen(true)}>{t("Log ventilation")}</Btn>}>
        <b>{live.strip.title}</b><div className="text-xs text-muted">{live.strip.text}{!live.spaceId && ` ${t("Ventilation logs belong to a room — HQ sets up the rooms, so ask HQ to place this unit in one.")}`}</div>
      </Banner>
      <Card title={c.title} sub={c.sub} action={<div className="flex flex-wrap gap-2">
        <Tabs value={live.metric} onChange={(m) => patch({ metric: m })} tabs={airMetrics.map((k) => ({ id: k, label: t(metricInfo[k].tab) }))} />
        <Tabs value={live.period} onChange={(p) => patch({ period: p })} tabs={airPeriods.map((p) => ({ id: p, label: t(p) }))} />
        <Tabs value={view} onChange={setView} tabs={[{ id: "chart", label: t("Chart") }, { id: "table", label: t("Table") }]} />
      </div>}>
        {!c.hasSensor ? <p className="text-[13px] text-muted">{t("{unit} has no {metric} sensor — nothing to chart.", { unit: live.unitName, metric: label })}</p>
          : !c.valid ? <p className="text-[13px] text-muted">{noValid}</p>
          : view === "chart" ? (
            <>
              <LineChart points={c.points} min={c.min} max={c.max} threshold={c.guide ?? undefined} ticks={c.ticks} shadeGaps width={1200} height={300} labels={c.labels} />
              <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted">
                <span className="text-primary">— {label} ({info.unit})</span>
                {c.guide !== null && <span className="text-crit">- - {c.guide} {info.unit} {t(live.metric === "co2" ? "ventilation guide" : "cleaning guide")}</span>}
                {c.gaps && <span>{t("▧ No data (gap, not joined)")}</span>}
              </div>
            </>
          ) : (
            <div className="max-h-[420px] overflow-y-auto">
              <DataTable rows={live.rows} rowKey={(r) => r.key} cols={[
                { key: "t", label: t("Time"), render: (r) => r.time },
                { key: "v", label: `${label} (${info.unit})`, render: (r) => (r.gap ? <span className="text-muted">{r.value}</span> : r.value) },
                { key: "n", label: t("Readings"), render: (r) => <span className="text-xs text-muted">{r.note}</span> },
              ]} />
            </div>
          )}
        {c.hasSensor && c.total > c.readings && <p className="mt-2 text-xs font-semibold text-warn">{t("Showing the latest {n} of {total} readings — earlier readings are not loaded, so the start of the window is incomplete.", { n: c.readings, total: c.total })}</p>}
      </Card>
      <Card title={t("Ventilation history")} sub={live.history ? `${live.path} · ${t(live.history.total === 1 ? "{n} log" : "{n} logs", { n: live.history.total })} · ${t("manual records, nothing sent to the AC or to HQ")}` : live.path}>
        {!live.history ? <p className="text-[13px] text-muted">{t("Ventilation logs belong to a room — {unit} is not in one.", { unit: live.unitName })}</p>
          : live.history.rows.length === 0 ? <p className="text-[13px] text-muted">{t("No ventilation logged in this room yet.")}</p>
          : live.history.rows.map((l) => (
            <div key={l.id} className="flex flex-wrap justify-between gap-2 border-b border-line py-2 text-[13px] last:border-0">
              <span>{l.text} <span className="text-xs text-muted">· {l.co2}</span></span><span className="text-xs text-muted">{l.when} · {l.by}</span>
            </div>
          ))}
        {live.history && live.history.total > live.history.rows.length && <p className="mt-1 text-xs text-muted">{t("Showing the latest {n}.", { n: live.history.rows.length })}</p>}
      </Card>
      {open && live.spaceId && <LogModal spaceId={live.spaceId} unitId={live.unitId} path={live.path} co2Now={live.co2Now} onClose={() => setOpen(false)} />}
    </Page>
  );
}

/** Log ventilation (Figma Client 05c): room, method, duration and the current CO2 the record keeps (co2AtLog). */
function LogModal({ spaceId, unitId, path, co2Now, onClose }: { spaceId: string; unitId: string; path: string; co2Now: string | null; onClose: () => void }) {
  const t = useT();
  const [pending, run] = useAction();
  const [method, setMethod] = useState<VentMethod>("window_opened");
  const [minutes, setMinutes] = useState(15);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const save = () => run(() => logVentilation({ spaceId, unitId, method, durationMinutes: minutes }),
    (v) => (v.co2AtLog ? t("Ventilation logged · CO2 {value} ppm recorded", { value: airNumber("co2", v.co2AtLog.value) }) : t("Ventilation logged · no current CO2 reading recorded")), onClose,
    (f) => setErrors(f.fieldErrors)); // VALIDATION keeps the modal and the input
  return (
    <Modal open title={t("Log ventilation")} onClose={onClose} footer={<><Btn onClick={onClose}>{t("Cancel")}</Btn><Btn variant="primary" disabled={pending} onClick={save}>{t("Save log")}</Btn></>}>
      <SummaryList items={[
        [t("Room"), path],
        [t("Method"), <Field key="m" label={t("Method")} className="[&>span:first-child]:sr-only" error={errors.method && t("Choose a method")}><Select className="w-auto self-start py-1" value={method} onChange={(e) => setMethod(e.target.value as VentMethod)}>{ventMethods.map((m) => <option key={m.id} value={m.id}>{t(m.label)}</option>)}</Select></Field>],
        [t("Duration"), <Field key="d" label={t("Duration")} className="[&>span:first-child]:sr-only" error={errors.durationMinutes && t("1–240 minutes")}><Select className="w-auto self-start py-1" value={minutes} onChange={(e) => setMinutes(Number(e.target.value))}>{ventDurations.map((n) => <option key={n} value={n}>{t("{n} min", { n })}</option>)}</Select></Field>],
        [t("Current CO2"), co2Now ?? t("Not measured — the log keeps no CO2 value")],
      ]} />
      <p className="text-xs text-muted">{t("Saved to this room’s ventilation log with your name and time. Nothing is sent to the AC or to HQ — keep watching the chart to see whether CO2 drops.")}</p>
    </Modal>
  );
}
