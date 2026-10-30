import type { ApiMeasurement } from "@ac/web/lib/air";

export type UnitRow = { id: string; name: string; loc: string; obs: string; temp: number | null; hum: number | null; power: number | null; state: "running" | "stopped" | "unknown"; conn: "online" | "offline" };

export const units: UnitRow[] = [
  { id: "unit-online-rto", name: "Bedroom AC", loc: "Home A › 1F › Bedroom", obs: "09:12", temp: 28.0, hum: 60, power: 680, state: "running", conn: "online" },
  { id: "unit-bedroom-2", name: "Bedroom AC #2", loc: "Home A › 1F › Bedroom", obs: "09:12", temp: 27.0, hum: 58, power: 0, state: "stopped", conn: "online" },
  { id: "unit-non-rto", name: "Living room AC", loc: "Home A › 1F › Living room", obs: "09:11", temp: 26.5, hum: 55, power: 510, state: "running", conn: "online" },
  { id: "unit-kitchen-a", name: "Kitchen AC", loc: "Home A › 1F › Kitchen", obs: "09:12", temp: 27.2, hum: 58, power: 545, state: "running", conn: "online" },
  { id: "unit-study-a", name: "Study AC", loc: "Home A › 2F › Study", obs: "last seen Sep 22 18:40", temp: null, hum: null, power: null, state: "unknown", conn: "offline" },
];

export const week = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

// ---- Core API projections (DATA_SOURCE=api) ----

/** UnitSummary of service-contracts.ts (fields used by the customer screens). */
export type ApiUnit = {
  id: string;
  displayName: string;
  connection: "online" | "offline" | "unknown" | "connecting" | "error";
  effectivePowerState: "on" | "off" | "unknown";
  observedState: { power: boolean | null; celsius: number | null; observedAt: string | null };
  lastSeenAt: string | null;
  latestMeasurements?: Pick<ApiMeasurement, "metric" | "value" | "unit" | "quality">[]; // IR213
};

const hhmm = (iso: string | null) => (iso ? new Date(iso).toLocaleTimeString("en-MY", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "Asia/Kuala_Lumpur" }) : "—");

/** The latest valid reading of a metric (IR213): a stale, suspect or missing one is no current value. */
const reading = (u: ApiUnit, metric: string) => {
  const m = u.latestMeasurements?.find((x) => x.metric === metric);
  return m && m.quality === "valid" && m.value !== null ? m : null;
};

/** The row's room temperature, humidity and power (W) are the unit's latest readings, never its setting. */
export function unitRowFromApi(u: ApiUnit): UnitRow {
  const temp = reading(u, "temperature"), hum = reading(u, "humidity"), power = reading(u, "power");
  return {
    id: u.id,
    name: u.displayName,
    loc: "",
    obs: u.connection === "online" ? hhmm(u.observedState.observedAt) : `last seen ${hhmm(u.lastSeenAt)}`,
    temp: temp && Math.round(temp.value! * 10) / 10,
    hum: hum && Math.round(hum.value!),
    power: power && Math.round(power.unit === "kW" ? power.value! * 1000 : power.value!),
    state: u.effectivePowerState === "on" ? "running" : u.effectivePowerState === "off" ? "stopped" : "unknown",
    conn: u.connection === "online" ? "online" : "offline",
  };
}

export type CustomerCounts = { total: number; powerOn: number; powerOff: number; powerUnknown: number };
export const mockCounts: CustomerCounts = { total: 5, powerOn: 3, powerOff: 1, powerUnknown: 1 };
