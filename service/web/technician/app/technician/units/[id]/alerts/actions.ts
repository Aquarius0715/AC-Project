"use server";

// Server Actions of the technician alert evidence screen (FR-T07): alerts.acknowledge / alerts.resolve with the
// alert version. The DAL verifies the session; the Core API lets a technician on the unit acknowledge with alert.read and
// resolve with alert.resolve, inside the work window (IR94, IR284), citing candidates of alerts.evidence (IR327).
import { refresh } from "next/cache";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";

export type ActionResult = { ok: true } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

async function run(write: () => Promise<unknown>): Promise<ActionResult> {
  try {
    await write();
    return { ok: true };
  } catch (error) {
    return actionFailure(error);
  } finally {
    refresh();
  }
}

export async function acknowledgeAlert(alertId: string, version: number) {
  return run(() => coreOp("alerts.acknowledge", { alertId }, { write: true, expectedVersion: version }));
}

export async function resolveAlert(alertId: string, version: number, resolutionReason: string, resolutionEvidenceIds: string[]) {
  return run(() => coreOp("alerts.resolve", { alertId, resolutionReason, resolutionEvidenceIds }, { write: true, expectedVersion: version }));
}
