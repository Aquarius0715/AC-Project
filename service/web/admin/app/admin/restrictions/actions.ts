"use server";

// Server Actions of the HQ restriction screens (FR-A09, FR-A10). Each one is a public endpoint: the DAL verifies the
// session and the Core API authorizes it (restriction.write, restriction.override) and checks the restriction version
// (the contract version for schedule).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { Policy } from "@ac/web/lib/restrictions";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

async function run<T>(fn: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (error) {
    return error instanceof CoreError ? { ok: false, messageKey: error.error.messageKey, code: error.error.code, fieldErrors: error.error.fieldErrors ?? {}, retryAfterSeconds: error.error.retryAfterSeconds } : { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}
const write = (version: number) => ({ write: true, expectedVersion: version });

export interface ScheduleInput { contractId: string; expectedContractVersion: number; causeInvoiceIds: string[]; unitIds: string[]; policy: Policy; executeAfter: string; reason: string; rulesVersion: string }
/** restrictions.schedule (noticeAt is the acceptance time; the notice goes to the customer's clients, IR05). */
export async function scheduleRestriction(input: ScheduleInput) {
  return run(async () => { const r = await coreOp<{ id: string }>("restrictions.schedule", input, { write: true }); return r.id; });
}
export async function executeRestriction(id: string, version: number, confirmedRulesVersion: string) {
  return run(async () => { await coreOp("restrictions.execute", { restrictionId: id, confirmedRulesVersion }, write(version)); return null; });
}
export async function requestRelease(id: string, version: number) {
  return run(async () => { await coreOp("restrictions.release", { restrictionId: id }, write(version)); return null; });
}
export async function retryUnits(id: string, version: number, unitIds: string[], phase: "apply" | "release", confirmedRulesVersion: string, reason: string) {
  return run(async () => { await coreOp("restrictions.retry", { restrictionId: id, unitIds, phase, confirmedRulesVersion, reason }, write(version)); return null; });
}
export async function reconcileUnits(id: string, version: number, unitIds: string[]) {
  return run(async () => { await coreOp("restrictions.reconcile", { restrictionId: id, unitIds }, write(version)); return null; });
}
export async function deferRestriction(id: string, version: number, until: string, reason: string) {
  return run(async () => { await coreOp("restrictions.defer", { restrictionId: id, until, reason }, write(version)); return null; });
}
export async function exemptRestriction(id: string, version: number, until: string, reason: string) {
  return run(async () => { await coreOp("restrictions.exempt", { restrictionId: id, until, reason }, write(version)); return null; });
}
export async function cancelRestriction(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("restrictions.cancel", { restrictionId: id, reason }, write(version)); return null; });
}
export async function overrideRestriction(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("restrictions.override", { restrictionId: id, reason }, write(version)); return null; });
}
