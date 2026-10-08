"use server";

// Server Actions of the HQ device registry (FR-A04, FR-A20). Each one is a public endpoint: the DAL verifies the session
// and the Core API authorizes the call (device.write, audit.read) and checks the expected version where the write
// version catalog requires one.
import { refresh } from "next/cache";
import { coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import type { CampaignDraft, Metric } from "@ac/web/lib/devices";
import { parseWaves } from "@ac/web/lib/devices";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run<T>(fn: () => Promise<T>, rerender = true): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    if (rerender) refresh();
  }
}

/** capabilities.save: a new model (no id) or the next capability version (id + the version it was read at). */
export async function saveCapability(input: Record<string, unknown> & { id?: string }, version?: number) {
  return run(() => coreOp<{ id: string; version: number }>("capabilities.save", input, { write: true, expectedVersion: input.id ? version : undefined }).then((k) => ({ id: k.id, version: k.version })));
}

export type HistoryEntry = { at: string; actor: string; from: number | null; to: number | null; reason: string | null; changes: string[] };

/** The saved versions of a capability: audit.list for its id over the last 12 months (DD-A04 step 6; a read). */
export async function capabilityHistory(capabilityId: string) {
  return run(async () => {
    const now = await coreNow();
    const from = new Date(now.getTime() - 365 * 24 * 3600 * 1000).toISOString();
    const to = new Date(now.getTime() + 24 * 3600 * 1000).toISOString();
    const page = await coreOp<{ items: { occurredAt: string; actorId: string; previousVersion: number | null; nextVersion: number | null; reason: string | null; maskedBefore: Record<string, string | null> | null; maskedAfter: Record<string, string | null> | null }[] }>(
      "audit.list", { limit: 100, filters: { targetId: capabilityId, from, to } });
    return page.items.map((a): HistoryEntry => ({
      at: a.occurredAt, actor: a.actorId, from: a.previousVersion, to: a.nextVersion, reason: a.reason,
      changes: Object.keys({ ...a.maskedBefore, ...a.maskedAfter }).filter((k) => (a.maskedBefore ?? {})[k] !== (a.maskedAfter ?? {})[k]),
    }));
  }, false);
}

export async function registerDevice(serial: string, sensorTypes: Metric[], unitId: string) {
  return run(() => coreOp<{ id: string }>("devices.register", { serial, sensorTypes, unitId }, { write: true }).then((d) => d.id));
}

export async function bindDevice(deviceId: string, version: number, unitId: string, reason: string) {
  return run(() => coreOp("devices.bind", { deviceId, unitId, reason }, { write: true, expectedVersion: version }).then(() => null));
}

export async function checkDevice(deviceId: string, version: number) {
  return run(() => coreOp("devices.check", { id: deviceId }, { write: true, expectedVersion: version }).then(() => null));
}

export async function calibrateSensor(deviceId: string, version: number, sensorId: string, metric: Metric, unit: string, referenceValue: number, measuredValue: number) {
  return run(async () => {
    const calibratedAt = (await coreNow()).toISOString();
    await coreOp("devices.calibrate", { deviceId, sensorId, metric, unit, referenceValue, measuredValue, calibratedAt }, { write: true, expectedVersion: version });
    return null;
  });
}

export async function updateFirmware(deviceId: string, version: number, firmwareVersion: string) {
  return run(() => coreOp("devices.updateFirmware", { deviceId, firmwareVersion }, { write: true, expectedVersion: version }).then(() => null));
}

/** firmwareCampaigns.schedule; the start is a Kuala Lumpur local time (datetime-local). */
export async function scheduleCampaign(d: CampaignDraft) {
  return run(() => coreOp<{ id: string }>("firmwareCampaigns.schedule", {
    modelId: d.modelId, targetVersion: d.targetVersion, deviceIds: d.deviceIds, waves: parseWaves(d.waves) ?? [], window: { startLocal: d.startLocal, endLocal: d.endLocal },
    autoPauseFailurePercent: Number(d.autoPause), startAt: new Date(`${d.startAt}:00+08:00`).toISOString(),
  }, { write: true }).then((c) => c.id));
}

export async function controlCampaign(campaignId: string, version: number, action: "pause" | "resume" | "abort" | "retry_device", deviceId?: string, reason?: string) {
  return run(() => coreOp("firmwareCampaigns.control", { campaignId, action, ...(deviceId ? { deviceId } : {}), ...(reason ? { reason } : {}) }, { write: true, expectedVersion: version }).then(() => null));
}
