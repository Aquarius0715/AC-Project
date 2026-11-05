// Unit detail types and pure view helpers (units.get, commands, telemetry) shared by Server and Client Components.
// commands.create per changed setting, commands.get polling until the device answers or the command expires (D04),
// and units.setAlertPolicies with the unit version (FR-C03, FR-C15).
import { airNumber } from "@ac/web/lib/air";
import type { Metric, RestrictionAction, UnitAction as AnyUnitAction, UnitSymbol } from "@ac/web/lib/contracts.gen";
import { policyText } from "@ac/web/lib/restrictions";
import { DEFAULT_DISPLAY, showClock, showRecent, showTime, translator, type Display, type T, textOf } from "@ac/web/lib/i18n";

type Mode = "cool" | "dry" | "fan";
type Fan = "low" | "mid" | "high";
export type UnitAction = { kind: "set_power"; power: boolean } | { kind: "set_temperature"; celsius: number } | { kind: "set_mode"; mode: Mode } | { kind: "set_fan"; fanLevel: Fan };
/** What a stored command did (Command.action): a setting of the remote control, ventilation (an automation) or a
restriction's apply / remove (IR314). */
export type CommandAction = AnyUnitAction | RestrictionAction;
/** A unit's run state from its effective power: running, stopped, or unknown without a reported state. */
export type RunState = "running" | "stopped" | "unknown";
const RUN_STATE = new Map<string, RunState>([["on", "running"], ["off", "stopped"]]);
export const runState = (power: string | null | undefined): RunState => RUN_STATE.get(power ?? "") ?? "unknown";

/** UnitDetail of service-contracts.ts (fields shown on the control screen). */
export interface ApiUnitDetail {
  id: string;
  version: number;
  spaceId: string | null;
  displayName: string;
  alertPolicyIds: string[];
  connection: "online" | "offline" | "unknown" | "connecting" | "error";
  effectivePowerState: "on" | "off" | "unknown";
  observedState: { power: boolean | null; celsius: number | null; mode: Mode | null; fanLevel: Fan | null; observedAt: string | null };
  lastSeenAt: string | null;
  /** The latest reading of each metric with read-time quality (IR213: a valid reading past its sensor's stale limit is stale). */
  latestMeasurements: { id: string; unitId: string; sensorId: string; metric: Metric; value: number | null; unit: UnitSymbol; observedAt: string; origin: "measured" | "estimated" | "inspection"; quality: "valid" | "missing" | "stale" | "suspect"; qualityReason: string | null }[];
  capabilities: {
    manufacturer: string; model: string; control: boolean; modeControl: boolean; fanControl: boolean; temperature: { min: number; max: number; step: number } | null; modes: Mode[]; fanLevels: Fan[];
    ventilation: boolean; ventilationLevels: Fan[]; sensors: { metric: Metric; unit: UnitSymbol; staleAfterSeconds: number }[];
  };
  effectiveControlPolicy: { state: "unrestricted" } | { state: "restricted"; phase: string; policy: { kind: "temperature_limit"; minimumCoolingSetpoint: number } | { kind: "power_off" } };
  controlAvailability: { state: "available" } | { state: "blocked"; reasonKey: string };
  pendingCommands: ApiCommand[];
  location: { pathLabels: string[]; address: string | null; accessInstructions: string | null };
  installedAt: string | null;
  capabilityVersion: number;
  serviceScope: ("indoor" | "outdoor" | "electrical")[];
  components: string[];
}

/** What the unit page's remote control offers for a model (deterministic contracts, AT-X06; IR524, IR530): nothing
without control; the temperature row needs a range, the mode and fan rows their control and list only the model's
own values; the change summary and its review bar need a setting to change (the power is sent at once). Without a
model (the demo screen), every control of a full model. */
export interface ControlLayout {
  control: boolean;
  temperature: { min: number; max: number; step: number } | null;
  modes: Mode[];
  fans: Fan[];
  review: boolean;
}
export function controlLayout(cap: ApiUnitDetail["capabilities"] | null): ControlLayout {
  if (!cap) return { control: true, temperature: { min: 16, max: 30, step: 1 }, modes: ["cool", "dry", "fan"], fans: ["low", "mid", "high"], review: true };
  if (!cap.control) return { control: false, temperature: null, modes: [], fans: [], review: false };
  const modes = cap.modeControl ? cap.modes : [];
  const fans = cap.fanControl ? cap.fanLevels : [];
  return { control: true, temperature: cap.temperature, review: cap.temperature !== null || modes.length > 0 || fans.length > 0, modes, fans };
}

/** Whether the unit page's remote control sets the model's temperature (its temperature row): only then do other
screens point a temperature to Unit Control (IR544). */
export function setsTemperature(cap: ApiUnitDetail["capabilities"] | null): boolean {
  return controlLayout(cap).temperature !== null;
}

export interface ApiCommand { id: string; action: CommandAction; status: "requested" | "sent" | "acknowledged" | "failed" | "expired" | "cancelled"; requestedAt: string; acknowledgedAt?: string | null; expiresAt?: string; failureCode: string | null; source?: string }

const en = translator("en");

/** The words for the operating modes and fan levels (the stored values stay cool / dry / fan, low / mid / high). */
export const MODE_LABEL: Record<Mode, string> = { cool: "Cool", dry: "Dry", fan: "Fan" };
export const FAN_LABEL: Record<Fan, string> = { low: "Low", mid: "Mid", high: "High" };

/** A command as a sentence in the display language (IR258); every kind of the contract has its words (IR314). */
export function actionText(a: CommandAction, t: T = en): string {
  switch (a.kind) {
    case "set_power": {
      return t(a.power ? "Set power ON" : "Set power OFF");
    }
    case "set_temperature": {
      return t("Set temperature {celsius}°C", { celsius: a.celsius });
    }
    case "set_mode": {
      return t("Set mode {mode}", { mode: t(MODE_LABEL[a.mode]).toUpperCase() });
    }
    case "set_fan": {
      return t("Set fan {level}", { level: t(FAN_LABEL[a.fanLevel]).toUpperCase() });
    }
    case "ventilate": {
      return t("Set ventilation {level}", { level: t(FAN_LABEL[a.level]).toUpperCase() });
    }
    case "apply_restriction": {
      return t("Apply restriction: {policy}", { policy: policyText(a.policy, t) });
    }
    case "remove_restriction": {
      return t("Remove restriction");
    }
  }
}

const statusText: Record<ApiCommand["status"], string> = { requested: "requested", sent: "waiting for device", acknowledged: "acknowledged by device", failed: "failed", expired: "no device response (expired)", cancelled: "cancelled" };
const sourceText: Record<string, string> = { voice: " · by voice", automation: " · by an automation", restriction: " · by a restriction", diagnostic: " · technician test run" };

/** One command history row: the sentence, its state and who sent it, and when it was requested (IR44 time). */
export function historyRow(c: ApiCommand, t: T = en, display: Display = DEFAULT_DISPLAY) {
  const isBad = c.status === "failed" || c.status === "expired";
  const by = (c.source ? textOf(sourceText, c.source, t) : undefined) ?? "";
  return { id: c.id.slice(0, 8), text: `${actionText(c.action, t)} — ${t(statusText[c.status])}${c.failureCode && c.status === "failed" ? ` (${c.failureCode})` : ""}${by}`, when: showTime(c.requestedAt, display), bad: isBad };
}

const qualityText: Record<string, string> = { missing: "missing", stale: "stale", suspect: "suspect" };

/** The latest reading of a metric as text (IR213): a stale or suspect value keeps its quality beside it — shown, never
as a current value (D07); null without a reading or with a null value (no fallback to an older one). The time is the
IR44 time of day in the user's display time zone, with its date when it is not the scenario's today (`nowMs`, IR613). */
export function latest(d: ApiUnitDetail, metric: string, t: T = en, display: Display = DEFAULT_DISPLAY, nowMs?: number): { text: string; at: string } | null {
  const m = d.latestMeasurements.find((x) => x.metric === metric);
  // with the scenario's now, a reading of another day keeps its date (IR613)
  const at = (iso: string) => (nowMs === undefined ? showClock(iso, display) : showRecent(iso, nowMs, display));
  return m && m.value !== null ? { text: `${airNumber(metric, m.value)} ${m.unit}${m.quality === "valid" ? "" : ` (${t(qualityText[m.quality] ?? m.quality)})`}`, at: at(m.observedAt) } : null;
}

// Technician unit monitoring (FR-T02): the rolling series window for telemetry.series. The component groups of the 18
// ComponentKeys are techJob's componentGroups.
/** Buckets measurements into n equal slots (latest value per slot; null where no valid measurement). */
export function bucket(items: { value: number | null; observedAt: string; quality?: string }[], from: Date, to: Date, n = 12): (number | null)[] {
  const out: (number | null)[] = new Array(n).fill(null);
  const span = to.getTime() - from.getTime();
  for (const m of [...items].toSorted((a, b) => a.observedAt.localeCompare(b.observedAt))) {
    if (m.value === null || (m.quality && m.quality !== "valid")) continue;
    const i = Math.min(n - 1, Math.floor(((new Date(m.observedAt).getTime() - from.getTime()) / span) * n));
    if (i >= 0) out[i] = m.value;
  }
  return out;
}
