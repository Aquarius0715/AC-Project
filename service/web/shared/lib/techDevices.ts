// Technician IoT devices (FR-T11, FR-T12, DATA_SOURCE=api): the devices of the technician's assigned units, the job each
// device write runs under (IR94), the detail tiles, the firmware operation card and the device event rows with their
// alerts and response notes. Pure code shared by the Server Components and the client view.
import { klTime, type ApiCalibration, type ApiDevice, type ApiDeviceDetail, type ApiDeviceOperation, type Connection } from "@ac/web/lib/devices";

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

const ago = (iso: string | null, now: number) => {
  if (!iso) return "never seen";
  const s = Math.max(0, Math.round((now - Date.parse(iso)) / 1000));
  return s < 120 ? `${s} s ago` : s < 7200 ? `${Math.round(s / 60)} min ago` : `${Math.round(s / 3600)} h ago`;
};
/** One device of the list (Figma Technician 04): serial, connection, unit and firmware. */
export type TechDeviceRow = { id: string; serial: string; unitId: string | null; unit: string; conn: Connection; connText: string | null; fw: string; tamper: boolean };
export function techDeviceRows(ds: ApiDevice[], unitName: Map<string, string>, now: number): TechDeviceRow[] {
  return [...ds].sort((a, b) => a.serial.localeCompare(b.serial)).map((d) => ({
    id: d.id, serial: d.serial, unitId: d.unitId, unit: d.unitId ? unitName.get(d.unitId) ?? "unit" : "Unbound", conn: d.connection,
    connText: d.connection === "online" ? null : d.lastSeenAt ? ago(d.lastSeenAt, now) : null, fw: d.firmwareVersion, tamper: d.tamper === "detected",
  }));
}
export type Filter = "all" | "online" | "offline" | "tamper";
export const filterOf = (f: Filter) => (r: TechDeviceRow) => f === "all" || (f === "tamper" ? r.tamper : f === "online" ? r.conn === "online" : r.conn !== "online");

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

const hhmm = (iso: string) => klTime(iso).slice(11);
const connDot: Record<Connection, string> = { online: "ok", connecting: "primary", unknown: "muted", offline: "crit", error: "crit" };
/** The four tiles (Connection, Power signal, Tamper, Firmware) with their evidence lines; with the T12 events a fault
 * line shows when it was detected (Figma 04 “lost 08:12”). */
export function deviceTiles(d: ApiDeviceDetail, newer: string | null, now: number, faults: OpenFaults = {}) {
  const conn = d.connection;
  return [
    { label: "Connection", value: conn === "online" ? "Online" : conn.charAt(0).toUpperCase() + conn.slice(1), dot: connDot[conn] ?? "muted",
      sub: conn === "connecting" ? "check running — waiting for the device" : conn !== "online" && faults.connection ? `lost ${hhmm(faults.connection)}` : `last seen ${ago(d.lastSeenAt, now)}` },
    { label: "Power signal", value: d.powerSignal === "on" ? "ON" : d.powerSignal === "off" ? "Lost" : "Unknown", dot: d.powerSignal === "off" ? "crit" : d.powerSignal === "on" ? "primary" : "muted",
      sub: d.powerSignal === "off" && faults.power ? `power_signal ${hhmm(faults.power)}` : "from the device's dedicated power signal" },
    { label: "Tamper", value: d.tamper === "detected" ? "Detected" : "Clear", dot: d.tamper === "detected" ? "warn" : "ok",
      sub: d.tamper !== "detected" ? "no removal detected" : faults.tamper ? `cover opened ${hhmm(faults.tamper)}` : "cover opened — see device events", warn: d.tamper === "detected" },
    { label: "Firmware", value: d.firmwareVersion, dot: null, sub: newer ? `${newer} available` : "latest" },
  ];
}
const byVersion = (a: string, b: string) => a.localeCompare(b, "en", { numeric: true });
/** The firmware candidates above the installed version, oldest first (capability firmwareCandidates; v2 < v10). A
 * technician only updates forward; HQ campaigns handle anything else. */
export const newerVersions = (current: string, candidates: string[]) => [...new Set(candidates)].filter((v) => byVersion(v, current) > 0).sort(byVersion);
/** The newest of them, null when the device runs the latest. */
export const newerFirmware = (current: string, candidates: string[]) => newerVersions(current, candidates).pop() ?? null;

/** The firmware operation card (Figma 04 “Firmware update v1 → v2”): the active update, else the last finished one. */
export function firmwareCard(d: ApiDeviceDetail, ops: (ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string })[]) {
  const active = d.activeOperation?.kind === "firmware" ? (d.activeOperation as ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string }) : null;
  const last = active ?? ops.find((o) => o.kind === "firmware") ?? null;
  if (!last) return null;
  const by = last.expiresAt ? klTime(last.expiresAt).slice(11) : null;
  if (last.status === "queued" || last.status === "running") {
    return { tone: "primary" as const, title: `Firmware update ${d.firmwareVersion} → ${last.targetVersion}`, status: last.status === "queued" ? "Queued" : "Running",
      text: `Started ${klTime(last.startedAt ?? last.createdAt).slice(11)} · control commands locked (D05)${by ? ` · fails if the device does not confirm by ${by}` : ""}` };
  }
  if (last.status === "failed") return { tone: "crit" as const, title: `Firmware update to ${last.targetVersion} failed`, status: "Failed", text: `${failureText(last.failureCode)} · the old version ${d.firmwareVersion} is kept` };
  return { tone: "ok" as const, title: `Firmware ${last.targetVersion} installed`, status: "Succeeded", text: `finished ${last.finishedAt ? klTime(last.finishedAt) : "—"} · history appended` };
}

/** Why a device operation failed (IR67, REV19-018). */
export const failureText = (code: string | null | undefined) => ({
  TIMEOUT: "TIMEOUT — no confirmation within 60 s", OFFLINE: "OFFLINE — the device was not online at the start", CONFLICT: "CONFLICT — a command or another operation was running",
  UNAVAILABLE: "UNAVAILABLE — the device reported a failure",
} as Record<string, string>)[code ?? ""] ?? code ?? "no result";

/** One operation of the T11 history (Figma 04: started, “v1 → v2”, “heartbeat confirmed”). */
export type OperationRow = { id: string; kind: string; status: string; time: string; detail: string };
export function operationRows(ops: (ApiDeviceOperation & { startedAt?: string | null })[], current: string): OperationRow[] {
  return ops.map((o) => {
    const open = o.status === "queued" || o.status === "running";
    const detail = o.status === "failed" ? failureText(o.failureCode)
      : o.kind === "firmware" ? (open ? `${current} → ${o.targetVersion}` : `→ ${o.targetVersion} installed`)
      : o.status === "succeeded" ? "heartbeat confirmed" : o.status === "running" ? "waiting for the heartbeat" : "starts within a second";
    return { id: o.id, kind: o.kind, status: o.status, time: klTime(o.startedAt ?? o.createdAt), detail };
  });
}
const signed = (n: number) => (n < 0 ? `−${Math.abs(n).toFixed(1)}` : n.toFixed(1));
/** One calibration of the T11 history: reference, measured and the offset to apply (reference − measured). */
export const calibrationText = (c: ApiCalibration) =>
  `${c.metric} · ref ${c.referenceValue.toFixed(1)} ${c.unit} / measured ${c.measuredValue.toFixed(1)} ${c.unit} · offset ${signed(c.referenceValue - c.measuredValue)} ${c.unit} · ${klTime(c.calibratedAt)} — appended; earlier readings unchanged.`;

const eventDetail: Record<ApiDeviceEventFull["eventType"], string> = {
  tamper: "Cover opened while powered", power_lost: "Dedicated demo power signal lost", communication_lost: "Missed heartbeats (not proof of power loss)",
  restored: "Connection restored", operation_failed: "Device operation failed",
};
const restoredDetail: Record<NonNullable<ApiDeviceEventFull["recovery"]>["axis"], string> = {
  connection: "Connection restored", power: "Power signal restored", tamper: "Tamper cleared — cover closed (the alert stays until handled)",
};
/** One device event row (Figma 04 “Device events”): detection, evidence, its alerts and the recovery or response. */
export type EventRow = { id: string; version: number; time: string; type: ApiDeviceEventFull["eventType"]; tone: "warn" | "crit" | "ok" | "muted"; detail: string; recovery: string; notes: string[]; alerts: ApiAlertLite[] };
export function eventRows(es: ApiDeviceEventFull[], alerts: Map<string, ApiAlertLite>): EventRow[] {
  const byId = new Map(es.map((e) => [e.id, e]));
  return es.map((e) => {
    const as = e.alertIds.map((id) => alerts.get(id)).filter((a): a is ApiAlertLite => !!a);
    const open = as.filter((a) => a.status === "open").length;
    const src = e.recovery ? byId.get(e.recovery.sourceEventId) : undefined;
    const recovery = e.restoredAt ? `restored ${klTime(e.restoredAt).slice(5)}`
      : e.eventType === "restored" ? `recovers ${src ? `${src.eventType} ${klTime(src.occurredAt).slice(5)}` : "an earlier fault"}`
      : `open${as.length ? ` · ${open ? "unacknowledged" : as.every((a) => a.status === "resolved") ? "resolved" : "acknowledged"}` : ""}`;
    return {
      id: e.id, version: e.version, time: klTime(e.occurredAt).slice(5), type: e.eventType, tone: e.eventType === "tamper" ? "warn" : e.eventType === "restored" ? "ok" : e.eventType === "operation_failed" ? "muted" : "crit",
      detail: `${e.recovery ? restoredDetail[e.recovery.axis] : eventDetail[e.eventType]} · ${e.evidenceSource}${e.alertIds.length ? ` · ${e.alertIds.length} alert${e.alertIds.length === 1 ? "" : "s"}` : ""}`,
      recovery, notes: e.responseNotes.map((n) => `${klTime(n.at).slice(5)} — ${n.message}`), alerts: as,
    };
  });
}
/** The open alerts of the events, once each (acknowledged with alerts.acknowledge and the alert's version, SR23). */
export const openAlerts = (rows: EventRow[]) => [...new Map(rows.flatMap((r) => r.alerts).filter((a) => a.status === "open").map((a) => [a.id, a])).values()];
