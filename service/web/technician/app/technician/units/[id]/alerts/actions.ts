"use server";

// Server Actions of the technician alert evidence screen (FR-T07): alerts.acknowledge / alerts.resolve with the
// alert version. The DAL verifies the session; the Core API lets a technician on the unit acknowledge with alert.read and
// resolve with alert.resolve, inside the work window (IR94, IR284), citing candidates of alerts.evidence (IR327).
import { refresh } from "next/cache";
import { coreOp, type CoreOptions } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { OpCall , OpArgs } from "@ac/web/lib/opTypes";

export type ActionResult = { ok: true } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

async function run(call: OpCall, version: number): Promise<ActionResult> {
  try {
    await coreOp(...([...call, { write: true, expectedVersion: version }] as OpArgs<CoreOptions>));
    return { ok: true };
  } catch (error) {
    return actionFailure(error);
  } finally {
    refresh();
  }
}

export async function acknowledgeAlert(alertId: string, version: number) {
  return run(["alerts.acknowledge", { alertId }], version);
}

export async function resolveAlert(alertId: string, version: number, resolutionReason: string, resolutionEvidenceIds: string[]) {
  return run(["alerts.resolve", { alertId, resolutionReason, resolutionEvidenceIds }], version);
}
