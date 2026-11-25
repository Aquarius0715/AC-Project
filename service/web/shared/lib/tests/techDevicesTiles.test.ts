import { describe, expect, it } from "vitest";
import type { ApiDevice, ApiDeviceDetail, ApiDeviceOperation } from "@ac/web/lib/devices";
import {
  connectionDot, deviceRefusal, deviceTiles, firmwareCard, firmwareCardLook, firmwareSub, jobsFor, operationRows, operationTone, sensorRows, techDeviceRows,
} from "@ac/web/lib/techDevices";
import { translator } from "@ac/web/lib/i18n";

const t = translator("en");

// The technician's and HQ's device screens (FR-T11, FR-T12): the filter, card and operation colours, the firmware
// action's line, the jobs a device write runs under for an unbound device, "last seen" in minutes and never, a device
// planned for an unknown unit, the error and unknown-power tiles, a queued update without its deadline, an update or
// installation without its target version, a calibrated sensor, and a duplicate serial found by its field.

const NOW = Date.parse("2026-09-14T01:05:00Z");
const device = (over: Partial<ApiDevice> = {}): ApiDevice => ({
  id: "dev-1", version: 1, unitId: "unit-1", serial: "AC-0001", connection: "online", lastSeenAt: "2026-09-14T01:04:30Z", firmwareVersion: "v1", powerSignal: "on", tamper: "clear", sensors: [], ...over,
});
const detail = (over: Partial<ApiDeviceDetail> = {}): ApiDeviceDetail => ({ ...device(), calibrationRefs: [], activeOperation: null, ...over });
type Op = ApiDeviceOperation & { startedAt?: string | null; expiresAt?: string };
const op = (over: Partial<Op>): Op => ({ id: "op-1", kind: "firmware", status: "succeeded", targetVersion: null, failureCode: null, createdAt: "2026-09-14T01:00:00Z", finishedAt: null, ...over });

describe("the screens' colours and the firmware action", () => {
  it("colours each connection filter, firmware card and operation, warning for the rest", () => {
    expect([connectionDot("online"), connectionDot("offline"), connectionDot("unknown")]).toStrictEqual(["bg-ok", "bg-crit", "bg-warn"]);
    expect([firmwareCardLook("crit"), firmwareCardLook("ok"), firmwareCardLook("primary")]).toStrictEqual(["border-crit bg-crit-soft/40", "border-[#86efac] bg-ok-soft/40", "border-primary bg-primary-soft/40"]);
    expect([operationTone("succeeded"), operationTone("failed"), operationTone("running")]).toStrictEqual(["ok", "crit", "primary"]);
  });

  it("says the update is running, which versions are on offer, or none", () => {
    expect([firmwareSub(true, ["v2"], t), firmwareSub(false, ["v2", "v3"], t), firmwareSub(false, [], t)]).toStrictEqual(["running…", "v2, v3 available", "no newer candidate"]);
  });

  it("offers no job for an unbound device", () => {
    expect(jobsFor(null, [{ id: "j", unitId: "unit-1", status: "assigned" }])).toStrictEqual([]);
  });
});

describe("the device list and tiles", () => {
  it("says how long ago an offline device was seen, and names an unknown planned unit", () => {
    const rows = techDeviceRows([
      device({ id: "a", serial: "A", connection: "offline", lastSeenAt: "2026-09-14T00:35:00Z" }), device({ id: "b", serial: "B", connection: "offline", lastSeenAt: null }),
      device({ id: "c", serial: "C", unitId: null, targetUnitId: "unit-gone" }),
    ], new Map(), NOW);
    expect(rows.map((r) => [r.connText, r.unit])).toStrictEqual([["30 min ago", "unit"], [null, "unit"], [null, "Not bound yet — planned for unit"]]);
  });

  it("marks an error connection red, an unknown power signal grey, and a device never seen", () => {
    const [connection, power] = deviceTiles(detail({ connection: "error", lastSeenAt: null, powerSignal: "unknown" }), null, NOW);
    expect([connection?.value, connection?.dot, connection?.sub, power?.value, power?.dot]).toStrictEqual(["Error", "crit", "last seen never seen", "Unknown", "muted"]);
  });
});

describe("the firmware card and the operations", () => {
  it("shows a queued update without a deadline, and an update or installation without its target", () => {
    const queued = firmwareCard(detail({ activeOperation: op({ status: "queued" }) }), []);
    expect([queued?.title, queued?.status, queued?.text.includes("fails if")]).toStrictEqual(["Firmware update v1 → —", "Queued", false]);
    expect(firmwareCard(detail(), [op({ status: "failed", failureCode: "TIMEOUT" })])?.title).toBe("Firmware update to — failed");
    expect(firmwareCard(detail(), [op({})])).toMatchObject({ title: "Firmware — installed", text: "finished — · history appended" });
    expect(firmwareCard(detail({ activeOperation: op({ kind: "check", status: "running" }) }), [])).toBeNull(); // a check is not a firmware update
  });

  it("words a running firmware update and an installed one without their target", () => {
    expect(operationRows([op({ status: "running" }), op({ id: "op-2" })], "v1").map((r) => r.detail)).toStrictEqual(["v1 → —", "— installed"]);
  });

  it("shows when a sensor was calibrated", () => {
    const [sensor] = sensorRows(device({ sensors: [{ id: "s1", metric: "temperature", unit: "°C", staleAfterSeconds: 60, calibratedAt: "2026-09-10T02:00:00Z" }] }));
    expect([sensor?.metric, sensor?.stale, sensor?.calibrated === "Never"]).toStrictEqual(["Temperature", "60 s", false]);
  });
});

describe("a refused device write", () => {
  it("finds a duplicate serial by its field on a conflict, and gives no text for a VALIDATION", () => {
    expect(deviceRefusal({ code: "CONFLICT", messageKey: "error.conflict", fieldErrors: { serial: "error.duplicate" } })).toBe("This serial is already registered.");
    expect(deviceRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { serial: "error.required" } })).toBeNull();
  });
});
