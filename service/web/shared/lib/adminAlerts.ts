// HQ alerts and alert policies (FR-A05, DATA_SOURCE=api): alerts.list, policies.list, units.list, customers.list and
// members.list projected for the admin alerts screen. Pure code shared by the Server Component and the client view.
import { translator, type T } from "@ac/web/lib/i18n";
import { alertTitle } from "@ac/web/lib/alerts";

export type Severity = "critical" | "warning" | "normal";
export type Operator = "gt" | "gte" | "lt" | "lte";
export type Channel = "inApp" | "email" | "whatsapp";

/** Alert of service-contracts.ts. */
export type ApiAdminAlert = {
  id: string; version: number; unitId: string; policyId: string | null; type: string; severity: Severity; status: "open" | "acknowledged" | "resolved";
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

export type AdminAlert = {
  id: string; version: number; title: string; time: string; sev: Severity; meta: string; st: "Open" | "Acknowledged" | "Resolved";
  unit: string; context: string; evidence: string; cause: string; observed: string; evidenceRecords: string;
  timeline: { time: string; title: string; detail: string; tone?: "warn" | "ok" }[];
};

export type AdminPolicy = {
  id: string; version: number; kind: "alert" | "default_alert"; name: string; group: string; sub: string; on: boolean; units: string;
  customerId: string | null; customerName: string; priority: number; timezone: string; metric: string; operator: Operator;
  threshold: number; recovery: number; duration: number; severity: Severity; cooldown: number; escalate: number; channels: Channel[];
  recipients: { id: string; label: string; role: string }[]; recipientIds: string[]; activeWindow: ApiPolicy["activeWindow"];
};

const KL = "Asia/Kuala_Lumpur";
const hm = (iso: string) => new Date(iso).toLocaleTimeString("en-GB", { timeZone: KL, hour: "2-digit", minute: "2-digit" });
const full = (iso: string) => `${new Date(iso).toLocaleString("en-CA", { timeZone: KL, hour12: false }).replace(",", "").slice(0, 16)} MYT`;

export const metricLabel: Record<string, string> = {
  temperature: "Temperature", humidity: "Humidity", co2: "CO₂ (ppm)", pm25: "PM2.5", power: "Power", vibration: "Vibration",
  refrigerant_pressure: "Refrigerant pressure", compressor_cycles: "Compressor cycles", airflow_drop: "Airflow drop", heartbeat_gap: "Heartbeat gap",
};
export const metricUnit: Record<string, string> = { temperature: "°C", humidity: "%", co2: "ppm", pm25: "µg/m³", power: "kW", vibration: "mm/s", refrigerant_pressure: "kPa", compressor_cycles: "cycles/h", airflow_drop: "%", heartbeat_gap: "min" };
export const opSymbol: Record<Operator, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" };
const sevRank: Record<Severity, number> = { critical: 0, warning: 1, normal: 2 };
const stRank = { open: 0, acknowledged: 1, resolved: 2 } as const;
const stLabel = { open: "Open", acknowledged: "Acknowledged", resolved: "Resolved" } as const;

/** Recovery must lie on the safe side of the threshold for the operator (≥ / >: below, ≤ / <: above). */
export function recoveryError(op: Operator, threshold: number, recovery: number, t: T = translator("en")): string | undefined {
  const above = op === "gt" || op === "gte";
  if (above ? recovery < threshold : recovery > threshold) return undefined;
  return t(above ? "Recovery {recovery} must be below the {threshold} threshold (direction for {op})" : "Recovery {recovery} must be above the {threshold} threshold (direction for {op})", { recovery, threshold, op: opSymbol[op] });
}

export function adminAlertRows(alerts: ApiAdminAlert[], units: ApiUnitName[], customers: ApiCustomerName[], policies: ApiPolicy[]): AdminAlert[] {
  const unit = new Map(units.map((u) => [u.id, u]));
  const custByOrg = new Map(customers.map((c) => [c.organizationId, c.name]));
  const policy = new Map(policies.map((p) => [p.id, p.name]));
  return [...alerts]
    .sort((a, b) => stRank[a.status] - stRank[b.status] || sevRank[a.severity] - sevRank[b.severity] || b.detectedAt.localeCompare(a.detectedAt))
    .map((a) => {
      const u = unit.get(a.unitId);
      const customer = (u && custByOrg.get(u.customerOrgId)) ?? "unknown customer";
      const pol = a.policyId ? policy.get(a.policyId) ?? "policy" : "no policy";
      const timeline: AdminAlert["timeline"] = [{ time: hm(a.detectedAt), title: "Alert detected", detail: a.evidenceText, tone: "warn" }];
      if (a.acknowledgedAt) timeline.push({ time: hm(a.acknowledgedAt), title: "Acknowledged", detail: "Acknowledging does not resolve the alert" });
      timeline.push(a.resolvedAt
        ? { time: hm(a.resolvedAt), title: "Resolved", detail: a.resolutionReason ?? "", tone: "ok" }
        : { time: "—", title: a.acknowledgedAt ? "Acknowledged — waiting for resolution" : "Waiting for acknowledgement", detail: "" });
      return {
        id: a.id, version: a.version, title: alertTitle(a), time: hm(a.detectedAt), sev: a.severity,
        meta: `${customer} · ${u?.displayName ?? a.unitId} · ${pol}`, st: stLabel[a.status],
        unit: `${u?.displayName ?? "Unit"} · ${a.unitId}`, context: customer, evidence: `${a.evidenceKind} — “${a.evidenceText}”`,
        cause: a.causeCode === "unknown" ? "not determined" : `${a.causeCode} (suspected)`, observed: full(a.observedAt),
        evidenceRecords: a.evidenceIds.length ? `${a.evidenceIds.length} attached` : "none attached", timeline,
      };
    });
}

export function policyRows(policies: ApiPolicy[], customers: ApiCustomerName[], members: ApiMemberName[]): AdminPolicy[] {
  const cust = new Map(customers.map((c) => [c.id, c.name]));
  const member = new Map(members.map((m) => [m.id, m]));
  return policies
    .filter((p): p is ApiPolicy & { kind: "alert" | "default_alert" } => p.kind === "alert" || p.kind === "default_alert")
    .sort((a, b) => (a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind === "default_alert" ? -1 : 1))
    .map((p) => {
      const customerName = p.customerId ? cust.get(p.customerId) ?? "customer" : "";
      const op = p.operator ?? "gte";
      const sub = p.kind === "default_alert"
        ? `${p.rules?.length ?? 0} rules · ventilation (CO₂, PM2.5) + fault causes`
        : `${metricLabel[p.metric ?? ""] ?? p.metric} ${opSymbol[op]} ${p.threshold} ${metricUnit[p.metric ?? ""] ?? ""} for ${p.durationSeconds} s`;
      const recipients = (p.recipientMembershipIds ?? []).map((id) => { const m = member.get(id); return { id, label: m?.displayName || id, role: m?.role ?? "" }; });
      return {
        id: p.id, version: p.version, kind: p.kind, name: p.name, group: p.kind === "default_alert" ? "DEFAULT · ON EVERY UNIT" : customerName.toUpperCase(),
        sub, on: p.enabled, units: p.kind === "default_alert" ? "every unit" : `${p.unitIds.length} unit${p.unitIds.length === 1 ? "" : "s"}`,
        customerId: p.customerId ?? null, customerName, priority: p.priority, timezone: p.timezone, metric: p.metric ?? "temperature", operator: op,
        threshold: p.threshold ?? 0, recovery: p.recoveryThreshold ?? 0, duration: p.durationSeconds ?? 60, severity: p.severity ?? "warning",
        cooldown: p.cooldownMinutes ?? 5, escalate: p.escalateAfterMinutes ?? 60, channels: p.channels ?? ["inApp"], recipients,
        recipientIds: p.recipientMembershipIds ?? [], activeWindow: p.activeWindow ?? null,
      };
    });
}

/** policies.save input for an edited alert policy (AlertPolicyInput of service-contracts.ts). */
export function alertPolicyInput(p: AdminPolicy) {
  return {
    id: p.id, kind: "alert" as const, name: p.name, timezone: p.timezone, enabled: p.on, priority: p.priority, customerId: p.customerId,
    metric: p.metric, operator: p.operator, threshold: p.threshold, recoveryThreshold: p.recovery, durationSeconds: p.duration,
    activeWindow: p.activeWindow, severity: p.severity, recipientMembershipIds: p.recipientIds, channels: p.channels,
    escalateAfterMinutes: p.escalate, cooldownMinutes: p.cooldown,
  };
}
