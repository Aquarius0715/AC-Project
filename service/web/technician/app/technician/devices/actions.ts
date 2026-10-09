"use server";

// Server Actions of the technician device screens (FR-T11, FR-T12). Each one is a public endpoint: the DAL verifies the
// session and the Core API authorizes it (technician:device.maintain on an assigned job of the unit, IR94; alerts with
// alert.resolve) and checks the version the write version catalog requires.
import { refresh } from "next/cache";
import { coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import type { Metric } from "@ac/web/lib/devices";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run<T>(fn: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}

/** devices.register on an assigned unit (serial trimmed and upper-cased by the API before the uniqueness check). */
export async function registerDevice(serial: string, sensorTypes: Metric[], unitId: string, jobId: string) {
  return run(() => coreOp<{ id: string }>("devices.register", { serial, sensorTypes, unitId, jobId }, { write: true }).then((d) => d.id));
}
/** devices.bind to another assigned unit; a reason is required and the device gets new sensor IDs (SR24). */
export async function bindDevice(deviceId: string, version: number, unitId: string, jobId: string, reason: string) {
  return run(() => coreOp("devices.bind", { deviceId, unitId, jobId, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
export async function checkDevice(deviceId: string, version: number, jobId: string) {
  return run(() => coreOp("devices.check", { id: deviceId, jobId }, { write: true, expectedVersion: version }).then(() => null));
}
/** devices.calibrate at the current business time (a demo record; earlier readings stay unchanged). */
export async function calibrateSensor(deviceId: string, version: number, sensorId: string, metric: Metric, unit: string, referenceValue: number, measuredValue: number, jobId: string) {
  return run(async () => {
    const calibratedAt = (await coreNow()).toISOString();
    await coreOp("devices.calibrate", { deviceId, sensorId, metric, unit, referenceValue, measuredValue, calibratedAt, jobId }, { write: true, expectedVersion: version });
    return null;
  });
}
export async function updateFirmware(deviceId: string, version: number, firmwareVersion: string, jobId: string) {
  return run(() => coreOp("devices.updateFirmware", { deviceId, firmwareVersion, jobId }, { write: true, expectedVersion: version }).then(() => null));
}
/** devices.addResponseNote on one device event (its version); the device's physical state does not change. */
export async function addResponseNote(deviceId: string, eventId: string, eventVersion: number, responseNote: string) {
  return run(() => coreOp("devices.addResponseNote", { deviceId, eventId, responseNote: responseNote.trim() }, { write: true, expectedVersion: eventVersion }).then(() => null));
}
/** alerts.acknowledge of an alert raised by a device event (its version). Reconnection never clears it (DD-T12). */
export async function acknowledgeAlert(alertId: string, version: number) {
  return run(() => coreOp("alerts.acknowledge", { alertId }, { write: true, expectedVersion: version }).then(() => null));
}
