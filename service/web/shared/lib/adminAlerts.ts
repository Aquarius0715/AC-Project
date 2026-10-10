// HQ alerts and alert policies (FR-A05, DATA_SOURCE=api): alerts.list, policies.list, units.list, customers.list and
// members.list projected for the admin alerts screen. Pure code shared by the Server Component and the client view;
// Vitest covers it. Texts in the display language (`i` / `t`, IR292); times IR44 in the user's display time zone, with
// "today / yesterday" for the days next to now. Cause codes and evidence texts stay as the Core API records them.
import { EN, relativeTime, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import { alertTitle } from "@ac/web/lib/alerts";
import type { OpInput } from "@ac/web/lib/opTypes";
import type { Metric as ContractMetric } from "@ac/web/lib/contracts.gen";

const en = translator("en");

export type Severity = "critical" | "warning" | "normal";
export type Operator = "gt" | "gte" | "lt" | "lte";
export type Channel = "inApp" | "email" | "whatsapp";
export type AlertState = "open" | "acknowledged" | "resolved";

/** Alert of service-contracts.ts. */
export type ApiAdminAlert = {
  id: string; version: number; unitId: string; policyId: string | null; type: string; severity: Severity; status: AlertState;
  causeCode: string; evidenceKind: string; evidenceText: string; observedAt: string; evidenceIds: string[]; detectedAt: string;
  acknowledgedAt: string | null; resolvedAt: string | null; resolutionReason: string | null;
};
/** Policy of service-contracts.ts (alert and default_alert kinds; automation policies are not on this screen). */
export type ApiPolicy = {
  id: string; version: number; kind: "alert" | "default_alert" | "automation"; name: string; unitIds: string[]; timezone: string; enabled: boolean; priority: number;
  customerId?: string | null; recipientMembershipIds?: string[]; channels?: Channel[]; escalateAfterMinutes?: number; cooldownMinutes?: number;
  metric?: string; operator?: Operator; threshold?: number; recoveryThreshold?: number; durationSeconds?: number; severity?: Severity;
  activeWindow?: { weekdays: number[]; startLocal: string; endLocal: string } | null; rules?: { ruleKey: string; name: string }[];
};
export type ApiUnitName = { id: string; displayName: string; customerOrgId: string };
export type ApiCustomerName = { id: string; name: string; organizationId: string };
export type ApiMemberName = { id: string; displayName: string; role: string };

/** One alert of the screen: `state` is the alert's status for the logic, `st` its label in the display language. */
export type AdminAlert = {
  id: string; version: number; title: string; time: string; sev: Severity; meta: string; state: AlertState; st: string;
  unit: string; context: string; evidence: string; cause: string; observed: string; detected: string; evidenceRecords: string;
  timeline: { time: string; title: string; detail: string; tone?: "warn" | "ok" }[];
};

export type AdminPolicy = {
  id: string; version: number; kind: "alert" | "default_alert"; name: string; group: string; sub: string; on: boolean; units: string;
  customerId: string | null; customerName: string; priority: number; timezone: string; metric: string; operator: Operator;
  threshold: number; recovery: number; duration: number; severity: Severity; cooldown: number; escalate: number; channels: Channel[];
  recipients: { id: string; label: string; role: string }[]; recipientIds: string[]; activeWindow: ApiPolicy["activeWindow"];
};

export const metricLabel: Record<string, string> = {
  temperature: "Temperature", humidity: "Humidity", co2: "CO₂ (ppm)", pm25: "PM2.5", power: "Power", vibration: "Vibration",
  refrigerant_pressure: "Refrigerant pressure", compressor_cycles: "Compressor cycles", airflow_drop: "Airflow drop", heartbeat_gap: "Heartbeat gap",
};
export const metricUnit: Record<string, string> = { temperature: "°C", humidity: "%", co2: "ppm", pm25: "µg/m³", power: "kW", vibration: "mm/s", refrigerant_pressure: "kPa", compressor_cycles: "cycles/h", airflow_drop: "%", heartbeat_gap: "min" };
export const opSymbol: Record<Operator, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" };
/** The metrics an HQ alert policy may watch (IR66 item 3, the Core API's hqMetrics); default rules have their own. */
export const HQ_METRICS = ["temperature", "humidity", "co2", "pm25", "refrigerant_pressure", "vibration", "power"] as const;
const sevRank: Record<Severity, number> = { critical: 0, warning: 1, normal: 2 };
const stRank: Record<AlertState, number> = { open: 0, acknowledged: 1, resolved: 2 };
const stLabel: Record<AlertState, string> = { open: "Open", acknowledged: "Acknowledged", resolved: "Resolved" };
const evidenceKind: Record<string, string> = { inferred: "inferred", inspection: "inspection record", demo_observation: "demo observation" };
const roleWord: Record<string, string> = { client: "client", admin: "HQ", contractor: "contractor", technician: "technician" };

/** Recovery must lie on the safe side of the threshold for the operator (≥ / >: below, ≤ / <: above). */
export function recoveryError(op: Operator, threshold: number, recovery: number, t: T = en): string | undefined {
  const above = op === "gt" || op === "gte";
  if (above ? recovery < threshold : recovery > threshold) return undefined;
  return t(above ? "Recovery {recovery} must be below the {threshold} threshold (direction for {op})" : "Recovery {recovery} must be above the {threshold} threshold (direction for {op})", { recovery, threshold, op: opSymbol[op] });
}

/** The alerts, unresolved first, then by severity and newest; each with its unit, customer and policy, its evidence and
 * its timeline (detected, acknowledged, resolved or what it is waiting for). */
export function adminAlertRows(alerts: ApiAdminAlert[], units: ApiUnitName[], customers: ApiCustomerName[], policies: ApiPolicy[], now: number, i: I18n = EN): AdminAlert[] {
  const { t, display } = i;
  const when = (iso: string) => relativeTime(iso, now, i);
  const unit = new Map(units.map((u) => [u.id, u]));
  const custByOrg = new Map(customers.map((c) => [c.organizationId, c.name]));
  const policy = new Map(policies.map((p) => [p.id, p.name]));
  return [...alerts]
    .sort((a, b) => stRank[a.status] - stRank[b.status] || sevRank[a.severity] - sevRank[b.severity] || b.detectedAt.localeCompare(a.detectedAt))
    .map((a) => {
      const u = unit.get(a.unitId);
      const customer = (u && custByOrg.get(u.customerOrgId)) ?? t("unknown customer");
      const pol = a.policyId ? policy.get(a.policyId) ?? t("policy") : t("no policy");
      const timeline: AdminAlert["timeline"] = [{ time: when(a.detectedAt), title: t("Alert detected"), detail: a.evidenceText, tone: "warn" }];
      if (a.acknowledgedAt) timeline.push({ time: when(a.acknowledgedAt), title: t("Acknowledged"), detail: t("Acknowledging does not resolve the alert") });
      timeline.push(a.resolvedAt
        ? { time: when(a.resolvedAt), title: t("Resolved"), detail: a.resolutionReason ?? "", tone: "ok" }
        : { time: "—", title: t(a.acknowledgedAt ? "Acknowledged — waiting for resolution" : "Waiting for acknowledgement"), detail: "" });
      return {
        id: a.id, version: a.version, title: alertTitle(a, t), time: when(a.detectedAt), sev: a.severity,
        meta: `${customer} · ${u?.displayName ?? a.unitId} · ${pol}`, state: a.status, st: t(stLabel[a.status]),
        unit: `${u?.displayName ?? t("Unit")} · ${a.unitId}`, context: customer,
        evidence: `${evidenceKind[a.evidenceKind] ? t(evidenceKind[a.evidenceKind]) : a.evidenceKind} — “${a.evidenceText}”`,
        cause: a.causeCode === "unknown" ? t("not determined") : t("{cause} (suspected)", { cause: a.causeCode }),
        observed: showTime(a.observedAt, display), detected: showTime(a.detectedAt, display),
        evidenceRecords: a.evidenceIds.length ? t(a.evidenceIds.length === 1 ? "1 attached" : "{n} attached", { n: a.evidenceIds.length }) : t("none attached"), timeline,
      };
    });
}

/** The alert policies, the default policy first, then each customer's by name; the condition in words. */
export function policyRows(policies: ApiPolicy[], customers: ApiCustomerName[], members: ApiMemberName[], t: T = en): AdminPolicy[] {
  const cust = new Map(customers.map((c) => [c.id, c.name]));
  const member = new Map(members.map((m) => [m.id, m]));
  return policies
    .filter((p): p is ApiPolicy & { kind: "alert" | "default_alert" } => p.kind === "alert" || p.kind === "default_alert")
    .sort((a, b) => (a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind === "default_alert" ? -1 : 1))
    .map((p) => {
      const customerName = p.customerId ? cust.get(p.customerId) ?? t("customer") : "";
      const op = p.operator ?? "gte";
      const metric = p.metric ?? "";
      const rules = p.rules?.length ?? 0;
      const sub = p.kind === "default_alert"
        ? t(rules === 1 ? "1 rule · ventilation (CO₂, PM2.5) + fault causes" : "{n} rules · ventilation (CO₂, PM2.5) + fault causes", { n: rules })
        : `${metricLabel[metric] ? t(metricLabel[metric]) : metric} ${opSymbol[op]} ${p.threshold} ${metricUnit[metric] ?? ""} ${t("for {n} s", { n: p.durationSeconds ?? 0 })}`;
      const recipients = (p.recipientMembershipIds ?? []).map((id) => {
        const m = member.get(id);
        return { id, label: m?.displayName || id, role: m ? (roleWord[m.role] ? t(roleWord[m.role]) : m.role) : "" };
      });
      return {
        id: p.id, version: p.version, kind: p.kind, name: p.name, group: p.kind === "default_alert" ? t("DEFAULT · ON EVERY UNIT") : customerName.toUpperCase(),
        sub, on: p.enabled, units: p.kind === "default_alert" ? t("every unit") : t(p.unitIds.length === 1 ? "1 unit" : "{n} units", { n: p.unitIds.length }),
        customerId: p.customerId ?? null, customerName, priority: p.priority, timezone: p.timezone, metric: p.metric ?? "temperature", operator: op,
        threshold: p.threshold ?? 0, recovery: p.recoveryThreshold ?? 0, duration: p.durationSeconds ?? 60, severity: p.severity ?? "warning",
        cooldown: p.cooldownMinutes ?? 5, escalate: p.escalateAfterMinutes ?? 60, channels: p.channels ?? ["inApp"], recipients,
        recipientIds: p.recipientMembershipIds ?? [], activeWindow: p.activeWindow ?? null,
      };
    });
}

/** The policy form checks (IR66 limits): name 1–120, duration 1–86400 s, cooldown and escalation 1–1440 min, and the
 * recovery on the safe side of the threshold. */
export function policyErrors(p: Pick<AdminPolicy, "name" | "duration" | "cooldown" | "escalate" | "operator" | "threshold" | "recovery">, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!p.name.trim() || p.name.length > 120) e.name = t("1–120 characters");
  if (!(p.duration >= 1 && p.duration <= 86400)) e.duration = t("Duration must be 1–86400 s");
  if (!(p.cooldown >= 1 && p.cooldown <= 1440)) e.cooldown = t("1–1440");
  if (!(p.escalate >= 1 && p.escalate <= 1440)) e.escalate = t("1–1440");
  const rec = recoveryError(p.operator, p.threshold, p.recovery, t);
  if (rec) e.recovery = rec;
  return e;
}

/** policies.save input for an edited alert policy (AlertPolicyInput of service-contracts.ts). */
export function alertPolicyInput(p: AdminPolicy): OpInput<"policies.save"> {
  return {
    // an alert policy always belongs to a customer (the Default policy is saved as default_alert)
    id: p.id, kind: "alert" as const, name: p.name, timezone: p.timezone, enabled: p.on, priority: p.priority, customerId: p.customerId!,
    metric: p.metric as ContractMetric, operator: p.operator, threshold: p.threshold, recoveryThreshold: p.recovery, durationSeconds: p.duration,
    activeWindow: p.activeWindow ?? null, severity: p.severity, recipientMembershipIds: p.recipientIds, channels: p.channels,
    escalateAfterMinutes: p.escalate, cooldownMinutes: p.cooldown,
  };
}
