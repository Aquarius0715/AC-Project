"use server";

// Server Actions of the MRV demo workspace (FR-A14). Each one is a public endpoint: the DAL verifies the session and the
// Core API authorizes the call (mrv.read, mrv.write, mrv.review, mrv.factors) and checks the expected version.
import { refresh } from "next/cache";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { ApiMRVPreview, MRVConditions } from "@ac/web/lib/mrv";
import type { OpInput } from "@ac/web/lib/opTypes";

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

/** mrv.preview: the result for the conditions with nothing stored (a read; the route is not re-rendered). */
export async function previewReport(conditions: MRVConditions) {
  return run(() => coreOp<ApiMRVPreview>("mrv.preview", conditions), false);
}

/** mrv.saveDraft: version 1 of a new report, or the next version of one (id + the version it was read at). */
export async function saveReportDraft(conditions: MRVConditions, evidenceIds: string[], id?: string, version?: number) {
  return run(async () => { const r = await coreOp<{ id: string; version: number }>("mrv.saveDraft", { ...(id && { id }), conditions, evidenceIds }, { write: true, expectedVersion: id ? version : undefined }); return ({ id: r.id, version: r.version }); });
}

/** mrv.recordReview of the latest version (a demo review, not external certification). */
export async function recordReview(reportId: string, version: number, reviewComment: string) {
  return run(async () => { const r = await coreOp<{ version: number }>("mrv.recordReview", { reportVersion: version, reportId, reviewComment }, { write: true, expectedVersion: version }); return r.version; });
}

/** mrv.deleteDraft: a report no review touched, every version (DEC-103). */
export async function deleteReport(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("mrv.deleteDraft", { id, reason }, { write: true, expectedVersion: version }); return null; });
}

/** factors.delete: a factor no MRV report cites and that is not the default, every version (DEC-103). */
export async function deleteFactor(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("factors.delete", { id, reason }, { write: true, expectedVersion: version }); return null; });
}

/** factors.save: a new factor or the next version (id + the version it was read at). */
export async function saveFactor(input: OpInput<"factors.save">, version?: number) {
  return run(async () => { const f = await coreOp<{ id: string; version: number }>("factors.save", input, { write: true, expectedVersion: input.id ? version : undefined }); return ({ id: f.id, version: f.version }); });
}
