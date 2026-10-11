"use client";

import { Btn, Check, Field, Input, LineChart, Select, cx } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { windowText } from "@ac/web/lib/customerPolicies";
import { HQ_METRICS, metricLabel, metricUnit, opSymbol, type ApiPolicy, type Operator } from "@ac/web/lib/adminAlerts";

type Window = NonNullable<ApiPolicy["activeWindow"]>;
/** A condition as both editors hold it: an alert policy's and a default rule's (IR120 item 3). */
export type Condition = { metric: string; operator: Operator; threshold: number; recovery: number; duration: number };

const OPERATORS: Operator[] = ["gte", "gt", "lte", "lt"];
const DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const RISE = [0.67, 0.8, 0.93, 1.03, 1.07, 1.03, 0.97, 0.9, 0.83];
const FALL = [1.2, 1.1, 1.0, 0.95, 0.93, 0.97, 1.03, 1.1, 1.15];

/** The condition (Figma Admin 38:5 / 268:2): metric, comparison, threshold, duration and recovery, the chart, and
 * whether the recovery lies on the safe side. A default rule's metric is fixed (`fixedMetric`). */
export function ConditionFields({ c, onChange, errors, tried, fixedMetric }: { c: Condition; onChange: (p: Partial<Condition>) => void; errors: Record<string, string>; tried: boolean; fixedMetric?: boolean }) {
  const t = useT();
  const unit = metricUnit[c.metric] ?? "";
  return (
    <>
      <div className="grid-fluid" style={{ ["--min" as string]: "130px" }}>
        {fixedMetric
          ? <Field label={t("Metric")} hint={unit && t("unit fixed: {unit}", { unit })}><Input readOnly value={metricLabel[c.metric] ? t(metricLabel[c.metric]) : c.metric} /></Field>
          : <Field label={t("Metric")}><Select value={c.metric} onChange={(e) => onChange({ metric: e.target.value })}>{HQ_METRICS.map((k) => <option key={k} value={k}>{t(metricLabel[k])}</option>)}</Select></Field>}
        <Field label={t("Comparison")}><Select value={c.operator} onChange={(e) => onChange({ operator: e.target.value as Operator })}>{OPERATORS.map((o) => <option key={o} value={o}>{opSymbol[o]}</option>)}</Select></Field>
        <Field label={t("Alert when {op}", { op: opSymbol[c.operator] })} error={tried ? errors.threshold : undefined}><Input type="number" value={c.threshold} onChange={(e) => onChange({ threshold: +e.target.value })} /></Field>
        <Field label={t("For (s)")} error={tried ? errors.duration : undefined}><Input type="number" value={c.duration} onChange={(e) => onChange({ duration: +e.target.value })} /></Field>
        <Field label={t("Recover")} error={errors.recovery}><Input type="number" value={c.recovery} onChange={(e) => onChange({ recovery: +e.target.value })} /></Field>
      </div>
      {/* an illustrative curve that crosses the threshold and comes back (not real readings) */}
      <div className="mt-2"><LineChart points={(c.operator === "lt" || c.operator === "lte" ? FALL : RISE).map((k) => Math.round(k * c.threshold * 10) / 10)} threshold={c.threshold} height={90} /></div>
      <p className={cx("text-xs", errors.recovery ? "text-crit" : "text-ok")}>{errors.recovery ? `✕ ${errors.recovery}` : t("✓ Recovery {recovery} is on the safe side of the {threshold} threshold ({op})", { recovery: c.recovery, threshold: c.threshold, op: opSymbol[c.operator] })}</p>
    </>
  );
}

/** “Only if …” (DD-A05 item 10): the days and local hours in the policy's time zone, or any time (null). */
export function WindowEditor({ value, onChange, timezone, error }: { value: Window | null | undefined; onChange: (w: Window | null) => void; timezone: string; error?: string }) {
  const t = useT();
  const set = (w: Partial<Window>) => value && onChange({ ...value, ...w });
  return (
    <div className="flex flex-col gap-2">
      <Check label={t("Only on some days and times")} checked={!!value} onChange={(v) => onChange(v ? { weekdays: [1, 2, 3, 4, 5], startLocal: "08:00", endLocal: "19:00" } : null)} />
      {value ? (
        <div className="flex flex-wrap items-center gap-2">
          <Btn size="sm" onClick={() => set({ weekdays: [1, 2, 3, 4, 5] })}>{t("Mon–Fri")}</Btn><Btn size="sm" onClick={() => set({ weekdays: [1, 2, 3, 4, 5, 6, 7] })}>{t("Every day")}</Btn>
          {DAYS.map((d, i) => <button key={d} type="button" aria-pressed={value.weekdays.includes(i + 1)} onClick={() => set({ weekdays: value.weekdays.includes(i + 1) ? value.weekdays.filter((x) => x !== i + 1) : [...value.weekdays, i + 1].sort() })} className={cx("rounded-control border px-2 py-1 text-xs font-semibold", value.weekdays.includes(i + 1) ? "border-primary bg-primary-soft text-primary" : "border-line")}>{t(d)}</button>)}
          <Input aria-label={t("From")} type="time" className="w-32" value={value.startLocal} onChange={(e) => set({ startLocal: e.target.value })} /> – <Input aria-label={t("To")} type="time" className="w-32" value={value.endLocal} onChange={(e) => set({ endLocal: e.target.value })} />
          <span className="text-xs text-muted">{timezone}</span>
        </div>
      ) : <p className="text-[13px]">{t("Any day · any time")}</p>}
      {value && !error && <p className="text-xs text-muted">{windowText(value, t)}</p>}
      {error && <p className="text-xs text-crit">✕ {error}</p>}
    </div>
  );
}
