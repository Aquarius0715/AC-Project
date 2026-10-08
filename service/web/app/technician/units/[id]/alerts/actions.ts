"use server";

// Server Actions of the technician alert evidence screen (FR-T07): alerts.acknowledge / alerts.resolve with the
// alert version. The DAL verifies the session; the Core API applies technician:alert.resolve:assigned (IR94).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@/lib/dal";

export type ActionResult = { ok: true } | { ok: false; messageKey: string; code: string };

async function run(op: string, input: unknown, version: number): Promise<ActionResult> {
  try {
    await coreOp(op, input, { write: true, expectedVersion: version });
    return { ok: true };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE" };
  } finally {
    refresh();
  }
}

export async function acknowledgeAlert(alertId: string, version: number) {
  return run("alerts.acknowledge", { alertId }, version);
}

export async function resolveAlert(alertId: string, version: number, resolutionReason: string) {
  return run("alerts.resolve", { alertId, resolutionReason, resolutionEvidenceIds: [] }, version);
}
