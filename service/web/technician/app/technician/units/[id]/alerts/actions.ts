"use server";

// Server Actions of the technician alert evidence screen (FR-T07): alerts.acknowledge / alerts.resolve with the
// alert version. The DAL verifies the session; the Core API lets a technician on the unit acknowledge with alert.read and
// resolve with alert.resolve, inside the work window (IR94, IR284).
import { refresh } from "next/cache";
import { coreOp, type CoreOptions, CoreError } from "@ac/web/lib/dal";
import type { OpCall } from "@ac/web/lib/opTypes";
import type { OpArgs } from "@ac/web/lib/opTypes";

export type ActionResult = { ok: true } | { ok: false; messageKey: string; code: string };

async function run(call: OpCall, version: number): Promise<ActionResult> {
  try {
    await coreOp(...([...call, { write: true, expectedVersion: version }] as OpArgs<CoreOptions>));
    return { ok: true };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE" };
  } finally {
    refresh();
  }
}

export async function acknowledgeAlert(alertId: string, version: number) {
  return run(["alerts.acknowledge", { alertId }], version);
}

export async function resolveAlert(alertId: string, version: number, resolutionReason: string) {
  return run(["alerts.resolve", { alertId, resolutionReason, resolutionEvidenceIds: [] }], version);
}
