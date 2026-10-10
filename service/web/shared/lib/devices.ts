// HQ device registry (FR-A04, FR-A20, DATA_SOURCE=api): capabilities, devices with their histories and firmware
// campaigns projected for /admin/devices. Pure code shared by the Server Component and the client view; Vitest covers
// it. Texts in the display language (`i` / `t`, IR295); instants in the user's display time zone, and a campaign's
// start is typed there (NFR-08), while its install window stays the device's local time. Metric and status codes
// stay codes where the API shows them.
import { EN, showTime, translator, zonedInstant, type I18n, type T } from "@ac/web/lib/i18n";
import type { OpInput } from "@ac/web/lib/opTypes";
import type { UnitSymbol } from "@ac/web/lib/contracts.gen";

const en = translator("en");
export type Mode = "cool" | "dry" | "fan";
export type Fan = "low" | "mid" | "high";
export type Metric = "temperature" | "humidity" | "co2" | "pm25" | "power" | "vibration" | "refrigerant_pressure" | "compressor_cycles" | "airflow_drop" | "heartbeat_gap";
export type Connection = "online" | "offline" | "unknown" | "connecting" | "error";

/** The fixed unit of each metric (DD-A04 boundary cases). */
export const metricUnit: Record<Metric, UnitSymbol> = {
  temperature: "°C", humidity: "%", co2: "ppm", pm25: "µg/m³", power: "kW", vibration: "mm/s", refrigerant_pressure: "kPa", compressor_cycles: "cycles/h",
  airflow_drop: "%", heartbeat_gap: "min",
};
export const metrics = Object.keys(metricUnit) as Metric[];
export const modes: Mode[] = ["cool", "dry", "fan"];
export const fans: Fan[] = ["low", "mid", "high"];

/** Capability of service-contracts.ts. */
export type ApiCapability = {
  id: string; version: number; updatedAt: string; manufacturer: string; model: string; control: boolean; modeControl: boolean; fanControl: boolean;
  temperature: { min: number; max: number; step: number } | null; modes: Mode[]; fanLevels: Fan[]; ventilation: boolean; ventilationLevels: Fan[];
  sensors: { metric: Metric; unit: UnitSymbol; staleAfterSeconds: number; boundaryId: string | null }[]; firmwareCandidates: string[];
};
/** The unit fields this screen needs (UnitSummary). */
export type ApiUnitLite = { id: string; displayName: string; modelId: string; customerOrgId: string; archived: boolean };
/** Device / DeviceDetail of service-contracts.ts. */
export type ApiDevice = {
  id: string; version: number; unitId: string | null; serial: string; connection: Connection; lastSeenAt: string | null; firmwareVersion: string;
  powerSignal: "unknown" | "on" | "off"; tamper: "clear" | "detected"; sensors: { id: string; metric: Metric; unit: UnitSymbol; staleAfterSeconds: number; calibratedAt: string | null }[];
};
export type ApiDeviceOperation = { id: string; kind: "check" | "calibrate" | "firmware"; status: "queued" | "running" | "succeeded" | "failed"; targetVersion: string | null; failureCode: string | null; createdAt: string; finishedAt: string | null };
export type ApiDeviceDetail = ApiDevice & { calibrationRefs: string[]; activeOperation: ApiDeviceOperation | null };
export type ApiCalibration = { id: string; sensorId: string; metric: Metric; unit: UnitSymbol; referenceValue: number; measuredValue: number; calibratedAt: string; actorId: string };
export type ApiCampaign = {
  id: string; version: number; modelId: string; fromVersions: string[]; targetVersion: string; deviceIds: string[]; waves: { label: string; percent: number }[];
  window: { startLocal: string; endLocal: string }; autoPauseFailurePercent: number; startAt: string; state: "scheduled" | "running" | "paused" | "aborted" | "completed";
  progress: { succeeded: number; installing: number; pending: number; failed: number; skipped: number };
  results: { deviceId: string; waveIndex: number; status: "pending" | "installing" | "succeeded" | "failed" | "skipped"; reasonKey: string | null }[]; reason: string | null;
};

const KL = "Asia/Kuala_Lumpur";
export const klTime = (iso: string) =>
  new Date(iso).toLocaleString("en-CA", { timeZone: KL, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).replace(",", "");

export type ModelRow = { id: string; version: number; name: string; used: string; ventilation: boolean; updated: string; cap: ApiCapability };

export function modelRows(caps: ApiCapability[], units: ApiUnitLite[], devices: ApiDevice[], i: I18n = EN): ModelRow[] {
  const { t } = i;
  return caps.map((k) => {
    const mine = units.filter((u) => u.modelId === k.id && !u.archived);
    const ids = new Set(mine.map((u) => u.id));
    const devs = devices.filter((d) => d.unitId && ids.has(d.unitId)).length;
    return {
      id: k.id, version: k.version, name: `${k.manufacturer} ${k.model}`, ventilation: k.ventilation, updated: showTime(k.updatedAt, i.display), cap: k,
      used: `${t(mine.length === 1 ? "1 unit" : "{n} units", { n: mine.length })} · ${t(devs === 1 ? "1 device" : "{n} devices", { n: devs })}`,
    };
  });
}

/** The editable capability form (DD-A04 fields). Numbers stay strings until saved. */
export type CapabilityDraft = {
  manufacturer: string; model: string; control: boolean; modeControl: boolean; fanControl: boolean; temperatureOn: boolean; min: string; max: string; step: string;
  modes: Mode[]; fanLevels: Fan[]; ventilation: boolean; ventilationLevels: Fan[]; sensors: { metric: Metric; staleAfterSeconds: string }[]; firmware: string; changeReason: string;
};

export function capabilityDraft(k?: ApiCapability): CapabilityDraft {
  return {
    manufacturer: k?.manufacturer ?? "", model: k?.model ?? "", control: k?.control ?? false, modeControl: k?.modeControl ?? false, fanControl: k?.fanControl ?? false,
    temperatureOn: !!k?.temperature, min: String(k?.temperature?.min ?? 16), max: String(k?.temperature?.max ?? 30), step: String(k?.temperature?.step ?? 1),
    modes: k?.modes ?? [], fanLevels: k?.fanLevels ?? [], ventilation: k?.ventilation ?? false, ventilationLevels: k?.ventilationLevels ?? [],
    sensors: (k?.sensors ?? []).map((s) => ({ metric: s.metric, staleAfterSeconds: String(s.staleAfterSeconds) })), firmware: (k?.firmwareCandidates ?? []).join(", "), changeReason: "",
  };
}

const num = (s: string) => (s.trim() === "" ? NaN : Number(s));

/** The capability rules of DD-A04 (boundary cases) checked before capabilities.save; the API checks them again. */
export function capabilityErrors(d: CapabilityDraft, update: boolean, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (d.manufacturer.trim().length < 1 || d.manufacturer.trim().length > 120) e.manufacturer = t("1–120 characters");
  if (d.model.trim().length < 1 || d.model.trim().length > 120) e.model = t("1–120 characters");
  if ((d.modeControl || d.fanControl || d.temperatureOn) && !d.control) e.control = t("Mode, fan or temperature control needs power control");
  if (d.modeControl && d.modes.length === 0) e.modes = t("Choose at least one mode");
  if (d.fanControl && d.fanLevels.length === 0) e.fanLevels = t("Choose at least one fan level");
  if (d.temperatureOn) {
    const [min, max, step] = [num(d.min), num(d.max), num(d.step)];
    if (!(min < max)) e.temperature = t("min must be below max");
    else if (!(step > 0 && step <= max - min)) e.temperature = t("0 < step ≤ max − min");
  }
  if (d.ventilation && (d.ventilationLevels.length === 0 || !d.ventilationLevels.includes("low"))) e.ventilationLevels = t("Ventilation levels must include Low");
  const seen = new Set<string>();
  for (const s of d.sensors) {
    if (seen.has(s.metric)) e.sensors = t("One sensor per metric ({metric} twice)", { metric: s.metric });
    seen.add(s.metric);
    const v = num(s.staleAfterSeconds);
    if (!(Number.isInteger(v) && v >= 10 && v <= 86400)) e.sensors = t("Stale after must be 10–86400 seconds");
  }
  if (d.firmware.split(",").map((x) => x.trim()).some((x) => x.length > 32)) e.firmware = t("Firmware versions are at most 32 characters");
  if (update && (d.changeReason.trim().length < 1 || d.changeReason.trim().length > 1000)) e.changeReason = t("A reason is required (1–1000 characters)");
  return e;
}

/** The capabilities.save input (Save<Capability> & {changeReason}); sensor units and boundaries are derived (IR11/12). */
export function capabilityInput(d: CapabilityDraft, id?: string): OpInput<"capabilities.save"> {
  return {
    ...(id ? { id, changeReason: d.changeReason.trim() } : {}),
    manufacturer: d.manufacturer.trim(), model: d.model.trim(), control: d.control, modeControl: d.modeControl, fanControl: d.fanControl,
    temperature: d.temperatureOn ? { min: num(d.min), max: num(d.max), step: num(d.step) } : null,
    modes: d.modeControl ? d.modes : [], fanLevels: d.fanControl ? d.fanLevels : [], ventilation: d.ventilation, ventilationLevels: d.ventilation ? d.ventilationLevels : [],
    sensors: d.sensors.map((s) => ({ metric: s.metric, unit: metricUnit[s.metric], staleAfterSeconds: num(s.staleAfterSeconds), boundaryId: s.metric === "power" ? "ac_input_electricity" : null })),
    firmwareCandidates: [...new Set(d.firmware.split(",").map((x) => x.trim()).filter(Boolean))],
  };
}

export type DeviceRow = { id: string; version: number; serial: string; unitId: string | null; unit: string; conn: Connection; fw: string; tamper: boolean; power: string };

export function deviceRows(devices: ApiDevice[], units: ApiUnitLite[], t: T = en): DeviceRow[] {
  const name = new Map(units.map((u) => [u.id, u.displayName]));
  return devices.map((d) => ({
    id: d.id, version: d.version, serial: d.serial, unitId: d.unitId, unit: d.unitId ? name.get(d.unitId) ?? t("unit") : t("Unbound"), conn: d.connection, fw: d.firmwareVersion,
    tamper: d.tamper === "detected", power: d.powerSignal,
  }));
}

export type CampaignRow = { id: string; version: number; model: string; versions: string; state: ApiCampaign["state"]; devices: number; start: string; c: ApiCampaign };
export function campaignRows(cs: ApiCampaign[], caps: ApiCapability[], i: I18n = EN): CampaignRow[] {
  const { t } = i;
  const model = new Map(caps.map((k) => [k.id, `${k.manufacturer} ${k.model}`]));
  return cs.map((c) => ({
    id: c.id, version: c.version, model: model.get(c.modelId) ?? t("model"), versions: `${c.fromVersions.join(", ") || t("any")} → ${c.targetVersion}`, state: c.state,
    devices: c.deviceIds.length, start: showTime(c.startAt, i.display), c,
  }));
}

/** The firmware campaign form (DD-A20): waves ascend and end at 100 %, auto-pause 1–50 %, start ≥ 24 h ahead. The start
 * is a datetime-local value in the user's display time zone (NFR-08); the window is the device's local time. The wave
 * labels are stored with the campaign, so they stay English data; the screen words each wave itself. */
export type CampaignDraft = { modelId: string; targetVersion: string; deviceIds: string[]; waves: string; startLocal: string; endLocal: string; autoPause: string; startAt: string };
export function parseWaves(s: string): { label: string; percent: number }[] | null {
  const ps = s.split(",").map((x) => Number(x.replace("%", "").trim()));
  if (ps.length === 0 || ps.some((p) => !Number.isInteger(p) || p < 1 || p > 100)) return null;
  for (let i = 1; i < ps.length; i++) if (ps[i] <= ps[i - 1]) return null;
  if (ps[ps.length - 1] !== 100) return null;
  return ps.map((p, i) => ({ label: i === ps.length - 1 ? `Wave ${i + 1} · 100 %` : `Wave ${i + 1} · ${p} %`, percent: p }));
}
/** The campaign's start as an instant: the datetime-local value read in the display time zone ("" while empty). */
export const campaignStart = (d: Pick<CampaignDraft, "startAt">, zone: string) => (d.startAt ? zonedInstant(d.startAt.slice(0, 10), d.startAt.slice(11, 16), zone) : "");
export function campaignErrors(d: CampaignDraft, now: Date, zone = "Asia/Kuala_Lumpur", t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!d.modelId) e.modelId = t("Choose a model");
  if (!d.targetVersion) e.targetVersion = t("Choose a target version");
  if (d.deviceIds.length === 0) e.deviceIds = t("Choose at least one device");
  if (!parseWaves(d.waves)) e.waves = t("Ascending percentages ending at 100, e.g. 10, 50, 100");
  const ap = Number(d.autoPause);
  if (!(Number.isInteger(ap) && ap >= 1 && ap <= 50)) e.autoPause = t("1–50 %");
  if (!/^\d{2}:\d{2}$/.test(d.startLocal) || !/^\d{2}:\d{2}$/.test(d.endLocal)) e.window = t("HH:MM – HH:MM");
  const at = Date.parse(campaignStart(d, zone));
  if (!d.startAt || !(at >= now.getTime() + 24 * 3600 * 1000)) e.startAt = t("At least 24 hours ahead");
  return e;
}
