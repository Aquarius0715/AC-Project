"use server";

// Server Actions of technician diagnostic control (FR-T10). Each one is a public endpoint: the DAL verifies the session
// and the Core API authorizes it (technician:control.diagnose on an assigned job inside its work window, IR94).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { UnitAction } from "@ac/web/lib/units";
import type { ApiCommandRow, ApiRun } from "@ac/web/lib/techControl";

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

/** commands.create for the job with the technician's reason; the device acknowledgement follows on the page. */
export async function sendDiagnostic(input: { unitId: string; jobId: string; action: UnitAction; reason: string; expectedUnitVersion: number }) {
  return run(() => coreOp<ApiCommandRow>("commands.create", { ...input, reason: input.reason.trim() }, { write: true }));
}
/** diagnosticRuns.create: the start command now, the end command at startedAt + duration (1–15 minutes). */
export async function startTestRun(input: { jobId: string; unitId: string; startAction: UnitAction; endAction: UnitAction; durationMinutes: number; reason: string; expectedUnitVersion: number; expectedJobVersion: number }) {
  return run(() => coreOp<ApiRun>("diagnosticRuns.create", { ...input, reason: input.reason.trim() }, { write: true }));
}
