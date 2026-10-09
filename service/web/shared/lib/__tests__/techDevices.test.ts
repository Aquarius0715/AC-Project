import { describe, expect, it } from "vitest";
import type { ApiDevice, ApiDeviceDetail, ApiDeviceOperation } from "@ac/web/lib/devices";
import {
  calibrationText, deviceTiles, eventRows, failureText, filterOf, firmwareCard, jobFor, newerFirmware, newerVersions, openAlerts, openFaults, operationRows,
  techDeviceRows, type ApiAlertLite, type ApiDeviceEventFull, type TechJob,
} from "@ac/web/lib/techDevices";

// 2026-09-14T01:05:00Z is 09:05 in Kuala Lumpur (the display time zone of every mapper)
const NOW = Date.parse("2026-09-14T01:05:00Z");
const device = (over: Partial<ApiDevice> = {}): ApiDevice => ({
  id: "dev-1", version: 1, unitId: "unit-1", serial: "AC-DEMO-0001", connection: "online", lastSeenAt: "2026-09-14T01:04:30Z", firmwareVersion: "v1", powerSignal: "on", tamper: "clear", sensors: [], ...over,
});
const detail = (over: Partial<ApiDeviceDetail> = {}): ApiDeviceDetail => ({ ...device(), calibrationRefs: [], activeOperation: null, ...over });
const op = (over: Partial<ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string }> = {}): ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string } => ({
  id: "op-1", kind: "check", status: "succeeded", targetVersion: null, failureCode: null, createdAt: "2026-09-14T01:00:00Z", finishedAt: "2026-09-14T01:00:05Z", ...over,
});
const event = (over: Partial<ApiDeviceEventFull> = {}): ApiDeviceEventFull => ({
  id: "ev-1", version: 1, deviceId: "dev-1", eventType: "communication_lost", evidenceSource: "heartbeat", occurredAt: "2026-09-14T01:00:00Z", restoredAt: null, alertIds: [], recovery: null, responseNotes: [], ...over,
});
const alert = (over: Partial<ApiAlertLite> = {}): ApiAlertLite => ({ id: "al-1", version: 1, status: "open", type: "tamper", severity: "critical", ...over });

describe("jobFor (IR94: the job a device write runs under)", () => {
  const jobs: TechJob[] = [{ id: "j-assigned", unitId: "unit-1", status: "assigned" }, { id: "j-progress", unitId: "unit-1", status: "in_progress" }, { id: "j-rework", unitId: "unit-2", status: "rework_requested" }, { id: "j-done", unitId: "unit-3", status: "completed" }];
  it("prefers the job in progress over an assigned one", () => expect(jobFor("unit-1", jobs)).toBe("j-progress"));
  it("takes a rework job when nothing else is open", () => expect(jobFor("unit-2", jobs)).toBe("j-rework"));
  it("gives null for completed jobs, unknown units and unbound devices", () => {
    expect(jobFor("unit-3", jobs)).toBeNull();
    expect(jobFor("unit-9", jobs)).toBeNull();
    expect(jobFor(null, jobs)).toBeNull();
  });
});

describe("techDeviceRows / filterOf", () => {
  const names = new Map([["unit-1", "Bedroom AC"]]);
  const rows = techDeviceRows([device({ id: "b", serial: "AC-B", unitId: null, connection: "offline", lastSeenAt: "2026-09-14T01:04:30Z", tamper: "detected" }), device({ id: "a", serial: "AC-A" }), device({ id: "c", serial: "AC-C", connection: "unknown", lastSeenAt: null })], names, NOW);
  it("sorts by serial and names the unit (Unbound without one)", () => {
    expect(rows.map((r) => r.serial)).toEqual(["AC-A", "AC-B", "AC-C"]);
    expect(rows[0].unit).toBe("Bedroom AC");
    expect(rows[1].unit).toBe("Unbound");
  });
  it("shows the age of the last heartbeat only while the device is not online", () => {
    expect(rows[0].connText).toBeNull();
    expect(rows[1].connText).toBe("30 s ago");
    expect(rows[2].connText).toBeNull();
  });
  it("filters all / online / offline (any non-online) / tamper", () => {
    expect(rows.filter(filterOf("all")).length).toBe(3);
    expect(rows.filter(filterOf("online")).map((r) => r.id)).toEqual(["a"]);
    expect(rows.filter(filterOf("offline")).map((r) => r.id)).toEqual(["b", "c"]);
    expect(rows.filter(filterOf("tamper")).map((r) => r.id)).toEqual(["b"]);
  });
});

describe("deviceTiles", () => {
  it("reads an online device with the heartbeat age and the newer firmware", () => {
    const [conn, power, tamper, fw] = deviceTiles(detail(), "v2", NOW);
    expect(conn).toMatchObject({ value: "Online", dot: "ok", sub: "last seen 30 s ago" });
    expect(power).toMatchObject({ value: "ON", dot: "primary" });
    expect(tamper).toMatchObject({ value: "Clear", dot: "ok", sub: "no removal detected" });
    expect(fw).toMatchObject({ value: "v1", sub: "v2 available" });
    expect(deviceTiles(detail(), null, NOW)[3].sub).toBe("latest");
  });
  it("explains a running check and dates the open faults of each axis (T12)", () => {
    expect(deviceTiles(detail({ connection: "connecting" }), null, NOW)[0]).toMatchObject({ value: "Connecting", dot: "primary", sub: "check running — waiting for the device" });
    const faults = { connection: "2026-09-14T00:12:00Z", power: "2026-09-14T00:33:00Z", tamper: "2026-09-14T00:41:00Z" };
    const [conn, power, tamper] = deviceTiles(detail({ connection: "offline", powerSignal: "off", tamper: "detected" }), null, NOW, faults);
    expect(conn).toMatchObject({ value: "Offline", dot: "crit", sub: "lost 08:12" });
    expect(power).toMatchObject({ value: "Lost", dot: "crit", sub: "power_signal 08:33" });
    expect(tamper).toMatchObject({ value: "Detected", dot: "warn", sub: "cover opened 08:41", warn: true });
  });
  it("points to the events when a fault has no dated event", () => {
    const [conn, , tamper] = deviceTiles(detail({ connection: "error", tamper: "detected" }), null, NOW);
    expect(conn).toMatchObject({ value: "Error", dot: "crit", sub: "last seen 30 s ago" });
    expect(tamper.sub).toBe("cover opened — see device events");
  });
});

describe("firmware candidates", () => {
  it("offers only versions above the installed one, in numeric order", () => {
    expect(newerVersions("v2", ["v10", "v1", "v2", "v3", "v3"])).toEqual(["v3", "v10"]);
    expect(newerVersions("v10", ["v1", "v2"])).toEqual([]);
    expect(newerFirmware("v1", ["v2", "v10"])).toBe("v10");
    expect(newerFirmware("v10", ["v2", "v10"])).toBeNull();
  });
});

describe("firmwareCard", () => {
  it("is absent without a firmware operation", () => expect(firmwareCard(detail(), [op()])).toBeNull());
  it("shows the active update with the D05 lock and its deadline", () => {
    const active = op({ id: "fw", kind: "firmware", status: "running", targetVersion: "v2", startedAt: "2026-09-14T01:00:01Z", expiresAt: "2026-09-14T01:01:00Z", finishedAt: null });
    expect(firmwareCard(detail({ activeOperation: active }), [])).toEqual({ tone: "primary", title: "Firmware update v1 → v2", status: "Running", text: "Started 09:00 · control commands locked (D05) · fails if the device does not confirm by 09:01" });
    const queued = { ...active, status: "queued" as const, startedAt: null };
    expect(firmwareCard(detail({ activeOperation: queued }), [])?.status).toBe("Queued");
  });
  it("keeps the old version after a failure and reports a finished install", () => {
    expect(firmwareCard(detail(), [op({ kind: "firmware", status: "failed", targetVersion: "v2", failureCode: "TIMEOUT" })])).toEqual({ tone: "crit", title: "Firmware update to v2 failed", status: "Failed", text: "TIMEOUT — no confirmation within 60 s · the old version v1 is kept" });
    expect(firmwareCard(detail({ firmwareVersion: "v2" }), [op({ kind: "firmware", status: "succeeded", targetVersion: "v2", finishedAt: "2026-09-14T01:00:05Z" })])).toEqual({ tone: "ok", title: "Firmware v2 installed", status: "Succeeded", text: "finished 2026-09-14 09:00 · history appended" });
  });
});

describe("operation history", () => {
  it("names every failure code and passes unknown ones through", () => {
    expect(failureText("TIMEOUT")).toMatch(/^TIMEOUT — no confirmation within 60 s$/);
    expect(failureText("OFFLINE")).toMatch(/not online at the start/);
    expect(failureText("CONFLICT")).toMatch(/another operation/);
    expect(failureText("UNAVAILABLE")).toMatch(/reported a failure/);
    expect(failureText("DEVICE_REJECTED")).toBe("DEVICE_REJECTED");
    expect(failureText(null)).toBe("no result");
  });
  it("describes each operation state from the technician's point of view", () => {
    const rows = operationRows([
      op({ id: "1" }), op({ id: "2", status: "running", startedAt: "2026-09-14T01:00:01Z", finishedAt: null }), op({ id: "3", status: "queued", finishedAt: null }),
      op({ id: "4", kind: "firmware", status: "running", targetVersion: "v2", finishedAt: null }), op({ id: "5", kind: "firmware", status: "succeeded", targetVersion: "v2" }),
      op({ id: "6", status: "failed", failureCode: "TIMEOUT" }),
    ], "v1");
    expect(rows.map((r) => r.detail)).toEqual(["heartbeat confirmed", "waiting for the heartbeat", "starts within a second", "v1 → v2", "→ v2 installed", "TIMEOUT — no confirmation within 60 s"]);
    expect(rows[1].time).toBe("2026-09-14 09:00"); // startedAt, else createdAt
    expect(rows[0]).toMatchObject({ id: "1", kind: "check", status: "succeeded" });
  });
  it("prints a calibration with the signed offset reference − measured", () => {
    const base = { id: "c", sensorId: "s", metric: "temperature" as const, unit: "°C", calibratedAt: "2026-09-14T01:04:00Z", actorId: "u" };
    expect(calibrationText({ ...base, referenceValue: 25, measuredValue: 25.4 })).toBe("temperature · ref 25.0 °C / measured 25.4 °C · offset −0.4 °C · 2026-09-14 09:04 — appended; earlier readings unchanged.");
    expect(calibrationText({ ...base, referenceValue: 25.5, measuredValue: 25 })).toContain("offset 0.5 °C");
  });
});

describe("device events (T12)", () => {
  const lost = event({ id: "lost", sequence: 1 } as Partial<ApiDeviceEventFull>);
  it("finds the open fault of each axis and ignores restored ones", () => {
    const es = [
      lost, event({ id: "lost-old", occurredAt: "2026-09-13T23:00:00Z", restoredAt: "2026-09-13T23:30:00Z" }),
      event({ id: "power", eventType: "power_lost", evidenceSource: "power_signal", occurredAt: "2026-09-14T00:30:00Z" }),
      event({ id: "tamper", eventType: "tamper", evidenceSource: "tamper_signal", occurredAt: "2026-09-14T00:40:00Z", restoredAt: "2026-09-14T00:50:00Z" }),
      event({ id: "back", eventType: "restored", occurredAt: "2026-09-14T00:50:00Z", recovery: { axis: "tamper", sourceEventId: "tamper", value: "clear" } }),
    ];
    expect(openFaults(es)).toEqual({ connection: "2026-09-14T01:00:00Z", power: "2026-09-14T00:30:00Z" });
    expect(openFaults([])).toEqual({});
  });
  it("renders detection, evidence, alerts, recovery and notes per row", () => {
    const alerts = new Map([["al-1", alert()], ["al-2", alert({ id: "al-2", status: "acknowledged" })], ["al-3", alert({ id: "al-3", status: "resolved" })]]);
    const es = [
      event({ id: "r", eventType: "restored", occurredAt: "2026-09-14T01:10:00Z", recovery: { axis: "connection", sourceEventId: "lost", value: "online" } }),
      event({ id: "t", eventType: "tamper", evidenceSource: "tamper_signal", alertIds: ["al-1", "missing"], responseNotes: [{ actorId: "u", message: "Cover refitted", at: "2026-09-14T01:02:00Z" }] }),
      event({ id: "t2", eventType: "tamper", evidenceSource: "tamper_signal", alertIds: ["al-2"] }), event({ id: "t3", eventType: "tamper", evidenceSource: "tamper_signal", alertIds: ["al-3"] }),
      { ...lost, restoredAt: "2026-09-14T01:10:00Z" }, event({ id: "f", eventType: "operation_failed" }), event({ id: "p", eventType: "power_lost", evidenceSource: "power_signal" }),
    ];
    const rows = eventRows(es, alerts);
    expect(rows[0]).toMatchObject({ tone: "ok", detail: "Connection restored · heartbeat", recovery: "recovers communication_lost 09-14 09:00", time: "09-14 09:10" });
    expect(rows[1]).toMatchObject({ tone: "warn", detail: "Cover opened while powered · tamper_signal · 2 alerts", recovery: "open · unacknowledged", notes: ["09-14 09:02 — Cover refitted"] });
    expect(rows[1].alerts.map((a) => a.id)).toEqual(["al-1"]);
    expect(rows[2].recovery).toBe("open · acknowledged");
    expect(rows[3].recovery).toBe("open · resolved");
    expect(rows[4]).toMatchObject({ tone: "crit", detail: "Missed heartbeats (not proof of power loss) · heartbeat", recovery: "restored 09-14 09:10" });
    expect(rows[5].tone).toBe("muted");
    expect(rows[6]).toMatchObject({ tone: "crit", detail: "Dedicated demo power signal lost · power_signal", recovery: "open" });
    expect(eventRows([event({ eventType: "restored", recovery: { axis: "power", sourceEventId: "gone", value: "on" } })], new Map())[0].recovery).toBe("recovers an earlier fault");
  });
  it("lists each open alert once for the acknowledge links", () => {
    const rows = eventRows([event({ id: "a", alertIds: ["al-1"] }), event({ id: "b", eventType: "restored", alertIds: ["al-1"], recovery: { axis: "connection", sourceEventId: "a", value: "online" } }), event({ id: "c", alertIds: ["al-2"] })],
      new Map([["al-1", alert()], ["al-2", alert({ id: "al-2", status: "acknowledged" })]]));
    expect(openAlerts(rows).map((a) => a.id)).toEqual(["al-1"]);
  });
});
