// Technician IoT devices (FR-T11, FR-T12, DD-T11, DD-T12, Figma Technician 03-x, DATA_SOURCE=api): the devices of the
// technician's assigned units, the job each device write runs under (IR94), the detail tiles, the firmware operation
// card and the device event rows with their alerts and response notes. Pure code shared by the Server Components and
// the client view; Vitest covers it. Texts in the display language (`i` / `t`, IR287); instants in the user's display
// time zone (IR44). Event types, evidence sources and versions stay codes, as in Figma.
import { EN, relativeTime, showClock, showTime, translator, type I18n, type T } from "@ac/web/lib/i18n";
import type { ApiCalibration, ApiDevice, ApiDeviceDetail, ApiDeviceOperation, Connection } from "@ac/web/lib/devices";

const en = translator("en");

/** DeviceEvent of service-contracts.ts. */
export type ApiDeviceEventFull = {
  id: string; version: number; deviceId: string; eventType: "communication_lost" | "power_lost" | "tamper" | "restored" | "operation_failed";
  evidenceSource: "heartbeat" | "power_signal" | "tamper_signal"; occurredAt: string; restoredAt: string | null; alertIds: string[];
  recovery: { axis: "connection" | "power" | "tamper"; sourceEventId: string; value: string } | null; responseNotes: { actorId: string; message: string; at: string }[];
};
/** The alert fields the event rows need (alerts.get). */
export type ApiAlertLite = { id: string; version: number; status: "open" | "acknowledged" | "resolved"; type: string; severity: string };
/** A technician's job on a unit (jobs.list summary projection). */
export type TechJob = { id: string; unitId: string; status: string };

const rank: Record<string, number> = { in_progress: 0, assigned: 1, rework_requested: 2 };
/** The job a device write on the unit runs under (IR94): in progress first, else assigned or rework; null without one. */
export function jobFor(unitId: string | null, jobs: TechJob[]): string | null {
  if (!unitId) return null;
  return jobs.filter((j) => j.unitId === unitId && j.status in rank).sort((a, b) => rank[a.status] - rank[b.status])[0]?.id ?? null;
}

const ago = (iso: string | null, now: number, t: T) => {
  if (!iso) return t("never seen");
  const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
  return s < 120 ? t("{n} s ago", { n: s }) : s < 7200 ? t("{n} min ago", { n: Math.round(s / 60) }) : t("{n} h ago", { n: Math.round(s / 3600) });
};
/** One device of the list (Figma Technician 03, Admin 246:2): serial, connection, unit, firmware and its sensor count. */
export type TechDeviceRow = { id: string; serial: string; unitId: string | null; unit: string; conn: Connection; connText: string | null; fw: string; tamper: boolean; sensors: number };
export function techDeviceRows(ds: ApiDevice[], unitName: Map<string, string>, now: number, t: T = en): TechDeviceRow[] {
  return [...ds].sort((a, b) => a.serial.localeCompare(b.serial)).map((d) => ({
    id: d.id, serial: d.serial, unitId: d.unitId, unit: d.unitId ? unitName.get(d.unitId) ?? t("unit") : t("Unbound"), conn: d.connection,
    connText: d.connection === "online" ? null : d.lastSeenAt ? ago(d.lastSeenAt, now, t) : null, fw: d.firmwareVersion, tamper: d.tamper === "detected", sensors: d.sensors.length,
  }));
}
export type Filter = "all" | "online" | "offline" | "tamper" | "nosensors";
export const FILTERS: { id: Filter; label: string }[] = [{ id: "all", label: "All" }, { id: "online", label: "Online" }, { id: "offline", label: "Offline" }, { id: "tamper", label: "Tamper" }];
/** HQ also lists the devices without sensors (Figma Admin 246:2). */
export const HQ_FILTERS: { id: Filter; label: string }[] = [...FILTERS, { id: "nosensors", label: "No sensors" }];
export const filterOf = (f: Filter) => (r: TechDeviceRow) =>
  f === "all" || (f === "tamper" ? r.tamper : f === "nosensors" ? r.sensors === 0 : f === "online" ? r.conn === "online" : r.conn !== "online");

/** When the open fault of each axis was detected (the newest fault of the axis while it has no restoredAt, T12 events). */
export type OpenFaults = Partial<Record<"connection" | "power" | "tamper", string>>;
const faultAxis: Partial<Record<ApiDeviceEventFull["eventType"], keyof OpenFaults>> = { communication_lost: "connection", power_lost: "power", tamper: "tamper" };
export function openFaults(es: ApiDeviceEventFull[]): OpenFaults {
  const out: OpenFaults = {};
  const seen = new Set<string>();
  for (const e of [...es].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))) {
    const k = faultAxis[e.eventType];
    if (!k || seen.has(k)) continue;
    seen.add(k);
    if (!e.restoredAt) out[k] = e.occurredAt;
  }
  return out;
}

const connDot: Record<Connection, string> = { online: "ok", connecting: "primary", unknown: "muted", offline: "crit", error: "crit" };
const CONNECTION: Record<Connection, string> = { online: "Online", connecting: "Connecting", unknown: "Unknown", offline: "Offline", error: "Error" };
/** The four tiles (Connection, Power signal, Tamper, Firmware) with their evidence lines; with the T12 events a fault
 * line shows when it was detected (Figma 03 “lost 08:12”). */
export function deviceTiles(d: ApiDeviceDetail, newer: string | null, now: number, faults: OpenFaults = {}, i: I18n = EN) {
  const { t, display } = i;
  const clock = (iso: string) => showClock(iso, display);
  const conn = d.connection;
  return [
    { label: t("Connection"), value: t(CONNECTION[conn] ?? conn), dot: connDot[conn] ?? "muted",
      sub: conn === "connecting" ? t("check running — waiting for the device") : conn !== "online" && faults.connection ? t("lost {time}", { time: clock(faults.connection) }) : t("last seen {time}", { time: ago(d.lastSeenAt, now, t) }) },
    { label: t("Power signal"), value: d.powerSignal === "on" ? t("ON") : d.powerSignal === "off" ? t("Lost") : t("Unknown"), dot: d.powerSignal === "off" ? "crit" : d.powerSignal === "on" ? "primary" : "muted",
      sub: d.powerSignal === "off" && faults.power ? `power_signal ${clock(faults.power)}` : t("from the device's dedicated power signal") },
    { label: t("Tamper"), value: d.tamper === "detected" ? t("Detected") : t("tamper::Clear"), dot: d.tamper === "detected" ? "warn" : "ok",
      sub: d.tamper !== "detected" ? t("no removal detected") : faults.tamper ? t("cover opened {time}", { time: clock(faults.tamper) }) : t("cover opened — see device events"), warn: d.tamper === "detected" },
    { label: t("Firmware"), value: d.firmwareVersion, dot: null, sub: newer ? t("{version} available", { version: newer }) : t("latest") },
  ];
}
const byVersion = (a: string, b: string) => a.localeCompare(b, "en", { numeric: true });
/** The firmware candidates above the installed version, oldest first (capability firmwareCandidates; v2 < v10). A
 * technician only updates forward; HQ campaigns handle anything else. */
export const newerVersions = (current: string, candidates: string[]) => [...new Set(candidates)].filter((v) => byVersion(v, current) > 0).sort(byVersion);
/** The newest of them, null when the device runs the latest. */
export const newerFirmware = (current: string, candidates: string[]) => newerVersions(current, candidates).pop() ?? null;

/** The firmware operation card (Figma 03 “Firmware update v1 → v2”): the active update, else the last finished one. */
export function firmwareCard(d: ApiDeviceDetail, ops: (ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string })[], i: I18n = EN) {
  const { t, display } = i;
  const active = d.activeOperation?.kind === "firmware" ? (d.activeOperation as ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string }) : null;
  const last = active ?? ops.find((o) => o.kind === "firmware") ?? null;
  if (!last) return null;
  if (last.status === "queued" || last.status === "running") {
    const started = t("Started {time} · control commands locked (D05)", { time: showClock(last.startedAt ?? last.createdAt, display) });
    return { tone: "primary" as const, title: t("Firmware update {from} → {to}", { from: d.firmwareVersion, to: last.targetVersion ?? "—" }), status: t(last.status === "queued" ? "Queued" : "Running"),
      text: last.expiresAt ? `${started} · ${t("fails if the device does not confirm by {time}", { time: showClock(last.expiresAt, display) })}` : started };
  }
  if (last.status === "failed") return { tone: "crit" as const, title: t("Firmware update to {version} failed", { version: last.targetVersion ?? "—" }), status: t("Failed"), text: `${failureText(last.failureCode, t)} · ${t("the old version {version} is kept", { version: d.firmwareVersion })}` };
  return { tone: "ok" as const, title: t("Firmware {version} installed", { version: last.targetVersion ?? "—" }), status: t("Succeeded"), text: t("finished {time} · history appended", { time: last.finishedAt ? showTime(last.finishedAt, display) : "—" }) };
}

const FAILURE: Record<string, string> = {
  TIMEOUT: "TIMEOUT — no confirmation within 60 s", OFFLINE: "OFFLINE — the device was not online at the start", CONFLICT: "CONFLICT — a command or another operation was running",
  UNAVAILABLE: "UNAVAILABLE — the device reported a failure",
};
/** Why a device operation failed (IR67, REV19-018). */
export const failureText = (code: string | null | undefined, t: T = en) => (code && FAILURE[code] ? t(FAILURE[code]) : code ?? t("no result"));

const KIND: Record<ApiDeviceOperation["kind"], string> = { check: "Connection check", calibrate: "Calibration", firmware: "Firmware update" };
const STATUS: Record<ApiDeviceOperation["status"], string> = { queued: "Queued", running: "Running", succeeded: "Succeeded", failed: "Failed" };
/** One operation of the T11 history (Figma 03: started, “v1 → v2”, “heartbeat confirmed”). */
export type OperationRow = { id: string; kind: string; status: ApiDeviceOperation["status"]; statusText: string; time: string; detail: string };
export function operationRows(ops: (ApiDeviceOperation & { startedAt?: string | null })[], current: string, i: I18n = EN): OperationRow[] {
  const { t, display } = i;
  return ops.map((o) => {
    const open = o.status === "queued" || o.status === "running";
    const detail = o.status === "failed" ? failureText(o.failureCode, t)
      : o.kind === "firmware" ? (open ? `${current} → ${o.targetVersion}` : t("→ {version} installed", { version: o.targetVersion ?? "—" }))
      : o.status === "succeeded" ? t("heartbeat confirmed") : o.status === "running" ? t("waiting for the heartbeat") : t("starts within a second");
    return { id: o.id, kind: t(KIND[o.kind] ?? o.kind), status: o.status, statusText: t(STATUS[o.status] ?? o.status), time: showTime(o.startedAt ?? o.createdAt, display), detail };
  });
}
const signed = (n: number) => (n < 0 ? `−${Math.abs(n).toFixed(1)}` : n.toFixed(1));
/** One calibration of the T11 history: reference, measured and the offset to apply (reference − measured). */
export const calibrationText = (c: ApiCalibration, i: I18n = EN) =>
  i.t("{metric} · ref {reference} / measured {measured} · offset {offset} · {time} — appended; earlier readings unchanged.", {
    metric: c.metric, reference: `${c.referenceValue.toFixed(1)} ${c.unit}`, measured: `${c.measuredValue.toFixed(1)} ${c.unit}`, offset: `${signed(c.referenceValue - c.measuredValue)} ${c.unit}`, time: showTime(c.calibratedAt, i.display),
  });
/** The sensors of the device for the T11 table: metric, unit, stale limit and the last calibration. */
export const sensorRows = (d: ApiDevice, i: I18n = EN) =>
  d.sensors.map((s) => ({ id: s.id, metric: s.metric, unit: s.unit, stale: i.t("{n} s", { n: s.staleAfterSeconds }), calibrated: s.calibratedAt ? showTime(s.calibratedAt, i.display) : i.t("Never") }));

const eventDetail: Record<ApiDeviceEventFull["eventType"], string> = {
  tamper: "Cover opened while powered", power_lost: "Dedicated demo power signal lost", communication_lost: "Missed heartbeats (not proof of power loss)",
  restored: "Connection restored", operation_failed: "Device operation failed",
};
const restoredDetail: Record<NonNullable<ApiDeviceEventFull["recovery"]>["axis"], string> = {
  connection: "Connection restored", power: "Power signal restored", tamper: "Tamper cleared — cover closed (the alert stays until handled)",
};
/** One device event row (Figma 03 “Device events”): detection, evidence, its alerts and the recovery or response. */
export type EventRow = { id: string; version: number; time: string; type: ApiDeviceEventFull["eventType"]; tone: "warn" | "crit" | "ok" | "muted"; detail: string; recovery: string; notes: string[]; alerts: ApiAlertLite[] };
export function eventRows(es: ApiDeviceEventFull[], alerts: Map<string, ApiAlertLite>, now: number, i: I18n = EN): EventRow[] {
  const { t } = i;
  const when = (iso: string) => relativeTime(iso, now, i);
  const byId = new Map(es.map((e) => [e.id, e]));
  return es.map((e) => {
    const as = e.alertIds.map((id) => alerts.get(id)).filter((a): a is ApiAlertLite => !!a);
    const open = as.filter((a) => a.status === "open").length;
    const src = e.recovery ? byId.get(e.recovery.sourceEventId) : undefined;
    const recovery = e.restoredAt ? t("restored {time}", { time: when(e.restoredAt) })
      : e.eventType === "restored" ? (src ? t("recovers {event} {time}", { event: src.eventType, time: when(src.occurredAt) }) : t("recovers an earlier fault"))
      : as.length ? t(open ? "open · unacknowledged" : as.every((a) => a.status === "resolved") ? "open · resolved" : "open · acknowledged") : t("open");
    return {
      id: e.id, version: e.version, time: when(e.occurredAt), type: e.eventType, tone: e.eventType === "tamper" ? "warn" : e.eventType === "restored" ? "ok" : e.eventType === "operation_failed" ? "muted" : "crit",
      detail: `${t(e.recovery ? restoredDetail[e.recovery.axis] : eventDetail[e.eventType])} · ${e.evidenceSource}${e.alertIds.length ? ` · ${t(e.alertIds.length === 1 ? "1 alert" : "{n} alerts", { n: e.alertIds.length })}` : ""}`,
      recovery, notes: e.responseNotes.map((n) => `${when(n.at)} — ${n.message}`), alerts: as,
    };
  });
}
/** The open alerts of the events, once each (acknowledged with alerts.acknowledge and the alert's version, SR23). */
export const openAlerts = (rows: EventRow[]) => [...new Map(rows.flatMap((r) => r.alerts).filter((a) => a.status === "open").map((a) => [a.id, a])).values()];

const REFUSED: Record<string, string> = {
  OFFLINE: "OFFLINE — the device is not reachable; nothing was started (an update needs an online device).",
  CONFLICT: "CONFLICT — another operation is running or the device changed; the page shows the latest state.",
  FORBIDDEN: "FORBIDDEN — outside your assignment or work window, or without device permission.",
};
/** A refused device write in words (null for a VALIDATION that only marks fields). */
export function deviceRefusal(f: { code: string; messageKey: string; fieldErrors: Record<string, string> }, t: T = en): string | null {
  if (f.messageKey === "errors.duplicate_serial" || (f.code === "CONFLICT" && f.fieldErrors.serial)) return t("CONFLICT — this serial is already registered.");
  return REFUSED[f.code] ? t(REFUSED[f.code]) : null;
}
const FIELD: Record<string, string> = { "error.required": "Required", "error.length": "1–1000 characters", "error.invalid": "Not valid", "error.duplicate": "Already registered" };
/** A field error of the Core API in words. */
export const deviceFieldText = (key: string, t: T = en) => (FIELD[key] ? t(FIELD[key]) : key.replace(/^errors?\./, "").replace(/_/g, " "));
