// HQ automation policies (FR-A11, DATA_SOURCE=api): kind=automation policies with their When / Then sentences, the
// policies.save input and the evaluation facts of automations.simulate / automations.fire. Pure code shared by the
// Server Component and the client view. Texts in the display language (`t`, IR303).
import { translator, type T } from "@ac/web/lib/i18n";
import type { OpInput } from "@ac/web/lib/opTypes";
import type { Fact } from "@ac/web/lib/contracts.gen";

const en = translator("en");

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
/** The subject of a measured condition, as the sentence starts. */
export const subjectText = (type: "tariff" | "solar" | "battery", t: T = en) =>
  type === "tariff" ? t("When the electricity tariff is") : type === "solar" ? t("When solar output is") : t("When battery output is");
export function conditionText(c: AutoCondition, t: T = en): string {
  switch (c.type) {
    case "occupancy": return c.occupied ? t("When the room is occupied") : t("When the room is not occupied");
    case "peak": return c.active ? t("When a peak period is active") : t("When no peak period is active");
    case "tariff": return `${subjectText("tariff", t)} ${cmp[c.operator]} ${c.value} MYR/kWh`;
    default: return `${subjectText(c.type, t)} ${cmp[c.operator]} ${c.value} kW`;
  }
}
/** A mode or a fan / ventilation level, worded. */
export const modeWord = (m: "cool" | "dry" | "fan", t: T = en) => ({ cool: t("cool"), dry: t("dry"), fan: t("fan") })[m] ?? m;
export const levelWord = (l: "low" | "mid" | "high", t: T = en) => ({ low: t("low"), mid: t("mid"), high: t("high") })[l] ?? l;
export function actionText(a: UnitAction, t: T = en): string {
  switch (a.kind) {
    case "set_power": return a.power ? t("turn the AC on") : t("turn the AC off");
    case "set_temperature": return t("set the temperature to {value} °C", { value: a.celsius });
    case "set_mode": return t("switch to {mode} mode", { mode: modeWord(a.mode, t) });
    case "set_fan": return t("set the fan to {level}", { level: levelWord(a.fanLevel, t) });
    default: return t("ventilate at {level}", { level: levelWord(a.level, t) });
  }
}

export type PolicyGroup = { label: string; rows: { id: string; version: number; name: string; sentence: string; priority: number; enabled: boolean; disabledReason: string | null }[] };
const ACROSS = "\u0000across"; // the group key of policies spanning customers (sorted first)
/** The list grouped by the customer of the target units; policies spanning customers go under “Across customers”. */
export function policyGroups(ps: ApiAutoPolicy[], customerOfUnit: Map<string, string>, t: T = en): PolicyGroup[] {
  const groups = new Map<string, PolicyGroup>();
  for (const p of [...ps].sort((a, b) => b.priority - a.priority || a.id.localeCompare(b.id))) {
    const customers = new Set(p.unitIds.map((u) => customerOfUnit.get(u) ?? t("unknown customer")));
    const key = customers.size === 1 ? [...customers][0] : ACROSS;
    const g = groups.get(key) ?? { label: key === ACROSS ? t("Across customers") : key, rows: [] };
    g.rows.push({
      id: p.id, version: p.version, name: p.name, sentence: `${conditionText(p.condition, t)} → ${actionText(p.action, t)}`, priority: p.priority, enabled: p.enabled,
      disabledReason: p.disabledReason && disabledReasonWord(p.disabledReason, t),
    });
    groups.set(key, g);
  }
  return [...groups.entries()].sort(([a], [b]) => (a === ACROSS ? -1 : b === ACROSS ? 1 : a.localeCompare(b))).map(([, g]) => g);
}

/** Why a policy was turned off automatically, worded; another code stays as it is. */
export const disabledReasonWord = (r: string, t: T = en) =>
  ({ consent_revoked: t("consent revoked"), capability_changed: t("capability changed"), unit_archived: t("unit archived") })[r] ?? r;

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
export function autoErrors(d: AutoDraft, tempRange: { min: number; max: number } | null, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (d.name.trim().length < 1 || d.name.trim().length > 120) e.name = t("1–120 characters");
  const pr = Number(d.priority);
  if (!(Number.isInteger(pr) && pr >= 0 && pr <= 100)) e.priority = t("An integer 0–100");
  if (!d.timezone.trim()) e.timezone = t("Required");
  if (d.unitIds.length === 0) e.unitIds = t("Choose at least one unit");
  if ((d.type === "tariff" || d.type === "solar" || d.type === "battery") && !(d.value.trim() !== "" && Number(d.value) >= 0)) e.value = t("A value ≥ 0");
  if (d.actionKind === "set_temperature") {
    const c = Number(d.celsius);
    if (!Number.isFinite(c)) e.celsius = t("A temperature");
    else if (tempRange && (c < tempRange.min || c > tempRange.max)) e.celsius = t("Within {min}–{max} °C on every target unit", { min: tempRange.min, max: tempRange.max });
  }
  return e;
}
export function autoInput(d: AutoDraft, id?: string): Extract<OpInput<"policies.save">, { kind: "automation" }> {
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
/** One synthetic fact of a simulation: a unit, the kind of fact (the policy's own when omitted; “+ Add fact” adds other
 * kinds, so other policies and customer rules can match too — Figma Admin 361:7456, IR323), its value and quality. */
export type FactRow = { unitId: string; kind?: AutoCondition["type"]; value: string; quality: "valid" | "missing" | "stale" | "suspect" };
/** EvaluationInput for the current business-clock minute (occurredAt must be in it); empty values are null facts and
 * each unit is evaluated once, whatever number of facts it has. */
export function evaluationInput(type: AutoCondition["type"], rows: FactRow[], now: Date, eventId: string): OpInput<"automations.simulate"> {
  const at = new Date(Math.floor(now.getTime() / 60000) * 60000).toISOString();
  return {
    eventId, occurredAt: at, phase: "condition" as const, unitIds: [...new Set(rows.map((r) => r.unitId))],
    facts: rows.map((r) => {
      const f = factOf[r.kind ?? type];
      return {
        unitId: r.unitId, metric: f.metric as Fact["metric"], unit: f.unit, observedAt: at, quality: r.quality,
        value: r.value.trim() === "" ? null : f.boolean ? r.value === "true" : Number(r.value),
      };
    }),
  };
}
/** Why a unit was not acted on (the suppression reasons of automations.simulate / fire), worded for HQ. */
export function reasonWord(reason: string, t: T = en): string {
  const words: Record<string, string> = {
    missing_data: t("missing data — skipped"), stale: t("stale data — skipped"), no_match: t("no rule matched"), consent_revoked: t("consent revoked"), owner_forbidden: t("owner not allowed"),
    restricted: t("blocked by an active restriction"), disabled: t("policy disabled"), busy: t("another operation is running"), offline: t("unit offline"),
    invalid_capability: t("not supported by the unit"), no_control_action: t("no control action"), reconciliation_required: t("reconciliation required"),
  };
  return words[reason] ?? reason;
}
/** A per-unit decision, worded. */
export const decisionWord = (d: string, t: T = en) => ({ selected: t("selected"), suppressed: t("suppressed"), requested: t("requested"), failed: t("failed") })[d] ?? d;
