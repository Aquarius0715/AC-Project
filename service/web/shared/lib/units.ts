// Unit detail types and pure view helpers (units.get, commands, telemetry) shared by Server and Client Components.
// commands.create per changed setting, commands.get polling until the device answers or the command expires (D04),
// and units.setAlertPolicies with the unit version (FR-C03, FR-C15).
import { airNumber } from "@ac/web/lib/air";

type Mode = "cool" | "dry" | "fan";
type Fan = "low" | "mid" | "high";
export type UnitAction = { kind: "set_power"; power: boolean } | { kind: "set_temperature"; celsius: number } | { kind: "set_mode"; mode: Mode } | { kind: "set_fan"; fanLevel: Fan };

/** UnitDetail of service-contracts.ts (fields shown on the control screen). */
export type ApiUnitDetail = {
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
  latestMeasurements: { id: string; unitId: string; sensorId: string; metric: string; value: number | null; unit: string; observedAt: string; origin: "measured" | "estimated" | "inspection"; quality: "valid" | "missing" | "stale" | "suspect"; qualityReason: string | null }[];
  capabilities: {
    manufacturer: string; model: string; control: boolean; modeControl: boolean; fanControl: boolean; temperature: { min: number; max: number; step: number } | null; modes: Mode[]; fanLevels: Fan[];
    ventilation: boolean; ventilationLevels: Fan[]; sensors: { metric: string; unit: string; staleAfterSeconds: number }[];
  };
  effectiveControlPolicy: { state: "unrestricted" } | { state: "restricted"; phase: string; policy: { kind: "temperature_limit"; minimumCoolingSetpoint: number } | { kind: "power_off" } };
  controlAvailability: { state: "available" } | { state: "blocked"; reasonKey: string };
  pendingCommands: ApiCommand[];
  location: { pathLabels: string[]; address: string | null; accessInstructions: string | null };
  installedAt: string | null;
  capabilityVersion: number;
  serviceScope: ("indoor" | "outdoor" | "electrical")[];
  components: string[];
};

export type ApiCommand = { id: string; action: UnitAction; status: "requested" | "sent" | "acknowledged" | "failed" | "expired" | "cancelled"; requestedAt: string; failureCode: string | null; source?: string };

const kl = (iso: string | null, withDate = false) =>
  iso ? new Date(iso).toLocaleString("en-MY", { ...(withDate ? { dateStyle: "medium" } : {}), timeStyle: "short", hour12: false, timeZone: "Asia/Kuala_Lumpur" } as Intl.DateTimeFormatOptions) : "—";
export const klTime = kl;

export function actionText(a: UnitAction): string {
  switch (a.kind) {
    case "set_power":
      return `Set power ${a.power ? "ON" : "OFF"}`;
    case "set_temperature":
      return `Set temperature ${a.celsius}°C`;
    case "set_mode":
      return `Set mode ${a.mode.toUpperCase()}`;
    case "set_fan":
      return `Set fan ${a.fanLevel.toUpperCase()}`;
  }
}

const statusText: Record<ApiCommand["status"], string> = { requested: "requested", sent: "waiting for device", acknowledged: "acknowledged by device", failed: "failed", expired: "no device response (expired)", cancelled: "cancelled" };

export function historyRow(c: ApiCommand) {
  const bad = c.status === "failed" || c.status === "expired";
  const by = c.source === "automation" ? " · by an automation" : c.source === "restriction" ? " · by a restriction" : c.source === "diagnostic" ? " · technician test run" : "";
  return { id: c.id.slice(0, 8), text: `${actionText(c.action)} — ${statusText[c.status]}${c.failureCode && c.status === "failed" ? ` (${c.failureCode})` : ""}${by}`, when: kl(c.requestedAt, true), bad };
}

/** The latest reading of a metric as text (IR213): a stale or suspect value keeps its quality beside it — shown, never
 * as a current value (D07); null without a reading or with a null value (no fallback to an older one). */
export function latest(d: ApiUnitDetail, metric: string): { text: string; at: string } | null {
  const m = d.latestMeasurements.find((x) => x.metric === metric);
  return m && m.value !== null ? { text: `${airNumber(metric, m.value)} ${m.unit}${m.quality === "valid" ? "" : ` (${m.quality})`}`, at: kl(m.observedAt) } : null;
}

// Technician unit register / monitoring (FR-T02, FR-T04): component groups of the 18 ComponentKeys and the
// rolling series window for telemetry.series.
const groupOf: Record<string, "Indoor" | "Outdoor" | "Electrical"> = {
  filter: "Indoor", evaporator_coil: "Indoor", blower_motor: "Indoor", blower_fan: "Indoor", drain_pipe: "Indoor", drain_pan: "Indoor", outlet: "Indoor", louver: "Indoor",
  condenser_coil: "Outdoor", compressor: "Outdoor", fan: "Outdoor", blade: "Outdoor", refrigerant_pipe: "Outdoor",
  thermostat: "Electrical", sensor: "Electrical", capacitor: "Electrical", contactor: "Electrical", wiring: "Electrical",
};

export function componentGroups(keys: string[]): Record<"Indoor" | "Outdoor" | "Electrical", string[]> {
  const out = { Indoor: [] as string[], Outdoor: [] as string[], Electrical: [] as string[] };
  for (const k of keys) out[groupOf[k] ?? "Electrical"].push(k.replace(/_/g, " "));
  return out;
}

export const windowMs = { "1h": 3600e3, "24h": 86400e3, "7d": 7 * 86400e3 } as const;

/** Buckets measurements into n equal slots (latest value per slot; null where no valid measurement). */
export function bucket(items: { value: number | null; observedAt: string; quality?: string }[], from: Date, to: Date, n = 12): (number | null)[] {
  const out: (number | null)[] = Array(n).fill(null);
  const span = to.getTime() - from.getTime();
  for (const m of [...items].sort((a, b) => a.observedAt.localeCompare(b.observedAt))) {
    if (m.value === null || (m.quality && m.quality !== "valid")) continue;
    const i = Math.min(n - 1, Math.floor(((new Date(m.observedAt).getTime() - from.getTime()) / span) * n));
    if (i >= 0) out[i] = m.value;
  }
  return out;
}
