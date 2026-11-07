// The technician's alert evidence (FR-T07, DD-T07, SCR-T07, Figma Technician 02-14…02-17) from the Core API: the
// alert with the condition that raised it (Alert.rule, read with the alert — technicians never read policies, IR284),
// its evidence and the latest reading of the rule's metric, how it resolves (a sustained remeasurement for a policy
// alert, D08; a reason with alert.resolve otherwise, IR66), its history and what it relates to. Pure code shared by
// the server loader and the view; Vitest covers it. Texts in the display language; instants in the user's display time
// zone (IR44).
import { EN, relativeTime, showDate, type I18n, type T, textOf } from "@ac/web/lib/i18n";
import { evidenceText, resolutionText } from "@ac/web/lib/evidence";
import { actionMessage } from "@ac/web/lib/actionMessage";
import { alertTitle } from "@ac/web/lib/alerts";
import { metricLabel, metricUnit } from "@ac/web/lib/adminAlerts";
import { airNumber } from "@ac/web/lib/air";
import { statusWord, typeLabel } from "@ac/web/lib/partnerJobDetail";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import { lookup } from "@ac/web/lib/lookup";

const ACKNOWLEDGED = "Acknowledged";

/** AlertRule of service-contracts.ts: the condition of the alert's policy or default rule. */
export interface ApiAlertRule { name: string; metric: string; operator: "gt" | "gte" | "lt" | "lte"; threshold: number; recoveryThreshold: number; durationSeconds: number }
/** Alert of service-contracts.ts (the fields this screen shows). */
export interface ApiTechAlert {
  id: string; version: number; unitId: string; type: string; severity: "critical" | "warning" | "normal"; status: "open" | "acknowledged" | "resolved"; causeCode: string;
  evidenceKind: string; evidenceText: string; observedAt: string; detectedAt: string; acknowledgedAt: string | null; resolvedAt: string | null; resolutionReason: string | null;
  previousAlertId: string | null; policyId: string | null; rule?: ApiAlertRule | null;
}
type Tone = "ok" | "warn" | "crit" | "primary" | "muted";

const OP: Record<string, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" };
/** A duration in its largest whole unit from two of them up: “60 s”, “10 min”, “3 h”. */
export function durationText(s: number, t: T): string {
  if (s >= 7200 && s % 3600 === 0) return t("{n} h", { n: s / 3600 });
  if (s >= 120 && s % 60 === 0) return t("{n} min", { n: s / 60 });
  return t("{n} s", { n: s });
}
const value = (metric: string, v: number) => `${airNumber(metric, v)} ${lookup(metricUnit, metric) ?? ""}`.trim();
const label = (metric: string, t: T) => (textOf(metricLabel, metric, t) ?? metric.replaceAll('_', " "));
/** The words of a rule (Figma: “Policy: temperature ≥ 30 °C, recovery < 28 °C, duration 60 s”); recovery is below the
recovery threshold for a high limit and above it for a low one (D08). */
export function ruleWords(r: ApiAlertRule, t: T) {
  const isUp = r.operator === "gt" || r.operator === "gte";
  const recovery = `${isUp ? "<" : ">"} ${value(r.metric, r.recoveryThreshold)}`, duration = durationText(r.durationSeconds, t);
  return {
    policy: t("Policy: {metric} {op} {threshold}, recovery {recovery}, duration {duration}", { metric: label(r.metric, t), op: lookup(OP, r.operator) ?? r.operator, threshold: value(r.metric, r.threshold), recovery, duration }),
    recovery: t("{recovery} for {duration}", { recovery, duration }), raise: `${label(r.metric, t)} ${lookup(OP, r.operator) ?? r.operator} ${value(r.metric, r.threshold)}`,
  };
}

const KIND: Record<string, string> = { demo_observation: "Observed (demo)", inferred: "Suspected", inspection: "Inspection record" };
const ORIGIN: Record<string, string> = { measured: "Measured", estimated: "Estimated", inspection: "Inspection" };
const QUALITY: Record<string, string> = { valid: "good", stale: "stale", suspect: "suspect", missing: "missing" };
const OPEN: [string, Tone] = ["Open", "crit"]; // an alert of an unknown status reads as open
const STATE: Record<string, [string, Tone]> = { open: ["Open", "crit"], acknowledged: [ACKNOWLEDGED, "warn"], resolved: ["Resolved", "ok"] };

export interface EvidenceCard { kind: string; title: string; value: string; sub: string }
export interface AlertView {
  id: string; version: number; status: ApiTechAlert["status"]; severity: ApiTechAlert["severity"]; policyless: boolean;
  banner: { title: string; sub: string; tone: Tone; state: string | null };
  cards: EvidenceCard[];
  resolution: { info: string | null; done: string | null; note: string; forbidden: string };
  history: { id: string; time: string; title: string; sub: string; badge: { text: string; tone: Tone } }[];
  related: [string, string][];
}
/** A unit with its place (“Bedroom AC · Home › Floor 1”). */
function unitPath(u: ApiUnitDetail): string {
  return u.location.pathLabels.length > 0 ? `${u.displayName} · ${u.location.pathLabels.join(" › ")}` : u.displayName;
}
/** How the technician can resolve an open alert: by reason, by a remeasurement when it has a policy, or not at all. */
function resolveWay(canResolve: boolean, hasRule: boolean): string {
  if (canResolve) return hasRule ? "with a reason, or by a remeasurement" : "with a reason";
  return hasRule ? "with a remeasurement only" : "no — ask a user with alert.resolve";
}
/** When the alert reached its state: resolved or acknowledged (else when it was raised). */
function stateTime(a: ApiTechAlert): string {
  return (a.status === "resolved" ? a.resolvedAt : a.acknowledgedAt) ?? a.detectedAt;
}
/** What resolves an open alert: a remeasurement within its recovery, or only a reason when it has no policy. */
function resolutionInfo(words: { recovery: string } | null, t: T): string {
  return words ? t("Automatic resolution needs a remeasurement {recovery} — or a reason below with alert.resolve.", { recovery: words.recovery })
    : t("No remeasurement resolves this alert — it has no policy. Resolve it with a reason below with alert.resolve.");
}
/** The selected alert as the screen shows it. `latest` is the unit's latest reading of the rule's metric (units.get);
`job` the URL's job (jobs.get); `canResolve` whether the user holds alert.resolve. */
export function alertView(a: ApiTechAlert, ctx: { alerts: ApiTechAlert[]; unit: ApiUnitDetail | null; job: { id: string; type: string } | null; canResolve: boolean }, nowMs: number, i: I18n = EN): AlertView {
  const { t, display } = i;
  const when = (iso: string) => relativeTime(iso, nowMs, i);
  const rule = a.rule ?? null, words = rule ? ruleWords(rule, t) : null;
  const title = rule?.name ?? alertTitle(a, t);
  const [state, tone] = lookup(STATE, a.status) ?? OPEN;
  const cards: EvidenceCard[] = [{ kind: t(lookup(KIND, a.evidenceKind) ?? "Evidence"), title: t("Evidence at detection"), value: evidenceText(a.evidenceText, t), sub: t("observed {time}", { time: when(a.observedAt) }) }];
  const m = rule ? ctx.unit?.latestMeasurements.find((x) => x.metric === rule.metric) : undefined;
  if (m) {
    cards.push({
      kind: t(lookup(ORIGIN, m.origin) ?? m.origin), title: t("Latest reading"), value: m.value === null ? "—" : value(m.metric, m.value),
      sub: [t("observed {time}", { time: when(m.observedAt) }), t(lookup(QUALITY, m.quality) ?? m.quality), m.origin === "estimated" ? t("confidence unknown — no numeric probability shown") : ""].filter(Boolean).join(" · "),
    });
  }
  const history: AlertView["history"] = [];
  if (a.resolvedAt) history.push({ id: "resolved", time: when(a.resolvedAt), title: t("Resolved"), sub: a.resolutionReason ? resolutionText(a.resolutionReason, t) : "", badge: { text: t("Resolved"), tone: "ok" } });
  if (a.acknowledgedAt) history.push({ id: "acknowledged", time: when(a.acknowledgedAt), title: t(ACKNOWLEDGED), sub: "", badge: { text: t(ACKNOWLEDGED), tone: "primary" } });
  history.push({ id: "raised", time: when(a.detectedAt), title: t("Raised"), sub: [evidenceText(a.evidenceText, t), rule && words && t("{condition} for {duration}", { condition: words.raise, duration: durationText(rule.durationSeconds, t) })].filter(Boolean).join(" · "), badge: { text: t(lookup(KIND, a.evidenceKind) ?? "Evidence"), tone: "primary" } });
  const prev = a.previousAlertId ? ctx.alerts.find((x) => x.id === a.previousAlertId) : undefined;
  const unit = ctx.unit ? unitPath(ctx.unit) : a.unitId.slice(0, 8);
  const resolver = a.status === "resolved" ? t("done {time}", { time: when(a.resolvedAt ?? a.detectedAt) }) : t(resolveWay(ctx.canResolve, !!rule));
  const previous = prev ? `${showDate(prev.detectedAt, display)} · ${statusWord(prev.status, t)}` : (a.previousAlertId?.slice(0, 8) ?? t("none"));
  return {
    id: a.id, version: a.version, status: a.status, severity: a.severity, policyless: !a.policyId,
    banner: {
      title: `${a.id.slice(0, 8)} · ${title}`,
      state: a.status === "open" ? null : `${t(state)} · ${when(stateTime(a))}`,
      sub: words ? words.policy : t("{kind} · no policy — it resolves only with a reason", { kind: t(lookup(KIND, a.evidenceKind) ?? "Evidence") }),
      tone,
    },
    resolution: {
      info: a.status === "resolved" ? null : resolutionInfo(words, t),
      done: a.status === "resolved" ? [t("Resolved {time} — {reason}.", { time: when(a.resolvedAt ?? a.detectedAt), reason: a.resolutionReason ? resolutionText(a.resolutionReason, t) : t("no reason recorded") }), a.acknowledgedAt ? t("Acknowledged {time}.", { time: when(a.acknowledgedAt) }) : ""].filter(Boolean).join(" ") : null,
      note: a.status === "resolved" ? t("Completing the job alone never resolves an alert. If the condition returns, a new alert is created that links back to alert {id}.", { id: a.id.slice(0, 8) })
        : t("If it recurs after it is resolved, a new alert is created that links back to this one."),
      forbidden: words ? t("Not resolved — you don’t have alert.resolve. The alert resolves automatically once a remeasurement stays {recovery}, or ask a permitted user.", { recovery: words.recovery })
        : t("Not resolved — you don’t have alert.resolve. Ask a permitted user to resolve it."),
    },
    related: [
      [t("Job"), ctx.job ? `${ctx.job.id.slice(0, 8)} · ${typeLabel(ctx.job.type, t)}` : "—"], [t("Unit"), unit], [t("Policy"), rule ? rule.name : t("No policy")],
      [t("Recovery rule"), words ? words.recovery : "—"], [t("Previous alert"), previous],
      [t("You can resolve"), resolver],
    ], cards, history,
  };
}

const RANK: Record<string, number> = { critical: 0, warning: 1, normal: 2 };
/** The alert the page opens: the URL's, else the worst unresolved one, else the newest. */
export function selectAlert(alerts: ApiTechAlert[], alertId?: string): ApiTechAlert | null {
  const open = alerts.filter((a) => a.status !== "resolved").toSorted((a, b) => (lookup(RANK, a.severity) ?? 9) - (lookup(RANK, b.severity) ?? 9) || b.detectedAt.localeCompare(a.detectedAt));
  return alerts.find((a) => a.id === alertId) ?? open[0] ?? [...alerts].toSorted((a, b) => b.detectedAt.localeCompare(a.detectedAt))[0] ?? null;
}
/** The unit's alerts to choose from: title, state and when each was detected, unresolved first. */
export function alertChoices(alerts: ApiTechAlert[], nowMs: number, i: I18n = EN) {
  const { t } = i;
  return [...alerts].toSorted((a, b) => Number(a.status === "resolved") - Number(b.status === "resolved") || b.detectedAt.localeCompare(a.detectedAt)).map((a) => {
    return {
      id: a.id, title: a.rule?.name ?? alertTitle(a, t), sub: `${t((lookup(STATE, a.status) ?? OPEN)[0])} · ${relativeTime(a.detectedAt, nowMs, i)}`, severity: a.severity,
    };
  });
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
export function refusal(r: { code: string; messageKey: string; retryAfterSeconds?: number | null }, forbidden: string, t: T): string {
  if (r.messageKey === "error.forbidden") return forbidden;
  const known = REFUSED[r.messageKey];
  if (known) return t(known); // in words, without the error code (IR590)
  if (r.code === "VALIDATION") return t("A resolution reason is required (1–1000 characters).");
  if (r.code === "UNAVAILABLE" || r.code === "TIMEOUT") return t("Not sent; your reason is kept. Try again.");
  return actionMessage({ ...r, fieldErrors: {} }, t); // the write limit's wait and other codes (IR380)
}
