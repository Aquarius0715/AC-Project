import { beforeEach, describe, expect, it, vi } from "vitest";

// the Server Actions of the technician device, alert evidence and diagnostic control screens: each write carries the
// version the API checks, the reasons and notes are trimmed, the page is refreshed whether or not the call succeeds, a
// refused call keeps its code and field errors, and a call that got no answer reads as unavailable
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; correlationId: string; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    readonly status: number;
    constructor(error: DomainError, options: ErrorOptions & { status: number }) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
      this.status = options.status;
    }
  }
  return { coreOp: vi.fn(), coreNow: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);

import { acknowledgeAlert as ackDeviceAlert, addResponseNote, bindDevice, calibrateSensor, checkDevice, registerDevice, updateFirmware } from "../app/technician/devices/actions";
import { acknowledgeAlert, resolveAlert } from "../app/technician/units/[id]/alerts/actions";
import { sendDiagnostic, startTestRun } from "../app/technician/units/[id]/control/actions";

const refusal = (code: string, fieldErrors: Record<string, string> = {}) =>
  new dal.CoreError({ code, fieldErrors, messageKey: `error.${code.toLowerCase()}`, correlationId: "c-1", retryAfterSeconds: null }, { status: 409 });
const unavailable = { ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} };

beforeEach(() => { dal.coreOp.mockReset(); dal.coreNow.mockReset(); cache.refresh.mockReset(); });

describe("device actions", () => {
  it("registers a device and returns its id", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "dev-9" });
    await expect(registerDevice("ac-001", ["temperature"], "unit-1", "job-1")).resolves.toStrictEqual({ ok: true, value: "dev-9" });
    expect(dal.coreOp).toHaveBeenCalledWith("devices.register", { serial: "ac-001", sensorTypes: ["temperature"], unitId: "unit-1", jobId: "job-1" }, { write: true });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("returns a refused register with its field errors and still refreshes", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("VALIDATION", { serial: "errors.duplicate" }));
    await expect(registerDevice("AC-001", [], "unit-1", "job-1")).resolves.toStrictEqual({
      ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { serial: "errors.duplicate" }, retryAfterSeconds: null,
    });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("binds with the device version and a trimmed reason", async () => {
    dal.coreOp.mockResolvedValueOnce({ ignored: true });
    await expect(bindDevice("dev-1", 3, "unit-2", "job-2", "  moved to the bedroom  ")).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenCalledWith("devices.bind", { deviceId: "dev-1", unitId: "unit-2", jobId: "job-2", reason: "moved to the bedroom" }, { write: true, expectedVersion: 3 });
  });

  it("checks, updates firmware and acknowledges an alert with their versions", async () => {
    dal.coreOp.mockResolvedValue(undefined);
    await checkDevice("dev-1", 4, "job-1");
    await updateFirmware("dev-1", 5, "2.1.0", "job-1");
    await ackDeviceAlert("alert-1", 6);
    expect(dal.coreOp.mock.calls).toStrictEqual([
      ["devices.check", { id: "dev-1", jobId: "job-1" }, { write: true, expectedVersion: 4 }],
      ["devices.updateFirmware", { deviceId: "dev-1", firmwareVersion: "2.1.0", jobId: "job-1" }, { write: true, expectedVersion: 5 }],
      ["alerts.acknowledge", { alertId: "alert-1" }, { write: true, expectedVersion: 6 }],
    ]);
    expect(cache.refresh).toHaveBeenCalledTimes(3);
  });

  it("adds a trimmed response note with the event's version", async () => {
    dal.coreOp.mockResolvedValueOnce(undefined);
    await expect(addResponseNote("dev-1", "ev-1", 2, "\tReplugged the sensor ")).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenCalledWith("devices.addResponseNote", { deviceId: "dev-1", eventId: "ev-1", responseNote: "Replugged the sensor" }, { write: true, expectedVersion: 2 });
  });

  it("calibrates at the business time of the Core API, not the server clock", async () => {
    dal.coreNow.mockResolvedValueOnce(new Date("2026-09-20T06:30:00Z"));
    dal.coreOp.mockResolvedValueOnce(undefined);
    await expect(calibrateSensor("dev-1", 7, "s-1", "temperature", "°C", 24, 24.6, "job-1")).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenCalledWith("devices.calibrate", {
      deviceId: "dev-1", sensorId: "s-1", metric: "temperature", unit: "°C", referenceValue: 24, measuredValue: 24.6, calibratedAt: "2026-09-20T06:30:00.000Z", jobId: "job-1",
    }, { write: true, expectedVersion: 7 });
  });

  it("does not calibrate when the business time cannot be read, and reads as unavailable", async () => {
    dal.coreNow.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(calibrateSensor("dev-1", 7, "s-1", "temperature", "°C", 24, 24.6, "job-1")).resolves.toStrictEqual(unavailable);
    expect(dal.coreOp).not.toHaveBeenCalled();
    expect(cache.refresh).toHaveBeenCalledOnce();
  });
});

describe("alert evidence actions", () => {
  it("acknowledges with the alert version", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "alert-1" });
    await expect(acknowledgeAlert("alert-1", 8)).resolves.toStrictEqual({ ok: true });
    expect(dal.coreOp).toHaveBeenCalledWith("alerts.acknowledge", { alertId: "alert-1" }, { write: true, expectedVersion: 8 });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("resolves with the reason and the cited evidence, and returns a version conflict as it is", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT"));
    await expect(resolveAlert("alert-1", 9, "Cleaned the filter", ["rd-1", "rd-2"])).resolves.toStrictEqual({
      ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {}, retryAfterSeconds: null,
    });
    expect(dal.coreOp).toHaveBeenCalledWith("alerts.resolve", { alertId: "alert-1", resolutionReason: "Cleaned the filter", resolutionEvidenceIds: ["rd-1", "rd-2"] }, { write: true, expectedVersion: 9 });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });
});

describe("diagnostic control actions", () => {
  it("sends a diagnostic command with a trimmed reason and returns the command", async () => {
    const row = { id: "cmd-1", status: "sent" };
    dal.coreOp.mockResolvedValueOnce(row);
    const input = { unitId: "unit-1", jobId: "job-1", action: { kind: "set_power" as const, power: true }, reason: " Checking the compressor ", expectedUnitVersion: 3 };
    await expect(sendDiagnostic(input)).resolves.toStrictEqual({ ok: true, value: row });
    expect(dal.coreOp).toHaveBeenCalledWith("commands.create", { ...input, reason: "Checking the compressor" }, { write: true });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("starts a test run with a trimmed reason, and a call with no answer reads as unavailable", async () => {
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    const input = { jobId: "job-1", unitId: "unit-1", startAction: { kind: "set_power" as const, power: true }, endAction: { kind: "set_power" as const, power: false }, durationMinutes: 5, reason: "Cooling test\n", expectedUnitVersion: 3, expectedJobVersion: 4 };
    await expect(startTestRun(input)).resolves.toStrictEqual(unavailable);
    expect(dal.coreOp).toHaveBeenCalledWith("diagnosticRuns.create", { ...input, reason: "Cooling test" }, { write: true });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });
});
