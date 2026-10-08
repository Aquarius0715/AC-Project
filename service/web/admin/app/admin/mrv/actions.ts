"use server";

// Server Actions of the MRV demo workspace (FR-A14). Each one is a public endpoint: the DAL verifies the session and the
// Core API authorizes the call (mrv.read, mrv.write, mrv.review, mrv.factors) and checks the expected version.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiMRVPreview, MRVConditions } from "@ac/web/lib/mrv";

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

/** mrv.preview: the result for the conditions with nothing stored (a read; the route is not re-rendered). */
export async function previewReport(conditions: MRVConditions) {
  return run(() => coreOp<ApiMRVPreview>("mrv.preview", conditions), false);
}

/** mrv.saveDraft: version 1 of a new report, or the next version of one (id + the version it was read at). */
export async function saveReportDraft(conditions: MRVConditions, evidenceIds: string[], id?: string, version?: number) {
  return run(() => coreOp<{ id: string; version: number }>("mrv.saveDraft", { ...(id ? { id } : {}), conditions, evidenceIds }, { write: true, expectedVersion: id ? version : undefined })
    .then((r) => ({ id: r.id, version: r.version })));
}

/** mrv.recordReview of the latest version (a demo review, not external certification). */
export async function recordReview(reportId: string, version: number, reviewComment: string) {
  return run(() => coreOp<{ version: number }>("mrv.recordReview", { reportId, reportVersion: version, reviewComment }, { write: true, expectedVersion: version }).then((r) => r.version));
}

/** factors.save: a new factor or the next version (id + the version it was read at). */
export async function saveFactor(input: Record<string, unknown> & { id?: string }, version?: number) {
  return run(() => coreOp<{ id: string; version: number }>("factors.save", input, { write: true, expectedVersion: input.id ? version : undefined }).then((f) => ({ id: f.id, version: f.version })));
}
