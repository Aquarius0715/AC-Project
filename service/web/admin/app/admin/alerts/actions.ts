"use server";

// Server Actions of the HQ alerts screen (FR-A05). Each one is a public endpoint: the DAL verifies the session and the
// Core API authorizes the call (alert.resolve, alert.policy.write) and checks the expected version.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import { alertPolicyInput, type AdminPolicy } from "@ac/web/lib/adminAlerts";

export type ActionResult = { ok: true } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run(fn: () => Promise<unknown>): Promise<ActionResult> {
  try {
    await fn();
    return { ok: true };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}

export async function acknowledgeAlert(alertId: string, version: number): Promise<ActionResult> {
  return run(() => coreOp("alerts.acknowledge", { alertId }, { write: true, expectedVersion: version }));
}

export async function resolveAlert(alertId: string, version: number, reason: string): Promise<ActionResult> {
  return run(() => coreOp("alerts.resolve", { alertId, resolutionReason: reason, resolutionEvidenceIds: [] }, { write: true, expectedVersion: version }));
}

export async function savePolicy(policy: AdminPolicy): Promise<ActionResult> {
  return run(() => coreOp("policies.save", alertPolicyInput(policy), { write: true, expectedVersion: policy.version }));
}
