// HQ automation policies (FR-A11, DATA_SOURCE=api): kind=automation policies with their When / Then sentences, the
// policies.save input and the evaluation facts of automations.simulate / automations.fire. Pure code shared by the
// Server Component and the client view.
export type Compare = "gt" | "gte" | "lt" | "lte";
export type AutoCondition =
  | { type: "occupancy"; occupied: boolean } | { type: "peak"; active: boolean }
  | { type: "tariff"; operator: Compare; value: number; unit: "MYR_per_kWh" } | { type: "solar" | "battery"; operator: Compare; value: number; unit: "kW" };
export type UnitAction =
  | { kind: "set_power"; power: boolean } | { kind: "set_temperature"; celsius: number } | { kind: "set_mode"; mode: "cool" | "dry" | "fan" }
  | { kind: "set_fan"; fanLevel: "low" | "mid" | "high" } | { kind: "ventilate"; level: "low" | "mid" | "high" };
/** Policy (kind automation) of service-contracts.ts. */
export type ApiAutoPolicy = {
  id: string; version: number; kind: "automation"; name: string; unitIds: string[]; ownerMembershipId: string; timezone: string; enabled: boolean; priority: number;
  disabledReason: string | null; condition: AutoCondition; action: UnitAction;
};

const cmp: Record<Compare, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" };
export function conditionText(c: AutoCondition): string {
  switch (c.type) {
    case "occupancy": return c.occupied ? "When the room is occupied" : "When the room is not occupied";
    case "peak": return c.active ? "When a peak period is active" : "When no peak period is active";
    case "tariff": return `When the electricity tariff is ${cmp[c.operator]} ${c.value} MYR/kWh`;
    default: return `When ${c.type} output is ${cmp[c.operator]} ${c.value} kW`;
  }
}
export function actionText(a: UnitAction): string {
  switch (a.kind) {
    case "set_power": return a.power ? "turn the AC on" : "turn the AC off";
    case "set_temperature": return `set the temperature to ${a.celsius} °C`;
    case "set_mode": return `switch to ${a.mode} mode`;
    case "set_fan": return `set the fan to ${a.fanLevel}`;
    default: return `ventilate at ${a.level}`;
  }
}

export type PolicyGroup = { label: string; rows: { id: string; version: number; name: string; sentence: string; priority: number; enabled: boolean; disabledReason: string | null }[] };
/** The list grouped by the customer of the target units; policies spanning customers go under “Across customers”. */
export function policyGroups(ps: ApiAutoPolicy[], customerOfUnit: Map<string, string>): PolicyGroup[] {
  const groups = new Map<string, PolicyGroup>();
  for (const p of [...ps].sort((a, b) => b.priority - a.priority || a.id.localeCompare(b.id))) {
    const customers = new Set(p.unitIds.map((u) => customerOfUnit.get(u) ?? "unknown customer"));
    const label = customers.size === 1 ? [...customers][0] : "Across customers";
    const g = groups.get(label) ?? { label, rows: [] };
    g.rows.push({ id: p.id, version: p.version, name: p.name, sentence: `${conditionText(p.condition)} → ${actionText(p.action)}`, priority: p.priority, enabled: p.enabled, disabledReason: p.disabledReason });
    groups.set(label, g);
  }
  return [...groups.values()].sort((a, b) => (a.label === "Across customers" ? -1 : b.label === "Across customers" ? 1 : a.label.localeCompare(b.label)));
}

/** The editor form (DD-A11 fields, IR07 shared inputs). */
export type AutoDraft = {
  name: string; priority: string; timezone: string; enabled: boolean; unitIds: string[];
  type: AutoCondition["type"]; flag: boolean; operator: Compare; value: string;
  actionKind: UnitAction["kind"]; power: boolean; celsius: string; mode: "cool" | "dry" | "fan"; level: "low" | "mid" | "high";
};
export function autoDraft(p?: ApiAutoPolicy, timezone = "Asia/Kuala_Lumpur"): AutoDraft {
  const c = p?.condition;
  const a = p?.action;
  return {
    name: p?.name ?? "", priority: String(p?.priority ?? 50), timezone: p?.timezone ?? timezone, enabled: p?.enabled ?? false, unitIds: p?.unitIds ?? [],
    type: c?.type ?? "tariff", flag: c?.type === "occupancy" ? c.occupied : c?.type === "peak" ? c.active : false,
    operator: c && "operator" in c ? c.operator : "gt", value: c && "value" in c ? String(c.value) : "0.6",
    actionKind: a?.kind ?? "set_temperature", power: a?.kind === "set_power" ? a.power : false, celsius: a?.kind === "set_temperature" ? String(a.celsius) : "26",
    mode: a?.kind === "set_mode" ? a.mode : "cool", level: a?.kind === "set_fan" ? a.fanLevel : a?.kind === "ventilate" ? a.level : "low",
  };
}
export function autoErrors(d: AutoDraft, tempRange: { min: number; max: number } | null): Record<string, string> {
  const e: Record<string, string> = {};
  if (d.name.trim().length < 1 || d.name.trim().length > 120) e.name = "1–120 characters";
  const pr = Number(d.priority);
  if (!(Number.isInteger(pr) && pr >= 0 && pr <= 100)) e.priority = "An integer 0–100";
  if (!d.timezone.trim()) e.timezone = "Required";
  if (d.unitIds.length === 0) e.unitIds = "Choose at least one unit";
  if ((d.type === "tariff" || d.type === "solar" || d.type === "battery") && !(d.value.trim() !== "" && Number(d.value) >= 0)) e.value = "A value ≥ 0";
  if (d.actionKind === "set_temperature") {
    const t = Number(d.celsius);
    if (!Number.isFinite(t)) e.celsius = "A temperature";
    else if (tempRange && (t < tempRange.min || t > tempRange.max)) e.celsius = `Within ${tempRange.min}–${tempRange.max} °C on every target unit`;
  }
  return e;
}
export function autoInput(d: AutoDraft, id?: string) {
  const condition: AutoCondition = d.type === "occupancy" ? { type: "occupancy", occupied: d.flag } : d.type === "peak" ? { type: "peak", active: d.flag }
    : d.type === "tariff" ? { type: "tariff", operator: d.operator, value: Number(d.value), unit: "MYR_per_kWh" } : { type: d.type, operator: d.operator, value: Number(d.value), unit: "kW" };
  const action: UnitAction = d.actionKind === "set_power" ? { kind: "set_power", power: d.power } : d.actionKind === "set_temperature" ? { kind: "set_temperature", celsius: Number(d.celsius) }
    : d.actionKind === "set_mode" ? { kind: "set_mode", mode: d.mode } : d.actionKind === "set_fan" ? { kind: "set_fan", fanLevel: d.level } : { kind: "ventilate", level: d.level };
  return { ...(id ? { id } : {}), kind: "automation" as const, name: d.name.trim(), unitIds: d.unitIds, timezone: d.timezone.trim(), enabled: d.enabled, priority: Number(d.priority), condition, action };
}

/** The fact a condition type reads (IR16 metric/unit table) and how the simulator row is entered. */
export const factOf: Record<AutoCondition["type"], { metric: string; unit: string; boolean: boolean }> = {
  occupancy: { metric: "occupied", unit: "boolean", boolean: true }, peak: { metric: "peak", unit: "boolean", boolean: true },
  tariff: { metric: "tariff", unit: "MYR_per_kWh", boolean: false }, solar: { metric: "solar", unit: "kW", boolean: false }, battery: { metric: "battery", unit: "kW", boolean: false },
};
export type FactRow = { unitId: string; value: string; quality: "valid" | "missing" | "stale" | "suspect" };
/** EvaluationInput for the current business-clock minute (occurredAt must be in it); empty values are null facts. */
export function evaluationInput(type: AutoCondition["type"], rows: FactRow[], now: Date, eventId: string) {
  const f = factOf[type];
  const at = new Date(Math.floor(now.getTime() / 60000) * 60000).toISOString();
  return {
    eventId, occurredAt: at, phase: "condition" as const, unitIds: rows.map((r) => r.unitId),
    facts: rows.map((r) => ({
      unitId: r.unitId, metric: f.metric, unit: f.unit, observedAt: at, quality: r.quality,
      value: r.value.trim() === "" ? null : f.boolean ? r.value === "true" : Number(r.value),
    })),
  };
}
export const reasonText: Record<string, string> = {
  missing_data: "missing data — skipped", stale: "stale data — skipped", no_match: "no rule matched", consent_revoked: "consent revoked", owner_forbidden: "owner not allowed",
  restricted: "blocked by an active restriction", disabled: "policy disabled", busy: "another operation is running", offline: "unit offline",
  invalid_capability: "not supported by the unit", no_control_action: "no control action", reconciliation_required: "reconciliation required",
};
