// Unit detail and remote control against the Core API (DATA_SOURCE=api): units.get projected for the control screen,
// commands.create per changed setting, commands.get polling until the device answers or the command expires (D04),
// and units.setAlertPolicies with the unit version (FR-C03, FR-C15).
import { callOp } from "@/lib/ops";
import { invalidate } from "@/lib/useOp";

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
  latestMeasurements: { metric: string; value: number | null; unit: string; observedAt: string }[];
  capabilities: { manufacturer: string; model: string; control: boolean; modeControl: boolean; fanControl: boolean; temperature: { min: number; max: number; step: number } | null; modes: Mode[]; fanLevels: Fan[] };
  effectiveControlPolicy: { state: "unrestricted" } | { state: "restricted"; phase: string; policy: { kind: "temperature_limit"; minimumCoolingSetpoint: number } | { kind: "power_off" } };
  controlAvailability: { state: "available" } | { state: "blocked"; reasonKey: string };
  pendingCommands: ApiCommand[];
  location: { pathLabels: string[] };
};

export type ApiCommand = { id: string; action: UnitAction; status: "requested" | "sent" | "acknowledged" | "failed" | "expired" | "cancelled"; requestedAt: string; failureCode: string | null };

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
  return { id: c.id.slice(0, 8), text: `${actionText(c.action)} — ${statusText[c.status]}${c.failureCode && c.status === "failed" ? ` (${c.failureCode})` : ""}`, when: kl(c.requestedAt, true), bad };
}

export function latest(d: ApiUnitDetail, metric: string): { text: string; at: string } | null {
  const m = d.latestMeasurements.find((x) => x.metric === metric && x.value !== null);
  return m ? { text: `${m.value} ${m.unit}`, at: kl(m.observedAt) } : null;
}

const terminal = new Set<ApiCommand["status"]>(["acknowledged", "failed", "expired", "cancelled"]);

/** Sends one command with the current unit version and waits for its end state (polls every 2 s, at most 40 s). */
export async function sendCommand(unitId: string, action: UnitAction, onUpdate: (c: ApiCommand) => void): Promise<ApiCommand> {
  const unit = await callOp<ApiUnitDetail>("units.get", { id: unitId });
  let c = await callOp<ApiCommand>("commands.create", { unitId, action, expectedUnitVersion: unit.version }, { write: true });
  onUpdate(c);
  for (let i = 0; i < 20 && !terminal.has(c.status); i++) {
    await new Promise((r) => setTimeout(r, 2000));
    c = await callOp<ApiCommand>("commands.get", { id: c.id });
    onUpdate(c);
  }
  invalidate();
  return c;
}

export async function setAlertPolicies(unitId: string, alertPolicyIds: string[]) {
  const unit = await callOp<ApiUnitDetail>("units.get", { id: unitId });
  await callOp("units.setAlertPolicies", { unitId, alertPolicyIds }, { write: true, expectedVersion: unit.version });
  invalidate();
}
