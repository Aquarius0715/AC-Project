// HQ SLA by customer (FR-A22, DD-A22, IR131 items 4–5, IR236, Figma Admin 06-10 699:21404) from the Core API: the
// period and contractor filters, the KPI tiles with the targets of the plan types in the scorecard, one row per
// customer, the recent breaches, the targets per plan type for the edit dialog and the CSV export. Pure code shared by
// the server loader and the client view; Vitest covers it. Texts in the display language (`i` / `t`, IR291): a breach
// is phrased from its kind and its response minutes, and the targets' start times are instants in the display time
// zone, as typed (NFR-08).
import { EN, showDate, showTime, translator, zonedParts, type I18n, type T } from "@ac/web/lib/i18n";
import { planLabel } from "@ac/web/lib/contracts";

const en = translator("en");

type PlanType = "rto" | "general" | "energy" | "environment";
export type ApiSlaMetrics = { responseWithinTarget: number | null; arrivalInWindow: number | null; firstTimeFix: number | null; averageRating: number | null; ratingCount: number; openOverdue: number };
export type ApiTargetView = { planType: PlanType; responseHours: number; arrivalInWindowPercent: number; firstTimeFixPercent: number; version: number; effectiveFrom: string | null; state: "in_effect" | "default" | "scheduled" };
export type ApiBreach = { jobId: string; customerId: string; kind: "response" | "arrival" | "first_time_fix" | "overdue"; detail: string; tookMinutes?: number | null; limitMinutes?: number | null };
export type ApiScorecard = {
  period: { from: string; to: string }; contractorOrgId: string | null; totals: ApiSlaMetrics;
  customers: (ApiSlaMetrics & { customerId: string; planType: PlanType; jobCount: number; status: "on_track" | "at_risk" | "breached" })[];
  breaches: ApiBreach[];
  targets: ApiTargetView[];
};
type Tone = "ok" | "warn" | "crit" | "muted";

export const PERIODS = [{ id: "30", label: "last 30 days", days: 30 }, { id: "90", label: "last 90 days", days: 90 }, { id: "365", label: "last 12 months", days: 365 }] as const;
/** The scorecard period of the URL key (default the last 90 days, DD-A22). */
export function periodOf(key: string | undefined, now: number): { id: string; from: string; to: string } {
  const p = PERIODS.find((x) => x.id === key) ?? PERIODS[1];
  return { id: p.id, from: new Date(now - p.days * 86_400_000).toISOString(), to: new Date(now).toISOString() };
}
const pct = (v: number | null) => (v === null ? "—" : `${Math.round(v)} %`);
const plans = (targets: ApiTargetView[], used: PlanType[]) => targets.filter((x) => x.state !== "scheduled" && used.includes(x.planType));
/** “target 95 %” for one plan type, “targets 85–95 % by plan” when the customers' plans differ. */
function targetText(values: number[], t: T): string {
  const u = [...new Set(values)].sort((a, b) => a - b);
  return !u.length ? t("no target") : u.length === 1 ? t("target {n} %", { n: u[0] }) : t("targets {from}–{to} % by plan", { from: u[0], to: u[u.length - 1] });
}
/** The plan type's name in the user's language. */
const planName = (plan: string, t: T) => (planLabel[plan as PlanType] ? t(planLabel[plan as PlanType]) : plan);

/** The KPI tiles (Figma 06-10) with the targets of the plan types of the customers in the scorecard; no data is “—”. */
export function slaTiles(sc: ApiScorecard, t: T = en): { label: string; value: string; sub: string; tone?: "warn" | "crit" }[] {
  const used = [...new Set(sc.customers.map((c) => c.planType))];
  const ts = plans(sc.targets, used.length ? used : ["general"]);
  const hours = [...new Set(ts.map((x) => x.responseHours))].sort((a, b) => a - b);
  const m = sc.totals;
  const below = (v: number | null, target: number) => (v !== null && v < target ? (target - v > 10 ? "crit" : "warn") : undefined);
  const arrival = Math.min(...ts.map((x) => x.arrivalInWindowPercent));
  const ftf = Math.min(...ts.map((x) => x.firstTimeFixPercent));
  return [
    { label: hours.length === 1 ? t("Response ≤ {n} h", { n: hours[0] }) : t("Response within target"), value: pct(m.responseWithinTarget), sub: hours.length === 1 ? t("target 100 % of jobs") : t("≤ {hours} h by plan · target 100 %", { hours: hours.join(" / ") }), tone: below(m.responseWithinTarget, 100) },
    { label: t("Arrival in window"), value: pct(m.arrivalInWindow), sub: targetText(ts.map((x) => x.arrivalInWindowPercent), t), tone: below(m.arrivalInWindow, arrival) },
    { label: t("First-time fix"), value: pct(m.firstTimeFix), sub: targetText(ts.map((x) => x.firstTimeFixPercent), t), tone: below(m.firstTimeFix, ftf) },
    { label: t("Avg. customer rating"), value: m.averageRating === null ? "—" : `${m.averageRating.toFixed(1)} ★`, sub: t(m.ratingCount === 1 ? "1 rating" : "{n} ratings", { n: m.ratingCount }) },
    { label: t("Open & overdue"), value: String(m.openOverdue), sub: t(sc.breaches.length === 1 ? "1 breach in the period" : "{n} breaches in the period", { n: sc.breaches.length }), tone: m.openOverdue > 0 ? "crit" : undefined },
  ];
}

export type SlaRow = { id: string; name: string; sub: string; plan: string; jobs: number; response: string; arrival: string; ftf: string; rating: string; overdue: number; status: { label: string; tone: Tone } };
const STATUS: Record<string, { label: string; tone: Tone }> = { on_track: { label: "On track", tone: "ok" }, at_risk: { label: "At risk", tone: "warn" }, breached: { label: "Breached", tone: "crit" } };
/** One customer of the scorecard (Figma “customer-a · Home A, Office A · 5 units”). */
export function slaRows(sc: ApiScorecard, customers: { id: string; name: string; organizationId: string }[], properties: { name: string; customerOrgId: string }[], units: { customerOrgId: string }[], t: T = en): SlaRow[] {
  return sc.customers.map((c) => {
    const cust = customers.find((x) => x.id === c.customerId);
    const org = cust?.organizationId ?? "";
    const props = properties.filter((p) => p.customerOrgId === org).map((p) => p.name);
    const n = units.filter((u) => u.customerOrgId === org).length;
    const status = STATUS[c.status];
    return {
      id: c.customerId, name: cust?.name ?? t("customer"),
      sub: `${props.length > 2 ? t("{n} properties", { n: props.length }) : props.join(", ") || t("no properties")} · ${t(n === 1 ? "1 unit" : "{n} units", { n })}`,
      plan: planName(c.planType, t), jobs: c.jobCount, response: pct(c.responseWithinTarget), arrival: pct(c.arrivalInWindow), ftf: pct(c.firstTimeFix),
      rating: c.averageRating === null ? "—" : `${c.averageRating.toFixed(1)} ★`, overdue: c.openOverdue, status: status ? { label: t(status.label), tone: status.tone } : { label: c.status, tone: "muted" },
    };
  });
}

/** “6 h 10 min” / “4 h”, as the Core API writes a breach's English detail. */
const duration = (min: number, t: T) => (min % 60 === 0 ? t("{n} h", { n: Math.floor(min / 60) }) : t("{h} h {m} min", { h: Math.floor(min / 60), m: min % 60 }));
/** A breach in words from its kind and response minutes (IR291); the API's English detail when it has neither. */
export function breachText(b: ApiBreach, t: T = en): string {
  if (b.kind === "response" && b.limitMinutes != null) {
    return b.tookMinutes != null ? t("response {took} vs {limit}", { took: duration(b.tookMinutes, t), limit: duration(b.limitMinutes, t) }) : t("no response within {limit}", { limit: duration(b.limitMinutes, t) });
  }
  const fixed: Record<string, string> = { arrival: "arrival outside the scheduled window", first_time_fix: "not fixed the first time", overdue: "open past its due time" };
  return fixed[b.kind] ? t(fixed[b.kind]) : b.detail;
}
const KIND: Record<string, string> = { response: "Response", arrival: "Arrival", first_time_fix: "First-time fix", overdue: "Overdue" };
/** The recent breaches (newest first, at most 50) with the customer and Open job →. */
export const breachRows = (sc: ApiScorecard, names: Map<string, string>, t: T = en) =>
  sc.breaches.map((b, i) => ({ key: `${b.jobId}:${b.kind}:${i}`, jobId: b.jobId, short: b.jobId.slice(0, 8), text: `${names.get(b.customerId) ?? t("customer")} · ${breachText(b, t)}`, kind: KIND[b.kind] ? t(KIND[b.kind]) : b.kind, tone: (b.kind === "overdue" ? "crit" : "warn") as Tone }));

/** The targets per plan type for the edit dialog: the one in effect (or the default) and a scheduled one, if any. */
export function targetsByPlan(targets: ApiTargetView[], i: I18n = EN) {
  const { t, display } = i;
  return (Object.keys(planLabel) as PlanType[]).map((plan) => {
    const now = targets.find((x) => x.planType === plan && x.state !== "scheduled")!;
    const next = targets.filter((x) => x.planType === plan && x.state === "scheduled");
    const since = now.state === "default" ? t("default") : t("v{v} since {date}", { v: now.version, date: showDate(now.effectiveFrom, display) });
    return {
      plan, label: planName(plan, t), now,
      line: `${since}: ${t("≤ {h} h · arrival {a} % · first-time fix {f} %", { h: now.responseHours, a: now.arrivalInWindowPercent, f: now.firstTimeFixPercent })}`,
      scheduled: next.map((x) => t("v{v} from {time}: ≤ {h} h · {a} % · {f} %", { v: x.version, time: showTime(x.effectiveFrom, display), h: x.responseHours, a: x.arrivalInWindowPercent, f: x.firstTimeFixPercent })),
    };
  });
}

/** The edit form (IR111): response 1–168 h, percentages 0–100, effective now or later (applies to jobs created afterwards). */
export function targetErrors(f: { responseHours: string; arrival: string; ftf: string; effectiveFrom: string | null }, now: number, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  const h = Number(f.responseHours);
  if (!/^\d+$/.test(f.responseHours.trim()) || h < 1 || h > 168) e.responseHours = t("1–168 whole hours.");
  for (const [k, v] of [["arrival", f.arrival], ["ftf", f.ftf]] as const) {
    const n = Number(v);
    if (!/^\d+(\.\d)?$/.test(v.trim()) || n < 0 || n > 100) e[k] = t("0–100 %.");
  }
  if (!f.effectiveFrom || Date.parse(f.effectiveFrom) < now - 60_000) e.effectiveFrom = t("Now or later.");
  return e;
}

/** The customers table as CSV (Export CSV, client-side): quoted fields, the header first, in the user's language; the
 * period's ends are dates in the display time zone. */
export function slaCsv(rows: SlaRow[], period: { from: string; to: string }, i: I18n = EN): string {
  const { t, display } = i;
  const q = (v: string | number) => `"${String(v).replace(/"/g, '""')}"`;
  const day = (iso: string) => zonedParts(iso, display.timeZone).date;
  const head = ["Customer", "Plan", "Jobs", "Response", "Arrival", "First-time fix", "Rating", "Overdue", "Status", "Period from", "Period to"].map((h) => t(h));
  return [head, ...rows.map((r) => [r.name, r.plan, r.jobs, r.response, r.arrival, r.ftf, r.rating, r.overdue, r.status.label, day(period.from), day(period.to)])].map((l) => l.map(q).join(",")).join("\n") + "\n";
}

/** Readable refusals of sla.saveTargets. */
export function slaRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): string {
  const keys: Record<string, string> = { "errors.targets_exist": "targets for this plan already start at that time", "error.past": "must be now or later", "error.range": "out of range", "error.invalid": "not a valid value", "error.required": "required" };
  const word = (k: string) => (keys[k] ? t(keys[k]) : k);
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${word(v)}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return f.code === "CONFLICT" ? t("Not saved (CONFLICT): {reason}.", { reason: word(f.messageKey) }) : `${word(f.messageKey)}.`;
  return `${f.code} — ${f.messageKey}`;
}
