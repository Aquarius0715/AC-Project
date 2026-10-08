// HQ device registry (FR-A04, FR-A20, DATA_SOURCE=api): capabilities, devices with their histories and firmware
// campaigns projected for /admin/devices. Pure code shared by the Server Component and the client view.
export type Mode = "cool" | "dry" | "fan";
export type Fan = "low" | "mid" | "high";
export type Metric = "temperature" | "humidity" | "co2" | "pm25" | "power" | "vibration" | "refrigerant_pressure" | "compressor_cycles" | "airflow_drop" | "heartbeat_gap";
export type Connection = "online" | "offline" | "unknown" | "connecting" | "error";

/** The fixed unit of each metric (DD-A04 boundary cases). */
export const metricUnit: Record<Metric, string> = {
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
  sensors: { metric: Metric; unit: string; staleAfterSeconds: number; boundaryId: string | null }[]; firmwareCandidates: string[];
};
/** The unit fields this screen needs (UnitSummary). */
export type ApiUnitLite = { id: string; displayName: string; modelId: string; customerOrgId: string; archived: boolean };
/** Device / DeviceDetail of service-contracts.ts. */
export type ApiDevice = {
  id: string; version: number; unitId: string | null; serial: string; connection: Connection; lastSeenAt: string | null; firmwareVersion: string;
  powerSignal: "unknown" | "on" | "off"; tamper: "clear" | "detected"; sensors: { id: string; metric: Metric; unit: string; staleAfterSeconds: number; calibratedAt: string | null }[];
};
export type ApiDeviceOperation = { id: string; kind: "check" | "calibrate" | "firmware"; status: "queued" | "running" | "succeeded" | "failed"; targetVersion: string | null; failureCode: string | null; createdAt: string; finishedAt: string | null };
export type ApiDeviceDetail = ApiDevice & { calibrationRefs: string[]; activeOperation: ApiDeviceOperation | null };
export type ApiCalibration = { id: string; sensorId: string; metric: Metric; unit: string; referenceValue: number; measuredValue: number; calibratedAt: string; actorId: string };
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

export function modelRows(caps: ApiCapability[], units: ApiUnitLite[], devices: ApiDevice[]): ModelRow[] {
  return caps.map((k) => {
    const mine = units.filter((u) => u.modelId === k.id && !u.archived);
    const ids = new Set(mine.map((u) => u.id));
    const devs = devices.filter((d) => d.unitId && ids.has(d.unitId)).length;
    return {
      id: k.id, version: k.version, name: `${k.manufacturer} ${k.model}`, ventilation: k.ventilation, updated: klTime(k.updatedAt), cap: k,
      used: `${mine.length} unit${mine.length === 1 ? "" : "s"} · ${devs} device${devs === 1 ? "" : "s"}`,
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
export function capabilityErrors(d: CapabilityDraft, update: boolean): Record<string, string> {
  const e: Record<string, string> = {};
  if (d.manufacturer.trim().length < 1 || d.manufacturer.trim().length > 120) e.manufacturer = "1–120 characters";
  if (d.model.trim().length < 1 || d.model.trim().length > 120) e.model = "1–120 characters";
  if ((d.modeControl || d.fanControl || d.temperatureOn) && !d.control) e.control = "Mode, fan or temperature control needs power control";
  if (d.modeControl && d.modes.length === 0) e.modes = "Choose at least one mode";
  if (d.fanControl && d.fanLevels.length === 0) e.fanLevels = "Choose at least one fan level";
  if (d.temperatureOn) {
    const [min, max, step] = [num(d.min), num(d.max), num(d.step)];
    if (!(min < max)) e.temperature = "min must be below max";
    else if (!(step > 0 && step <= max - min)) e.temperature = "0 < step ≤ max − min";
  }
  if (d.ventilation && (d.ventilationLevels.length === 0 || !d.ventilationLevels.includes("low"))) e.ventilationLevels = "Ventilation levels must include Low";
  const seen = new Set<string>();
  for (const s of d.sensors) {
    if (seen.has(s.metric)) e.sensors = `One sensor per metric (${s.metric} twice)`;
    seen.add(s.metric);
    const v = num(s.staleAfterSeconds);
    if (!(Number.isInteger(v) && v >= 10 && v <= 86400)) e.sensors = "Stale after must be 10–86400 seconds";
  }
  if (d.firmware.split(",").map((x) => x.trim()).some((x) => x.length > 32)) e.firmware = "Firmware versions are at most 32 characters";
  if (update && (d.changeReason.trim().length < 1 || d.changeReason.trim().length > 1000)) e.changeReason = "A reason is required (1–1000 characters)";
  return e;
}

/** The capabilities.save input (Save<Capability> & {changeReason}); sensor units and boundaries are derived (IR11/12). */
export function capabilityInput(d: CapabilityDraft, id?: string) {
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

export function deviceRows(devices: ApiDevice[], units: ApiUnitLite[]): DeviceRow[] {
  const name = new Map(units.map((u) => [u.id, u.displayName]));
  return devices.map((d) => ({
    id: d.id, version: d.version, serial: d.serial, unitId: d.unitId, unit: d.unitId ? name.get(d.unitId) ?? "unit" : "Unbound", conn: d.connection, fw: d.firmwareVersion,
    tamper: d.tamper === "detected", power: d.powerSignal,
  }));
}

const opLabel = { check: "Connection check", calibrate: "Calibration", firmware: "Firmware update" } as const;
export function operationItem(o: ApiDeviceOperation) {
  return {
    time: klTime(o.createdAt), title: `${opLabel[o.kind]}${o.targetVersion ? ` → ${o.targetVersion}` : ""} · ${o.status}`,
    detail: o.failureCode ? `failure ${o.failureCode}` : o.finishedAt ? `finished ${klTime(o.finishedAt)}` : "in progress", tone: o.status === "failed" ? ("crit" as const) : o.status === "succeeded" ? ("ok" as const) : undefined,
  };
}
export function calibrationItem(c: ApiCalibration) {
  return { time: klTime(c.calibratedAt), title: `${c.metric}: reference ${c.referenceValue} ${c.unit} · measured ${c.measuredValue} ${c.unit}`, detail: `offset ${(c.measuredValue - c.referenceValue).toFixed(2)} ${c.unit} · demo` };
}

export type CampaignRow = { id: string; version: number; model: string; versions: string; state: ApiCampaign["state"]; devices: number; start: string; c: ApiCampaign };
export function campaignRows(cs: ApiCampaign[], caps: ApiCapability[]): CampaignRow[] {
  const model = new Map(caps.map((k) => [k.id, `${k.manufacturer} ${k.model}`]));
  return cs.map((c) => ({
    id: c.id, version: c.version, model: model.get(c.modelId) ?? "model", versions: `${c.fromVersions.join(", ") || "any"} → ${c.targetVersion}`, state: c.state,
    devices: c.deviceIds.length, start: klTime(c.startAt), c,
  }));
}

/** The firmware campaign form (DD-A20): waves ascend and end at 100 %, auto-pause 1–50 %, start ≥ 24 h ahead. */
export type CampaignDraft = { modelId: string; targetVersion: string; deviceIds: string[]; waves: string; startLocal: string; endLocal: string; autoPause: string; startAt: string };
export function parseWaves(s: string): { label: string; percent: number }[] | null {
  const ps = s.split(",").map((x) => Number(x.replace("%", "").trim()));
  if (ps.length === 0 || ps.some((p) => !Number.isInteger(p) || p < 1 || p > 100)) return null;
  for (let i = 1; i < ps.length; i++) if (ps[i] <= ps[i - 1]) return null;
  if (ps[ps.length - 1] !== 100) return null;
  return ps.map((p, i) => ({ label: i === ps.length - 1 ? `Wave ${i + 1} · 100 %` : `Wave ${i + 1} · ${p} %`, percent: p }));
}
export function campaignErrors(d: CampaignDraft, now: Date): Record<string, string> {
  const e: Record<string, string> = {};
  if (!d.modelId) e.modelId = "Choose a model";
  if (!d.targetVersion) e.targetVersion = "Choose a target version";
  if (d.deviceIds.length === 0) e.deviceIds = "Choose at least one device";
  if (!parseWaves(d.waves)) e.waves = "Ascending percentages ending at 100, e.g. 10, 50, 100";
  const ap = Number(d.autoPause);
  if (!(Number.isInteger(ap) && ap >= 1 && ap <= 50)) e.autoPause = "1–50 %";
  if (!/^\d{2}:\d{2}$/.test(d.startLocal) || !/^\d{2}:\d{2}$/.test(d.endLocal)) e.window = "HH:MM – HH:MM";
  const at = Date.parse(`${d.startAt}:00+08:00`);
  if (!d.startAt || !(at >= now.getTime() + 24 * 3600 * 1000)) e.startAt = "At least 24 hours ahead";
  return e;
}
