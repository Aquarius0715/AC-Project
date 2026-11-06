// Technician IoT devices (FR-T11, FR-T12, DD-T11, DD-T12, Figma Technician 03-x, DATA_SOURCE=API): the devices of the
// technician's assigned units, the job each device write runs under (IR94), the detail tiles, the firmware operation
// card and the device event rows with their alerts and response notes. Pure code shared by the Server Components and
// the client view; Vitest covers it. Texts in the display language (`i` / `t`, IR287); instants in the user's display
// time zone (IR44). Event types, evidence sources and versions stay codes, as in Figma.
import { EN, relativeTime, showClock, showTime, translator, type I18n, type T, textOf } from "@ac/web/lib/i18n";
import type { ApiCalibration, ApiDevice, ApiDeviceDetail, ApiDeviceOperation, Connection } from "@ac/web/lib/devices";
import { metricLabel } from "@ac/web/lib/adminAlerts";
import { lookup } from "@ac/web/lib/lookup";

// the device screens' colours (technician and HQ): the connection filter's dot (unknown warns), the firmware card and an
// operation's tone (anything else is still under way)
const CONN_DOT = new Map([["online", "bg-ok"], ["offline", "bg-crit"]]);
const CARD_LOOK = new Map([["crit", "border-crit bg-crit-soft/40"], ["ok", "border-[#86efac] bg-ok-soft/40"]]);
const OPERATION_TONE = new Map<string, "ok" | "crit">([["succeeded", "ok"], ["failed", "crit"]]);
export const connectionDot = (filter: string): string => CONN_DOT.get(filter) ?? "bg-warn";
export const firmwareCardLook = (tone: string): string => CARD_LOOK.get(tone) ?? "border-primary bg-primary-soft/40";
export const operationTone = (status: string): "ok" | "crit" | "primary" => OPERATION_TONE.get(status) ?? "primary";
/** The firmware action's line: running, the newer versions on offer, or none. */
export function firmwareSub(isRunning: boolean, versions: string[], t: T): string {
  if (isRunning) return t("running…");
  return versions.length > 0 ? t("{version} available", { version: versions.join(", ") }) : t("no newer candidate");
}

const en = translator("en");

/** DeviceEvent of service-contracts.ts. */
export interface ApiDeviceEventFull {
  id: string; version: number; deviceId: string; eventType: "communication_lost" | "power_lost" | "tamper" | "restored" | "operation_failed";
  evidenceSource: "heartbeat" | "power_signal" | "tamper_signal"; occurredAt: string; restoredAt: string | null; alertIds: string[];
  recovery: { axis: "connection" | "power" | "tamper"; sourceEventId: string; value: string } | null; responseNotes: { actorId: string; message: string; at: string }[];
}
/** The alert fields the event rows need (alerts.get). */
export interface ApiAlertLite { id: string; version: number; status: "open" | "acknowledged" | "resolved"; type: string; severity: string }
/** A technician's job on a unit (jobs.list summary projection). */
export interface TechJob { id: string; unitId: string; status: string }

const rank: Record<string, number> = { in_progress: 0, assigned: 1, rework_requested: 2 };
/** The job a device write on the unit runs under (IR94): in progress first, else assigned or rework; null without one. */
export function jobFor(unitId: string | null, jobs: TechJob[]): string | null {
  return unitId ? jobs.filter((j) => j.unitId === unitId && j.status in rank).toSorted((a, b) => (lookup(rank, a.status) ?? 9) - (lookup(rank, b.status) ?? 9))[0]?.id ?? null : null;
}

const ago = (iso: string | null, now: number, t: T) => {
  if (!iso) return t("never seen");
  const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
  if (s < 120) return t("{n} s ago", { n: s });
  if (s < 7200) return t("{n} min ago", { n: Math.round(s / 60) });
  // past two days in days, not "222 h ago" (IR613)
  return s < 172_800 ? t("{n} h ago", { n: Math.round(s / 3600) }) : t("{n} d ago", { n: Math.round(s / 86_400) });
};
/** One device of the list (Figma Technician 03, Admin 246:2): serial, connection, unit, firmware and its sensor count. */
/** `unit` names the bound unit; a device not bound yet says so with its planned unit (D05, IR467). */
export interface TechDeviceRow { id: string; serial: string; unitId: string | null; targetUnitId: string | null; unit: string; conn: Connection; connText: string | null; fw: string; tamper: boolean; sensors: number }
/** The unit a device serves: the bound unit, the planned one when it is not bound yet, or none. */
function deviceUnit(d: ApiDevice, unitName: Map<string, string>, t: T): string {
  if (d.unitId) return unitName.get(d.unitId) ?? t("unit");
  return d.targetUnitId ? t("Not bound yet — planned for {unit}", { unit: unitName.get(d.targetUnitId) ?? t("unit") }) : t("Unbound");
}
export function techDeviceRows(ds: ApiDevice[], unitName: Map<string, string>, now: number, t: T = en): TechDeviceRow[] {
  return [...ds].toSorted((a, b) => a.serial.localeCompare(b.serial)).map((d) => {
    return {
      id: d.id, serial: d.serial, unitId: d.unitId, targetUnitId: d.targetUnitId ?? null, conn: d.connection,
      unit: deviceUnit(d, unitName, t),
      connText: d.connection !== "online" && d.lastSeenAt ? ago(d.lastSeenAt, now, t) : null, fw: d.firmwareVersion, tamper: d.tamper === "detected", sensors: d.sensors.length,
    };
  });
}
export type Filter = "all" | "online" | "offline" | "tamper" | "nosensors";
export const FILTERS: { id: Filter; label: string }[] = [{ id: "all", label: "All" }, { id: "online", label: "Online" }, { id: "offline", label: "Offline" }, { id: "tamper", label: "Tamper" }];
/** HQ also lists the devices without sensors (Figma Admin 246:2). */
export const HQ_FILTERS: { id: Filter; label: string }[] = [...FILTERS, { id: "nosensors", label: "No sensors" }];
export const filterOf = (f: Filter) => (r: TechDeviceRow): boolean => {
  switch (f) {
    case "all": { return true; }
    case "tamper": { return r.tamper; }
    case "nosensors": { return r.sensors === 0; }
    case "online": { return r.conn === "online"; }
    case "offline": { return r.conn !== "online"; }
  }
};

/** The device a state filter opens (IR616): when the open device is one the filter hides, the filter's first device —
so the detail never shows a device the list no longer has; null keeps the open one (or when the filter shows none). */
export function followFilter(rows: TechDeviceRow[], f: Filter, openId: string | null): string | null {
  const kept = rows.filter(filterOf(f));
  return openId && kept.every((r) => r.id !== openId) && kept[0] ? kept[0].id : null;
}

/** When the open fault of each axis was detected (the newest fault of the axis while it has no restoredAt, T12 events). */
export type OpenFaults = Partial<Record<"connection" | "power" | "tamper", string>>;
const faultAxis: Partial<Record<ApiDeviceEventFull["eventType"], keyof OpenFaults>> = { communication_lost: "connection", power_lost: "power", tamper: "tamper" };
export function openFaults(es: ApiDeviceEventFull[]): OpenFaults {
  const out: OpenFaults = {};
  const seen = new Set<string>();
  const newestFirst = [...es].toSorted((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt));
  for (const e of newestFirst) {
    const k = faultAxis[e.eventType];
    if (!k || seen.has(k)) continue;
    seen.add(k);
    if (!e.restoredAt) Object.assign(out, { [k]: e.occurredAt });
  }
  return out;
}

const connDot: Partial<Record<Connection, string>> = { online: "ok", connecting: "primary", unknown: "muted", offline: "crit", error: "crit" };
const CONNECTION: Partial<Record<Connection, string>> = { online: "Online", connecting: "Connecting", unknown: "Unknown", offline: "Offline", error: "Error" };
/** The four tiles (Connection, Power signal, Tamper, Firmware) with their evidence lines; with the T12 events a fault
line shows when it was detected (Figma 03 “lost 08:12”). */
// the power signal tile: what it reads and its dot
const UNKNOWN_POWER = { text: "Unknown", dot: "muted" } as const;
const POWER = new Map<string, { text: string; dot: "crit" | "primary" | "muted" }>([["on", { text: "ON", dot: "primary" }], ["off", { text: "Lost", dot: "crit" }]]);
export function deviceTiles(d: ApiDeviceDetail, newer: string | null, now: number, faults: OpenFaults = {}, i: I18n = EN) {
  const { t, display } = i;
  const clock = (iso: string) => showClock(iso, display);
  const conn = d.connection;
  const lost = conn !== "online" && faults.connection ? t("lost {time}", { time: clock(faults.connection) }) : t("last seen {time}", { time: ago(d.lastSeenAt, now, t) });
  const power = POWER.get(d.powerSignal) ?? UNKNOWN_POWER;
  const opened = faults.tamper ? t("cover opened {time}", { time: clock(faults.tamper) }) : t("cover opened — see device events");
  return [
    { label: t("Connection"), value: t(lookup(CONNECTION, conn) ?? conn), dot: lookup(connDot, conn) ?? "muted",
      sub: conn === "connecting" ? t("check running — waiting for the device") : lost },
    { label: t("Power signal"), value: t(power.text), dot: power.dot,
      sub: d.powerSignal === "off" && faults.power ? `power_signal ${clock(faults.power)}` : t("from the device's dedicated power signal") },
    { label: t("Tamper"), value: t(d.tamper === "detected" ? "Detected" : "tamper::Clear"), dot: d.tamper === "detected" ? "warn" : "ok",
      sub: d.tamper === "detected" ? opened : t("no removal detected"), warn: d.tamper === "detected" },
    { label: t("Firmware"), value: d.firmwareVersion, dot: null, sub: newer ? t("{version} available", { version: newer }) : t("latest") },
  ];
}
const byVersion = (a: string, b: string) => a.localeCompare(b, "en", { numeric: true });
/** The firmware candidates above the installed version, oldest first (capability firmwareCandidates; v2 < v10). A
technician only updates forward; HQ campaigns handle anything else. */
export const newerVersions = (current: string, candidates: string[]) => [...new Set(candidates)].filter((v) => byVersion(v, current) > 0).toSorted(byVersion);
/** The newest of them, null when the device runs the latest. */
export const newerFirmware = (current: string, candidates: string[]) => newerVersions(current, candidates).pop() ?? null;

/** The firmware operation card (Figma 03 “Firmware update v1 → v2”): the active update, else the last finished one. */
export function firmwareCard(d: ApiDeviceDetail, ops: (ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string })[], i: I18n = EN) {
  const { t, display } = i;
  const active = d.activeOperation?.kind === "firmware" ? (d.activeOperation as ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string }) : null;
  const last = active ?? ops.find((o) => o.kind === "firmware") ?? null;
  if (!last) return null;
  if (last.status === "queued" || last.status === "running") {
    const started = t("Started {time} · control commands locked", { time: showClock(last.startedAt ?? last.createdAt, display) });
    return { tone: "primary" as const, title: t("Firmware update {from} → {to}", { from: d.firmwareVersion, to: last.targetVersion ?? "—" }), status: t(last.status === "queued" ? "Queued" : "Running"),
      text: last.expiresAt ? `${started} · ${t("fails if the device does not confirm by {time}", { time: showClock(last.expiresAt, display) })}` : started };
  }
  if (last.status === "failed") return { tone: "crit" as const, title: t("Firmware update to {version} failed", { version: last.targetVersion ?? "—" }), status: t("Failed"), text: `${failureText(last.failureCode, t)} · ${t("the old version {version} is kept", { version: d.firmwareVersion })}` };
  return { tone: "ok" as const, title: t("Firmware {version} installed", { version: last.targetVersion ?? "—" }), status: t("Succeeded"), text: t("finished {time} · history appended", { time: last.finishedAt ? showTime(last.finishedAt, display) : "—" }) };
}

const FAILURE: Record<string, string> = {
  TIMEOUT: "no confirmation within 60 s", OFFLINE: "the device was not online at the start", CONFLICT: "a command or another operation was running",
  UNAVAILABLE: "the device reported a failure",
};
/** Why a device operation failed (IR67, REV19-018). */
export const failureText = (code: string | null | undefined, t: T = en) => textOf(FAILURE, code, t) ?? code ?? t("no result");

const KIND: Partial<Record<ApiDeviceOperation["kind"], string>> = { check: "Connection check", calibrate: "Calibration", firmware: "Firmware update" };
const STATUS: Partial<Record<ApiDeviceOperation["status"], string>> = { queued: "Queued", running: "Running", succeeded: "Succeeded", failed: "Failed" };
/** An operation in words, as “Connection check · Queued”. */
export const operationLabel = (o: Pick<ApiDeviceOperation, "kind" | "status">, t: T = en) => `${t(lookup(KIND, o.kind) ?? o.kind)} · ${t(lookup(STATUS, o.status) ?? o.status)}`;
/** One operation of the T11 history (Figma 03: started, “v1 → v2”, “heartbeat confirmed”). */
export interface OperationRow { id: string; kind: string; status: ApiDeviceOperation["status"]; statusText: string; time: string; detail: string }
/** What an operation did: its failure, the firmware it installs or installed, or the connection check's heartbeat. */
function operationDetail(o: ApiDeviceOperation, isOpen: boolean, current: string, t: T): string {
  if (o.status === "failed") return failureText(o.failureCode, t);
  if (o.kind === "firmware") return isOpen ? `${current} → ${o.targetVersion ?? "—"}` : t("{version} installed", { version: o.targetVersion ?? "—" });
  if (o.status === "succeeded") return t("heartbeat confirmed");
  return t(o.status === "running" ? "waiting for the heartbeat" : "starts within a second");
}
export function operationRows(ops: (ApiDeviceOperation & { startedAt?: string | null })[], current: string, i: I18n = EN): OperationRow[] {
  const { t, display } = i;
  return ops.map((o) => {
    const isOpen = o.status === "queued" || o.status === "running";
    const detail = operationDetail(o, isOpen, current, t);
    return { id: o.id, kind: t(lookup(KIND, o.kind) ?? o.kind), status: o.status, statusText: t(lookup(STATUS, o.status) ?? o.status), time: showTime(o.startedAt ?? o.createdAt, display), detail };
  });
}
const signed = (n: number) => (n < 0 ? `−${Math.abs(n).toFixed(1)}` : n.toFixed(1));
/** One calibration of the T11 history: reference, measured and the offset to apply (reference − measured). */
export const calibrationText = (c: ApiCalibration, i: I18n = EN) => {
  return i.t("{metric} · ref {reference} / measured {measured} · offset {offset} · {time} — appended; earlier readings unchanged.", {
    metric: metricText(c.metric, i.t), reference: `${c.referenceValue.toFixed(1)} ${c.unit}`, measured: `${c.measuredValue.toFixed(1)} ${c.unit}`, offset: `${signed(c.referenceValue - c.measuredValue)} ${c.unit}`, time: showTime(c.calibratedAt, i.display),
  });
};
/** A sensor's metric in words ("temperature" → "Temperature", in the display language); an unknown one as stored (IR499). */
export const metricText = (metric: string, t: T) => (textOf(metricLabel, metric, t) ?? metric);
/** The sensors of the device for the T11 table: metric, unit, stale limit and the last calibration. */
export const sensorRows = (d: ApiDevice, i: I18n = EN) =>
  d.sensors.map((s) => ({ id: s.id, metric: metricText(s.metric, i.t), unit: s.unit, stale: i.t("{n} s", { n: s.staleAfterSeconds }), calibrated: s.calibratedAt ? showTime(s.calibratedAt, i.display) : i.t("Never") }));

const eventDetail: Record<ApiDeviceEventFull["eventType"], string> = {
  tamper: "Cover opened while powered", power_lost: "Dedicated demo power signal lost", communication_lost: "Missed heartbeats (not proof of power loss)",
  restored: "Connection restored", operation_failed: "Device operation failed",
};
const restoredDetail: Record<NonNullable<ApiDeviceEventFull["recovery"]>["axis"], string> = {
  connection: "Connection restored", power: "Power signal restored", tamper: "Tamper cleared — cover closed (the alert stays until handled)",
};
/** What a device event means in words: the fault, or for a recovery the axis that came back. */
export const eventCause = (e: Pick<ApiDeviceEventFull, "eventType" | "recovery">, t: T = en) => t(e.recovery ? restoredDetail[e.recovery.axis] : eventDetail[e.eventType]);
/** One device event row (Figma 03 “Device events”): detection, evidence, its alerts and the recovery or response. */
export interface EventRow { id: string; version: number; time: string; type: ApiDeviceEventFull["eventType"]; tone: "warn" | "crit" | "ok" | "muted"; detail: string; recovery: string; notes: string[]; alerts: ApiAlertLite[] }
const EVENT_TONE: Partial<Record<ApiDeviceEventFull["eventType"], EventRow["tone"]>> = { tamper: "warn", restored: "ok", operation_failed: "muted" };
const alertCount = (n: number, t: T) => {
  if (n === 0) return "";
  return ` · ${n === 1 ? t("1 alert") : t("{n} alerts", { n })}`;
};
/** Where an event stands: restored, a recovery of an earlier fault, or open with its alerts' state. */
function eventRecovery(e: ApiDeviceEventFull, source: ApiDeviceEventFull | undefined, as: ApiAlertLite[], open: number, when: (iso: string) => string, t: T): string {
  if (e.restoredAt) return t("restored {time}", { time: when(e.restoredAt) });
  if (e.eventType === "restored") return source ? t("recovers {event} {time}", { event: source.eventType, time: when(source.occurredAt) }) : t("recovers an earlier fault");
  if (as.length === 0) return t("open");
  if (open) return t("open · unacknowledged");
  return t(as.every((a) => a.status === "resolved") ? "open · resolved" : "open · acknowledged");
}
export function eventRows(es: ApiDeviceEventFull[], alerts: Map<string, ApiAlertLite>, now: number, i: I18n = EN): EventRow[] {
  const { t } = i;
  const when = (iso: string) => relativeTime(iso, now, i);
  const byId = new Map(es.map((e) => [e.id, e]));
  return es.map((e) => {
    const as = e.alertIds.map((id) => alerts.get(id)).filter((a): a is ApiAlertLite => !!a);
    const open = as.filter((a) => a.status === "open").length;
    const source = e.recovery ? byId.get(e.recovery.sourceEventId) : undefined;
    const recovery = eventRecovery(e, source, as, open, when, t);
    return {
      id: e.id,
      version: e.version,
      time: when(e.occurredAt),
      type: e.eventType,
      tone: lookup(EVENT_TONE, e.eventType) ?? "crit",
      detail: `${eventCause(e, t)} · ${e.evidenceSource}${alertCount(e.alertIds.length, t)}`,
      notes: e.responseNotes.map((n) => `${when(n.at)} — ${n.message}`),
      alerts: as,
      recovery,
    };
  });
}
/** The open alerts of the events, once each (acknowledged with alerts.acknowledge and the alert's version, SR23). */
export const openAlerts = (rows: EventRow[]) => [...new Map(rows.flatMap((r) => r.alerts).filter((a) => a.status === "open").map((a) => [a.id, a])).values()];

const REFUSED: Record<string, string> = {
  OFFLINE: "The device is not reachable, so nothing was started (an update needs an online device).",
  CONFLICT: "Another operation is running or the device changed; the page shows the latest state.",
  FORBIDDEN: "Outside your assignment or work window, or your role has no device permission.",
};
/** A refused device write in words (null for a VALIDATION that only marks fields). */
export function deviceRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): string | null {
  const duplicate = f.messageKey === "errors.duplicate_serial" || (f.code === "CONFLICT" && f.fieldErrors.serial);
  return duplicate ? t("This serial is already registered.") : textOf(REFUSED, f.code, t) ?? null;
}
const FIELD: Record<string, string> = { "error.required": "Required", "error.length": "1–1000 characters", "error.invalid": "Not valid", "error.duplicate": "Already registered" };
/** A field error of the Core API in words. */
export const deviceFieldText = (key: string, t: T = en) => (textOf(FIELD, key, t) ?? key.replace(/^errors?\./, "").replaceAll('_', " "));
