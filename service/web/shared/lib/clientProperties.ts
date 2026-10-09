// Customer units & locations (FR-C02, FR-C14, DATA_SOURCE=api): the room list of air conditioners and the group
// control plan for one space. Pure code shared by the Server Component and the client view; the tree itself comes from
// @ac/web/lib/assets (locationTree, selection).
import { klStamp, one } from "@ac/web/lib/energy";
import type { ApiUnitDetail, UnitAction } from "@ac/web/lib/units";

type Mode = "cool" | "dry" | "fan";
type Fan = "low" | "mid" | "high";
const modeLabel: Record<Mode, string> = { cool: "Cool", dry: "Dry", fan: "Fan" };
const fanLabel: Record<Fan, string> = { low: "low", mid: "mid", high: "high" };

/** One air conditioner of a room (Figma Client 02d): setting, room temperature, power draw, power and connection. */
export type RoomUnit = {
  id: string; version: number; name: string; set: string; temp: string; watts: string; power: "running" | "stopped" | "unknown";
  conn: ApiUnitDetail["connection"]; seen: string | null; online: boolean;
};
const metric = (d: ApiUnitDetail, m: string) => d.latestMeasurements.find((x) => x.metric === m && x.value !== null)?.value ?? null;
export function roomUnits(ds: ApiUnitDetail[]): RoomUnit[] {
  return [...ds].sort((a, b) => a.displayName.localeCompare(b.displayName)).map((d) => {
    const o = d.observedState;
    const set = [o.celsius !== null ? `Set ${o.celsius}°C` : null, o.mode ? modeLabel[o.mode] : null, o.fanLevel ? `Fan ${fanLabel[o.fanLevel]}` : null].filter(Boolean).join(" · ");
    const t = metric(d, "temperature");
    const kw = metric(d, "power");
    return {
      id: d.id, version: d.version, name: d.displayName, set: set || "No setting reported yet", temp: t === null ? "—" : `${one(t)} °C room`,
      watts: kw === null ? "—" : `${Math.round(kw * 1000)} W`, power: d.effectivePowerState === "on" ? "running" : d.effectivePowerState === "off" ? "stopped" : "unknown",
      conn: d.connection, seen: d.lastSeenAt ? klStamp(d.lastSeenAt).slice(11) : null, online: d.connection === "online",
    };
  });
}

/** The group change (FR-C14): power, and with power on a setpoint, mode and fan level (null = keep the fan). */
export type GroupChange = { power: boolean; celsius: number; mode: Mode; fan: Fan | null };
export type PlanRow = { id: string; name: string; change: string; result: "Will send" | "Clamped" | "Skipped" | "No change"; note: string; actions: UnitAction[] };
/** Per AC: the commands that make its observed setting match the change (one commands.create per changed setting, as
 * single control sends them), clamped to its capability range and an active temperature restriction (IR46), or the
 * reason it is skipped (offline, no remote control, blocked, power-off restriction). */
export function groupPlan(ds: ApiUnitDetail[], ch: GroupChange): PlanRow[] {
  return ds.map((d) => {
    const o = d.observedState;
    const cap = d.capabilities;
    const skip = (note: string): PlanRow => ({ id: d.id, name: d.displayName, change: "—", result: "Skipped", note, actions: [] });
    if (d.connection !== "online") return skip(`Offline${d.lastSeenAt ? ` since ${klStamp(d.lastSeenAt).slice(11)}` : ""} — not sent`);
    if (!cap.control) return skip("No remote control on this model");
    if (d.controlAvailability.state === "blocked") return skip(`Control blocked (${d.controlAvailability.reasonKey.replace(/^errors?\./, "").replace(/_/g, " ")})`);
    const restriction = d.effectiveControlPolicy.state === "restricted" ? d.effectiveControlPolicy.policy : null;
    if (ch.power && restriction?.kind === "power_off") return skip("Power-off restriction — stays off");
    const actions: UnitAction[] = [];
    const parts: string[] = [];
    let note = "";
    if (o.power !== ch.power) {
      actions.push({ kind: "set_power", power: ch.power });
      parts.push(`${o.power === null ? "?" : o.power ? "On" : "Off"} → ${ch.power ? "On" : "Off"}`);
    }
    if (ch.power) {
      if (cap.temperature) {
        let c = Math.min(cap.temperature.max, Math.max(cap.temperature.min, ch.celsius));
        if (c !== ch.celsius) note = `${cap.temperature.min}–${cap.temperature.max} °C on this model`;
        if (restriction?.kind === "temperature_limit" && c < restriction.minimumCoolingSetpoint) {
          c = restriction.minimumCoolingSetpoint;
          note = `minimum ${c} °C (restriction)`;
        }
        if (o.celsius !== c) {
          actions.push({ kind: "set_temperature", celsius: c });
          parts.push(`${o.celsius ?? "?"} → ${c} °C`);
        }
      }
      if (cap.modeControl && cap.modes.includes(ch.mode) && o.mode !== ch.mode) {
        actions.push({ kind: "set_mode", mode: ch.mode });
        parts.push(modeLabel[ch.mode]);
      }
      if (ch.fan && cap.fanControl && cap.fanLevels.includes(ch.fan) && o.fanLevel !== ch.fan) {
        actions.push({ kind: "set_fan", fanLevel: ch.fan });
        parts.push(`Fan ${fanLabel[ch.fan]}`);
      }
    }
    if (!actions.length) return { id: d.id, name: d.displayName, change: "already set", result: "No change", note: "", actions };
    return { id: d.id, name: d.displayName, change: parts.join(", "), result: note ? "Clamped" : "Will send", note, actions };
  });
}
export const changeText = (ch: GroupChange) => (ch.power ? `Power On · ${ch.celsius} °C · ${modeLabel[ch.mode]} · Fan ${ch.fan ? fanLabel[ch.fan] : "unchanged"}` : "Power Off");
