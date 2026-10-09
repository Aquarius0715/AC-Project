"use server";

// Server Action of the quality review (FR-P05): jobs.review on the submitted report version with the job version as
// the expected version (write-version catalog); the page re-reads after it.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const failed = (e: unknown): Result<never> => {
  if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
  throw e;
};

export async function reviewReport(jobId: string, jobVersion: number, reportVersion: number, decision: "accept" | "return", reason: string | null): Promise<Result<{ status: string }>> {
  try {
    const job = await coreOp<{ status: string }>("jobs.review", { jobId, reportVersion, decision, reviewMode: "normal", ...(reason !== null ? { reason } : {}) }, { write: true, expectedVersion: jobVersion });
    return { ok: true, value: { status: job.status } };
  } catch (e) {
    return failed(e);
  } finally {
    refresh();
  }
}
