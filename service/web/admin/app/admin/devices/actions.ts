"use server";

// Server Actions of the HQ device registry (FR-A04, FR-A20). Each one is a public endpoint: the DAL verifies the session
// and the Core API authorizes the call (device.write, audit.read) and checks the expected version where the write
// version catalog requires one.
import { refresh } from "next/cache";
import { coreNow, coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { CampaignDraft, Metric } from "@ac/web/lib/devices";
import { parseWaves } from "@ac/web/lib/devices";
import type { OpInput } from "@ac/web/lib/opTypes";
import { lookup } from "@ac/web/lib/lookup";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

async function run<T>(fn: () => Promise<T>, rerender = true): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (error) {
    return actionFailure(error);
  } finally {
    if (rerender) refresh();
  }
}

/** capabilities.save: a new model (no id) or the next capability version (id + the version it was read at). */
export async function saveCapability(input: OpInput<"capabilities.save">, version?: number) {
  return run(async () => { const k = await coreOp<{ id: string; version: number }>("capabilities.save", input, { write: true, expectedVersion: input.id ? version : undefined }); return ({ id: k.id, version: k.version }); });
}

/** iotModels.save: a new IoT model (no id) or its next version (id + the version it was read at; DEC-105). */
export async function saveIotModel(input: OpInput<"iotModels.save">, version?: number) {
  return run(async () => { const k = await coreOp<{ id: string; version: number }>("iotModels.save", input, { write: true, expectedVersion: input.id ? version : undefined }); return ({ id: k.id, version: k.version }); });
}

export interface HistoryEntry { at: string; actor: string; from: number | null; to: number | null; reason: string | null; changes: string[] }

/** The saved versions of a capability: audit.list for its id over the last 12 months (DD-A04 step 6; a read). */
export async function capabilityHistory(capabilityId: string) {
  return run(async () => {
    const now = await coreNow();
    const from = new Date(now.getTime() - 365 * 24 * 3600 * 1000).toISOString();
    const to = new Date(now.getTime() + 24 * 3600 * 1000).toISOString();
    const page = await coreOp<{ items: { occurredAt: string; actorId: string; previousVersion: number | null; nextVersion: number | null; reason: string | null; maskedBefore: Record<string, string | null> | null; maskedAfter: Record<string, string | null> | null }[] }>(
      "audit.list", { limit: 100, filters: { targetId: capabilityId, from, to } });
    return page.items.map((a): HistoryEntry => {
      return {
        at: a.occurredAt, actor: a.actorId, from: a.previousVersion, to: a.nextVersion, reason: a.reason,
        changes: Object.keys({ ...a.maskedBefore, ...a.maskedAfter }).filter((k) => lookup(a.maskedBefore ?? {}, k) !== lookup(a.maskedAfter ?? {}, k)),
      };
    });
  }, false);
}

export async function registerDevice(serial: string, sensorTypes: Metric[], unitId: string, mqttNode: string | null, iotModelId: string | null) {
  return run(async () => { const d = await coreOp<{ id: string }>("devices.register", { serial, sensorTypes, unitId, ...(mqttNode && { mqttNode }), ...(iotModelId && { iotModelId }) }, { write: true }); return d.id; });
}

/** devices.setMqttNode: the node ID the device's ESP32 sends (null clears it); the IoT bridge reads it within 30 seconds (IR635). */
export async function setMqttNode(deviceId: string, version: number, mqttNode: string | null) {
  return run(async () => { await coreOp("devices.setMqttNode", { deviceId, mqttNode }, { write: true, expectedVersion: version }); return null; });
}

export async function bindDevice(deviceId: string, version: number, unitId: string, reason: string) {
  return run(async () => { await coreOp("devices.bind", { deviceId, unitId, reason }, { write: true, expectedVersion: version }); return null; });
}

/** devices.unbind: the device leaves its unit and stays registered (DEC-103). */
export async function unbindDevice(deviceId: string, version: number, reason: string) {
  return run(async () => { await coreOp("devices.unbind", { deviceId, reason }, { write: true, expectedVersion: version }); return null; });
}

/** devices.delete: a device that is not bound, has no operation under way and is in no unfinished campaign (DEC-103). */
export async function deleteDevice(deviceId: string, version: number, reason: string) {
  return run(async () => { await coreOp("devices.delete", { deviceId, reason }, { write: true, expectedVersion: version }); return null; });
}

/** capabilities.delete: a model no unit uses and no campaign targets, with every version (DEC-103). */
export async function deleteCapability(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("capabilities.delete", { id, reason }, { write: true, expectedVersion: version }); return null; });
}

/** iotModels.delete: an IoT model no unit or device uses and no campaign targets, with every version (DEC-105). */
export async function deleteIotModel(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("iotModels.delete", { id, reason }, { write: true, expectedVersion: version }); return null; });
}

export async function checkDevice(deviceId: string, version: number) {
  return run(async () => { await coreOp("devices.check", { id: deviceId }, { write: true, expectedVersion: version }); return null; });
}

export async function calibrateSensor(deviceId: string, version: number, sensorId: string, metric: Metric, unit: OpInput<"devices.calibrate">["unit"], referenceValue: number, measuredValue: number) {
  return run(async () => {
    const now = await coreNow();
    const calibratedAt = now.toISOString();
    await coreOp("devices.calibrate", { deviceId, sensorId, metric, unit, referenceValue, measuredValue, calibratedAt }, { write: true, expectedVersion: version });
    return null;
  });
}

export async function updateFirmware(deviceId: string, version: number, firmwareVersion: string) {
  return run(async () => { await coreOp("devices.updateFirmware", { deviceId, firmwareVersion }, { write: true, expectedVersion: version }); return null; });
}

/** firmwareCampaigns.schedule; `startAt` is the instant the form read in the user's display time zone (NFR-08). */
export async function scheduleCampaign(d: CampaignDraft, startAt: string) {
  return run(async () => { const c = await coreOp<{ id: string }>("firmwareCampaigns.schedule", {
      modelId: d.modelId, targetVersion: d.targetVersion, deviceIds: d.deviceIds, waves: parseWaves(d.waves) ?? [], window: { startLocal: d.startLocal, endLocal: d.endLocal },
      autoPauseFailurePercent: Number(d.autoPause), startAt,
    }, { write: true }); return c.id; });
}

/** firmwareCampaigns.delete: a campaign that has ended (aborted or completed), with its device results (DEC-103). */
export async function deleteCampaign(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("firmwareCampaigns.delete", { id, reason }, { write: true, expectedVersion: version }); return null; });
}

export async function controlCampaign(campaignId: string, version: number, action: "pause" | "resume" | "abort" | "retry_device", deviceId?: string, reason?: string) {
  return run(async () => { await coreOp("firmwareCampaigns.control", { campaignId, action, ...(deviceId && { deviceId }), ...(reason && { reason }) }, { write: true, expectedVersion: version }); return null; });
}
