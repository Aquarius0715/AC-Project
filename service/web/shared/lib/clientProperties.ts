// Customer units & locations (FR-C02, FR-C14, DATA_SOURCE=api): the room list of air conditioners and the group
// control plan for one space. Pure code shared by the Server Component and the client view; the tree itself comes from
// @ac/web/lib/assets (locationTree, selection). Texts in the display language and times in the user's display time
// zone (`i`, IR263).
import { one } from "@ac/web/lib/energy";
import { EN, showClock, translator, type I18n, type T } from "@ac/web/lib/i18n";
import type { ApiUnitDetail, UnitAction } from "@ac/web/lib/units";

type Mode = "cool" | "dry" | "fan";
type Fan = "low" | "mid" | "high";
const modeLabel: Record<Mode, string> = { cool: "Cool", dry: "Dry", fan: "Fan" };
const fanLabel: Record<Fan, string> = { low: "low", mid: "mid", high: "high" };
const en = translator("en");

/** One air conditioner of a room (Figma Client 02d): setting, room temperature, power draw, power and connection. */
export type RoomUnit = {
  id: string; version: number; name: string; set: string; temp: string; watts: string; power: "running" | "stopped" | "unknown";
  conn: ApiUnitDetail["connection"]; seen: string | null; online: boolean;
};
/** The latest reading of a metric when it is current (valid, IR213); a stale, suspect or null reading shows “—”. */
const metric = (d: ApiUnitDetail, m: string) => { const x = d.latestMeasurements.find((r) => r.metric === m); return x && x.quality === "valid" ? x.value : null; };
export function roomUnits(ds: ApiUnitDetail[], i: I18n = EN): RoomUnit[] {
  const { t } = i;
  return [...ds].sort((a, b) => a.displayName.localeCompare(b.displayName)).map((d) => {
    const o = d.observedState;
    const set = [o.celsius !== null ? t("Set {celsius}°C", { celsius: o.celsius }) : null, o.mode ? t(modeLabel[o.mode]) : null, o.fanLevel ? t("Fan {level}", { level: t(fanLabel[o.fanLevel]) }) : null].filter(Boolean).join(" · ");
    const temp = metric(d, "temperature");
    const kw = metric(d, "power");
    return {
      id: d.id, version: d.version, name: d.displayName, set: set || t("No setting reported yet"), temp: temp === null ? "—" : t("{temp} °C room", { temp: one(temp) }),
      watts: kw === null ? "—" : `${Math.round(kw * 1000)} W`, power: d.effectivePowerState === "on" ? "running" : d.effectivePowerState === "off" ? "stopped" : "unknown",
      conn: d.connection, seen: d.lastSeenAt ? showClock(d.lastSeenAt, i.display) : null, online: d.connection === "online",
    };
  });
}

/** The group change (FR-C14): power, and with power on a setpoint, mode and fan level (null = keep the fan). */
export type GroupChange = { power: boolean; celsius: number; mode: Mode; fan: Fan | null };
/** `result` is the plan's outcome (its English word, also the dictionary key); change and note are in the display language. */
export type PlanRow = { id: string; name: string; change: string; result: "Will send" | "Clamped" | "Skipped" | "No change"; note: string; actions: UnitAction[] };
/** Per AC: the commands that make its observed setting match the change (one commands.create per changed setting, as
 * single control sends them), clamped to its capability range and an active temperature restriction (IR46), or the
 * reason it is skipped (offline, no remote control, blocked, power-off restriction). */
export function groupPlan(ds: ApiUnitDetail[], ch: GroupChange, i: I18n = EN): PlanRow[] {
  const { t } = i;
  const onOff = (v: boolean) => t(v ? "On" : "Off");
  return ds.map((d) => {
    const o = d.observedState;
    const cap = d.capabilities;
    const skip = (note: string): PlanRow => ({ id: d.id, name: d.displayName, change: "—", result: "Skipped", note, actions: [] });
    if (d.connection !== "online") return skip(d.lastSeenAt ? t("Offline since {time} — not sent", { time: showClock(d.lastSeenAt, i.display) }) : t("Offline — not sent"));
    if (!cap.control) return skip(t("No remote control on this model"));
    if (d.controlAvailability.state === "blocked") return skip(t("Control blocked ({reason})", { reason: d.controlAvailability.reasonKey.replace(/^errors?\./, "").replace(/_/g, " ") }));
    const restriction = d.effectiveControlPolicy.state === "restricted" ? d.effectiveControlPolicy.policy : null;
    if (ch.power && restriction?.kind === "power_off") return skip(t("Power-off restriction — stays off"));
    const actions: UnitAction[] = [];
    const parts: string[] = [];
    let note = "";
    if (o.power !== ch.power) {
      actions.push({ kind: "set_power", power: ch.power });
      parts.push(`${o.power === null ? "?" : onOff(o.power)} → ${onOff(ch.power)}`);
    }
    if (ch.power) {
      if (cap.temperature) {
        let c = Math.min(cap.temperature.max, Math.max(cap.temperature.min, ch.celsius));
        if (c !== ch.celsius) note = t("{min}–{max} °C on this model", { min: cap.temperature.min, max: cap.temperature.max });
        if (restriction?.kind === "temperature_limit" && c < restriction.minimumCoolingSetpoint) {
          c = restriction.minimumCoolingSetpoint;
          note = t("minimum {celsius} °C (restriction)", { celsius: c });
        }
        if (o.celsius !== c) {
          actions.push({ kind: "set_temperature", celsius: c });
          parts.push(`${o.celsius ?? "?"} → ${c} °C`);
        }
      }
      if (cap.modeControl && cap.modes.includes(ch.mode) && o.mode !== ch.mode) {
        actions.push({ kind: "set_mode", mode: ch.mode });
        parts.push(t(modeLabel[ch.mode]));
      }
      if (ch.fan && cap.fanControl && cap.fanLevels.includes(ch.fan) && o.fanLevel !== ch.fan) {
        actions.push({ kind: "set_fan", fanLevel: ch.fan });
        parts.push(t("Fan {level}", { level: t(fanLabel[ch.fan]) }));
      }
    }
    if (!actions.length) return { id: d.id, name: d.displayName, change: t("already set"), result: "No change", note: "", actions };
    return { id: d.id, name: d.displayName, change: parts.join(", "), result: note ? "Clamped" : "Will send", note, actions };
  });
}
export const changeText = (ch: GroupChange, t: T = en) =>
  (ch.power ? t("Power On · {celsius} °C · {mode} · Fan {fan}", { celsius: ch.celsius, mode: t(modeLabel[ch.mode]), fan: t(ch.fan ? fanLabel[ch.fan] : "unchanged") }) : t("Power Off"));
