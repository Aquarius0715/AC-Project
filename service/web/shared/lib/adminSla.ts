// HQ SLA by customer (FR-A22, DD-A22, IR131 items 4–5, IR236, Figma Admin 06-10 699:21404) from the Core API: the
// period and contractor filters, the KPI tiles with the targets of the plan types in the scorecard, one row per
// customer, the recent breaches, the targets per plan type for the edit dialog and the CSV export. Pure code shared by
// the server loader and the client view; Vitest covers it.
import { klTime } from "@ac/web/lib/devices";
import { planLabel } from "@ac/web/lib/contracts";

type PlanType = "rto" | "general" | "energy" | "environment";
export type ApiSlaMetrics = { responseWithinTarget: number | null; arrivalInWindow: number | null; firstTimeFix: number | null; averageRating: number | null; ratingCount: number; openOverdue: number };
export type ApiTargetView = { planType: PlanType; responseHours: number; arrivalInWindowPercent: number; firstTimeFixPercent: number; version: number; effectiveFrom: string | null; state: "in_effect" | "default" | "scheduled" };
export type ApiScorecard = {
  period: { from: string; to: string }; contractorOrgId: string | null; totals: ApiSlaMetrics;
  customers: (ApiSlaMetrics & { customerId: string; planType: PlanType; jobCount: number; status: "on_track" | "at_risk" | "breached" })[];
  breaches: { jobId: string; customerId: string; kind: "response" | "arrival" | "first_time_fix" | "overdue"; detail: string }[];
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
const plans = (targets: ApiTargetView[], used: PlanType[]) => targets.filter((t) => t.state !== "scheduled" && used.includes(t.planType));
/** “target 95 %” for one plan type, “targets 85–95 % by plan” when the customers' plans differ. */
function targetText(values: number[]): string {
  const u = [...new Set(values)].sort((a, b) => a - b);
  return !u.length ? "no target" : u.length === 1 ? `target ${u[0]} %` : `targets ${u[0]}–${u[u.length - 1]} % by plan`;
}

/** The KPI tiles (Figma 06-10) with the targets of the plan types of the customers in the scorecard; no data is “—”. */
export function slaTiles(sc: ApiScorecard): { label: string; value: string; sub: string; tone?: "warn" | "crit" }[] {
  const used = [...new Set(sc.customers.map((c) => c.planType))];
  const ts = plans(sc.targets, used.length ? used : ["general"]);
  const hours = [...new Set(ts.map((t) => t.responseHours))].sort((a, b) => a - b);
  const t = sc.totals;
  const below = (v: number | null, target: number) => (v !== null && v < target ? (target - v > 10 ? "crit" : "warn") : undefined);
  const arrival = Math.min(...ts.map((x) => x.arrivalInWindowPercent));
  const ftf = Math.min(...ts.map((x) => x.firstTimeFixPercent));
  return [
    { label: hours.length === 1 ? `Response ≤ ${hours[0]} h` : "Response within target", value: pct(t.responseWithinTarget), sub: hours.length === 1 ? "target 100 % of jobs" : `≤ ${hours.join(" / ")} h by plan · target 100 %`, tone: below(t.responseWithinTarget, 100) },
    { label: "Arrival in window", value: pct(t.arrivalInWindow), sub: targetText(ts.map((x) => x.arrivalInWindowPercent)), tone: below(t.arrivalInWindow, arrival) },
    { label: "First-time fix", value: pct(t.firstTimeFix), sub: targetText(ts.map((x) => x.firstTimeFixPercent)), tone: below(t.firstTimeFix, ftf) },
    { label: "Avg. customer rating", value: t.averageRating === null ? "—" : `${t.averageRating.toFixed(1)} ★`, sub: `${t.ratingCount} rating${t.ratingCount === 1 ? "" : "s"}` },
    { label: "Open & overdue", value: String(t.openOverdue), sub: `${sc.breaches.length} breach${sc.breaches.length === 1 ? "" : "es"} in the period`, tone: t.openOverdue > 0 ? "crit" : undefined },
  ];
}

export type SlaRow = { id: string; name: string; sub: string; plan: string; jobs: number; response: string; arrival: string; ftf: string; rating: string; overdue: number; status: { label: string; tone: Tone } };
const STATUS: Record<string, { label: string; tone: Tone }> = { on_track: { label: "On track", tone: "ok" }, at_risk: { label: "At risk", tone: "warn" }, breached: { label: "Breached", tone: "crit" } };
/** One customer of the scorecard (Figma “customer-a · Home A, Office A · 5 units”). */
export function slaRows(sc: ApiScorecard, customers: { id: string; name: string; organizationId: string }[], properties: { name: string; customerOrgId: string }[], units: { customerOrgId: string }[]): SlaRow[] {
  return sc.customers.map((c) => {
    const cust = customers.find((x) => x.id === c.customerId);
    const org = cust?.organizationId ?? "";
    const props = properties.filter((p) => p.customerOrgId === org).map((p) => p.name);
    const n = units.filter((u) => u.customerOrgId === org).length;
    return {
      id: c.customerId, name: cust?.name ?? "customer", sub: `${props.length > 2 ? `${props.length} properties` : props.join(", ") || "no properties"} · ${n} unit${n === 1 ? "" : "s"}`,
      plan: planLabel[c.planType] ?? c.planType, jobs: c.jobCount, response: pct(c.responseWithinTarget), arrival: pct(c.arrivalInWindow), ftf: pct(c.firstTimeFix),
      rating: c.averageRating === null ? "—" : `${c.averageRating.toFixed(1)} ★`, overdue: c.openOverdue, status: STATUS[c.status] ?? { label: c.status, tone: "muted" },
    };
  });
}

const KIND: Record<string, string> = { response: "Response", arrival: "Arrival", first_time_fix: "First-time fix", overdue: "Overdue" };
/** The recent breaches (newest first, at most 50) with the customer and Open job →. */
export const breachRows = (sc: ApiScorecard, names: Map<string, string>) =>
  sc.breaches.map((b, i) => ({ key: `${b.jobId}:${b.kind}:${i}`, jobId: b.jobId, short: b.jobId.slice(0, 8), text: `${names.get(b.customerId) ?? "customer"} · ${b.detail}`, kind: KIND[b.kind] ?? b.kind, tone: (b.kind === "overdue" ? "crit" : "warn") as Tone }));

/** The targets per plan type for the edit dialog: the one in effect (or the default) and a scheduled one, if any. */
export function targetsByPlan(targets: ApiTargetView[]) {
  return (Object.keys(planLabel) as PlanType[]).map((plan) => {
    const now = targets.find((t) => t.planType === plan && t.state !== "scheduled")!;
    const next = targets.filter((t) => t.planType === plan && t.state === "scheduled");
    return {
      plan, label: planLabel[plan], now,
      line: `${now.state === "default" ? "default" : `v${now.version} since ${klTime(now.effectiveFrom!).slice(0, 10)}`}: ≤ ${now.responseHours} h · arrival ${now.arrivalInWindowPercent} % · first-time fix ${now.firstTimeFixPercent} %`,
      scheduled: next.map((t) => `v${t.version} from ${klTime(t.effectiveFrom!).slice(0, 16)}: ≤ ${t.responseHours} h · ${t.arrivalInWindowPercent} % · ${t.firstTimeFixPercent} %`),
    };
  });
}

/** The edit form (IR111): response 1–168 h, percentages 0–100, effective now or later (applies to jobs created afterwards). */
export function targetErrors(f: { responseHours: string; arrival: string; ftf: string; effectiveFrom: string | null }, now: number): Record<string, string> {
  const e: Record<string, string> = {};
  const h = Number(f.responseHours);
  if (!/^\d+$/.test(f.responseHours.trim()) || h < 1 || h > 168) e.responseHours = "1–168 whole hours.";
  for (const [k, v] of [["arrival", f.arrival], ["ftf", f.ftf]] as const) {
    const n = Number(v);
    if (!/^\d+(\.\d)?$/.test(v.trim()) || n < 0 || n > 100) e[k] = "0–100 %.";
  }
  if (!f.effectiveFrom || Date.parse(f.effectiveFrom) < now - 60_000) e.effectiveFrom = "Now or later.";
  return e;
}

/** The customers table as CSV (Export CSV, client-side): quoted fields, header first. */
export function slaCsv(rows: SlaRow[], period: { from: string; to: string }): string {
  const q = (v: string | number) => `"${String(v).replace(/"/g, '""')}"`;
  const head = ["Customer", "Plan", "Jobs", "Response", "Arrival", "First-time fix", "Rating", "Overdue", "Status", "Period from", "Period to"];
  return [head, ...rows.map((r) => [r.name, r.plan, r.jobs, r.response, r.arrival, r.ftf, r.rating, r.overdue, r.status.label, klTime(period.from).slice(0, 10), klTime(period.to).slice(0, 10)])].map((l) => l.map(q).join(",")).join("\n") + "\n";
}

/** Readable refusals of sla.saveTargets. */
export function slaRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }): string {
  const keys: Record<string, string> = { "errors.targets_exist": "targets for this plan already start at that time", "error.past": "must be now or later", "error.range": "out of range", "error.invalid": "not a valid value", "error.required": "required" };
  const fields = Object.entries(f.fieldErrors ?? {}).map(([k, v]) => `${k}: ${keys[v] ?? v}`);
  if (fields.length) return fields.join(" · ");
  if (keys[f.messageKey]) return `${f.code === "CONFLICT" ? "Not saved (CONFLICT): " : ""}${keys[f.messageKey]}.`;
  return `${f.code} — ${f.messageKey}`;
}
