// The technician's alert evidence (FR-T07, DD-T07, SCR-T07, Figma Technician 02-14…02-17) from the Core API: the
// alert with the condition that raised it (Alert.rule, read with the alert — technicians never read policies, IR284),
// its evidence and the latest reading of the rule's metric, how it resolves (a sustained remeasurement for a policy
// alert, D08; a reason with alert.resolve otherwise, IR66), its history and what it relates to. Pure code shared by
// the server loader and the view; Vitest covers it. Texts in the display language; instants in the user's display time
// zone (IR44).
import { EN, relativeTime, showDate, type I18n, type T } from "@ac/web/lib/i18n";
import { alertTitle } from "@ac/web/lib/alerts";
import { metricLabel, metricUnit } from "@ac/web/lib/adminAlerts";
import { airNumber } from "@ac/web/lib/air";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { ApiUnitDetail } from "@ac/web/lib/units";

/** AlertRule of service-contracts.ts: the condition of the alert's policy or default rule. */
export type ApiAlertRule = { name: string; metric: string; operator: "gt" | "gte" | "lt" | "lte"; threshold: number; recoveryThreshold: number; durationSeconds: number };
/** Alert of service-contracts.ts (the fields this screen shows). */
export type ApiTechAlert = {
  id: string; version: number; unitId: string; type: string; severity: "critical" | "warning" | "normal"; status: "open" | "acknowledged" | "resolved"; causeCode: string;
  evidenceKind: string; evidenceText: string; observedAt: string; detectedAt: string; acknowledgedAt: string | null; resolvedAt: string | null; resolutionReason: string | null;
  previousAlertId: string | null; policyId: string | null; rule?: ApiAlertRule | null;
};
type Tone = "ok" | "warn" | "crit" | "primary" | "muted";

const OP: Record<string, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" };
/** A duration in its largest whole unit from two of them up: “60 s”, “10 min”, “3 h”. */
export const durationText = (s: number, t: T) => (s >= 7200 && s % 3600 === 0 ? t("{n} h", { n: s / 3600 }) : s >= 120 && s % 60 === 0 ? t("{n} min", { n: s / 60 }) : t("{n} s", { n: s }));
const value = (metric: string, v: number) => `${airNumber(metric, v)} ${metricUnit[metric] ?? ""}`.trim();
const label = (metric: string, t: T) => (metricLabel[metric] ? t(metricLabel[metric]) : metric.replace(/_/g, " "));
/** The words of a rule (Figma: “Policy: temperature ≥ 30 °C, recovery < 28 °C, duration 60 s”); recovery is below the
 * recovery threshold for a high limit and above it for a low one (D08). */
export function ruleWords(r: ApiAlertRule, t: T) {
  const up = r.operator === "gt" || r.operator === "gte";
  const recovery = `${up ? "<" : ">"} ${value(r.metric, r.recoveryThreshold)}`, duration = durationText(r.durationSeconds, t);
  return {
    policy: t("Policy: {metric} {op} {threshold}, recovery {recovery}, duration {duration}", { metric: label(r.metric, t), op: OP[r.operator] ?? r.operator, threshold: value(r.metric, r.threshold), recovery, duration }),
    recovery: t("{recovery} for {duration}", { recovery, duration }), raise: `${label(r.metric, t)} ${OP[r.operator] ?? r.operator} ${value(r.metric, r.threshold)}`,
  };
}

const KIND: Record<string, string> = { demo_observation: "Observed (demo)", inferred: "Suspected", inspection: "Inspection record" };
const ORIGIN: Record<string, string> = { measured: "Measured", estimated: "Estimated", inspection: "Inspection" };
const QUALITY: Record<string, string> = { valid: "good", stale: "stale", suspect: "suspect", missing: "missing" };
const STATE: Record<string, [string, Tone]> = { open: ["Open", "crit"], acknowledged: ["Acknowledged", "warn"], resolved: ["Resolved", "ok"] };

export type EvidenceCard = { kind: string; title: string; value: string; sub: string };
export type AlertView = {
  id: string; version: number; status: ApiTechAlert["status"]; severity: ApiTechAlert["severity"]; policyless: boolean;
  banner: { title: string; sub: string; tone: Tone; state: string | null };
  cards: EvidenceCard[];
  resolution: { info: string | null; done: string | null; note: string; forbidden: string };
  history: { id: string; time: string; title: string; sub: string; badge: { text: string; tone: Tone } }[];
  related: [string, string][];
};
/** The selected alert as the screen shows it. `latest` is the unit's latest reading of the rule's metric (units.get);
 * `job` the URL's job (jobs.get); `canResolve` whether the user holds alert.resolve. */
export function alertView(a: ApiTechAlert, ctx: { alerts: ApiTechAlert[]; unit: ApiUnitDetail | null; job: { id: string; type: string } | null; canResolve: boolean }, nowMs: number, i: I18n = EN): AlertView {
  const { t, display } = i;
  const when = (iso: string) => relativeTime(iso, nowMs, i);
  const rule = a.rule ?? null, words = rule ? ruleWords(rule, t) : null;
  const title = rule?.name ?? alertTitle(a, t);
  const [state, tone] = STATE[a.status] ?? STATE.open;
  const cards: EvidenceCard[] = [{ kind: t(KIND[a.evidenceKind] ?? "Evidence"), title: t("Evidence at detection"), value: a.evidenceText, sub: t("observed {time}", { time: when(a.observedAt) }) }];
  const m = rule ? ctx.unit?.latestMeasurements.find((x) => x.metric === rule.metric) : undefined;
  if (m) {
    cards.push({
      kind: t(ORIGIN[m.origin] ?? m.origin), title: t("Latest reading"), value: m.value === null ? "—" : value(m.metric, m.value),
      sub: [t("observed {time}", { time: when(m.observedAt) }), t(QUALITY[m.quality] ?? m.quality), m.origin === "estimated" ? t("confidence unknown — no numeric probability shown") : ""].filter(Boolean).join(" · "),
    });
  }
  const history: AlertView["history"] = [];
  if (a.resolvedAt) history.push({ id: "resolved", time: when(a.resolvedAt), title: t("Resolved"), sub: a.resolutionReason ?? "", badge: { text: t("Resolved"), tone: "ok" } });
  if (a.acknowledgedAt) history.push({ id: "acknowledged", time: when(a.acknowledgedAt), title: t("Acknowledged"), sub: "", badge: { text: t("Acknowledged"), tone: "primary" } });
  history.push({ id: "raised", time: when(a.detectedAt), title: t("Raised"), sub: [a.evidenceText, words && t("{condition} for {duration}", { condition: words.raise, duration: durationText(rule!.durationSeconds, t) })].filter(Boolean).join(" · "), badge: { text: t(KIND[a.evidenceKind] ?? "Evidence"), tone: "primary" } });
  const prev = a.previousAlertId ? ctx.alerts.find((x) => x.id === a.previousAlertId) : undefined;
  const unit = ctx.unit ? `${ctx.unit.displayName}${ctx.unit.location.pathLabels.length ? ` · ${ctx.unit.location.pathLabels.join(" › ")}` : ""}` : a.unitId.slice(0, 8);
  const resolver = a.status === "resolved" ? t("done {time}", { time: when(a.resolvedAt ?? a.detectedAt) }) : ctx.canResolve ? t(rule ? "with a reason, or by a remeasurement" : "with a reason") : rule ? t("with a remeasurement only") : t("no — ask a user with alert.resolve");
  return {
    id: a.id, version: a.version, status: a.status, severity: a.severity, policyless: !a.policyId,
    banner: {
      title: `${a.id.slice(0, 8)} · ${title}`, tone, state: a.status === "open" ? null : `${t(state)} · ${when((a.status === "resolved" ? a.resolvedAt : a.acknowledgedAt) ?? a.detectedAt)}`,
      sub: words ? words.policy : t("{kind} · no policy — it resolves only with a reason (IR66)", { kind: t(KIND[a.evidenceKind] ?? "Evidence") }),
    },
    cards,
    resolution: {
      info: a.status === "resolved" ? null : words ? t("Automatic resolution needs a remeasurement {recovery} — or a reason below with alert.resolve.", { recovery: words.recovery })
        : t("No remeasurement resolves this alert — it has no policy. Resolve it with a reason below with alert.resolve (IR66)."),
      done: a.status !== "resolved" ? null : [t("Resolved {time} — {reason}.", { time: when(a.resolvedAt ?? a.detectedAt), reason: a.resolutionReason ?? t("no reason recorded") }), a.acknowledgedAt ? t("Acknowledged {time}.", { time: when(a.acknowledgedAt) }) : ""].filter(Boolean).join(" "),
      note: a.status === "resolved" ? t("Completing the job alone never resolves an alert. If the condition returns, a new alert is created with previousAlertId = {id}.", { id: a.id.slice(0, 8) })
        : t("Recurrence after resolution creates a new alert with previousAlertId linking back to this one."),
      forbidden: words ? t("Not resolved (FORBIDDEN) — you don’t have alert.resolve. The alert resolves automatically once a remeasurement stays {recovery}, or ask a permitted user.", { recovery: words.recovery })
        : t("Not resolved (FORBIDDEN) — you don’t have alert.resolve. Ask a permitted user to resolve it."),
    },
    history,
    related: [
      [t("Job"), ctx.job ? `${ctx.job.id.slice(0, 8)} · ${typeLabel(ctx.job.type, t)}` : "—"], [t("Unit"), unit], [t("Policy"), rule ? rule.name : t("No policy")],
      [t("Recovery rule"), words ? words.recovery : "—"], [t("Previous alert"), prev ? `${showDate(prev.detectedAt, display)} · ${statusWord(prev.status, t)}` : a.previousAlertId ? a.previousAlertId.slice(0, 8) : t("none")],
      [t("You can resolve"), resolver],
    ],
  };
}

const RANK: Record<string, number> = { critical: 0, warning: 1, normal: 2 };
/** The alert the page opens: the URL's, else the worst unresolved one, else the newest. */
export function selectAlert(alerts: ApiTechAlert[], alertId?: string): ApiTechAlert | null {
  const open = alerts.filter((a) => a.status !== "resolved").sort((a, b) => RANK[a.severity] - RANK[b.severity] || b.detectedAt.localeCompare(a.detectedAt));
  return alerts.find((a) => a.id === alertId) ?? open[0] ?? [...alerts].sort((a, b) => b.detectedAt.localeCompare(a.detectedAt))[0] ?? null;
}
/** The unit's alerts to choose from: title, state and when each was detected, unresolved first. */
export function alertChoices(alerts: ApiTechAlert[], nowMs: number, i: I18n = EN) {
  const { t } = i;
  return [...alerts].sort((a, b) => Number(a.status === "resolved") - Number(b.status === "resolved") || b.detectedAt.localeCompare(a.detectedAt)).map((a) => ({
    id: a.id, title: a.rule?.name ?? alertTitle(a, t), sub: `${t((STATE[a.status] ?? STATE.open)[0])} · ${relativeTime(a.detectedAt, nowMs, i)}`, severity: a.severity,
  }));
}

const REFUSED: Record<string, string> = {
  "errors.assignment_required": "You need an active assignment on this unit to act on its alerts.",
  "errors.assignment_not_started": "Your work window on this unit has not started yet.",
  "errors.assignment_ended": "Your work window on this unit has ended.",
  "error.versionConflict": "The alert changed elsewhere — the latest state is shown.",
  "error.invalidState": "The alert is no longer in that state — the latest state is shown.",
  "error.notFound": "This alert is not in your assignments.",
};
/** A refused acknowledge or resolve in words; a missing alert.resolve says how the alert can still resolve. */
export function refusal(r: { code: string; messageKey: string }, forbidden: string, t: T): string {
  if (r.messageKey === "error.forbidden") return forbidden;
  if (REFUSED[r.messageKey]) return `${r.code} — ${t(REFUSED[r.messageKey])}`;
  if (r.code === "VALIDATION") return t("VALIDATION — a resolution reason is required (1–1000 characters).");
  if (r.code === "UNAVAILABLE" || r.code === "TIMEOUT") return t("UNAVAILABLE — not sent; your reason is kept. Try again.");
  return `${r.code} — ${r.messageKey}`;
}
