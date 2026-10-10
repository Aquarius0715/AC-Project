"use server";

// Server Action of the contractor's schedule (FR-P03, DD-P03): jobs.assign with the job version as the expected version
// (write-version catalog: target job). The slot is the agreed visit time, the current slot, or — while the work is
// under way — the same start with a later end; the reason is required then (IR123 item 4). Membership, scope,
// qualification and overlaps are rechecked by the Core API on save. The saved slot comes back in the user's display
// language and time zone for the confirmation (IR275).
import { refresh } from "next/cache";
import { coreDisplay, coreOp, CoreError } from "@ac/web/lib/dal";
import { showSpan } from "@ac/web/lib/i18n";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);

/** jobs.assign: assign, reassign or extend; refreshes the page on success and on CONFLICT. */
export async function assignTechnician(jobId: string, jobVersion: number, technicianMembershipId: string, slot: { startAt: string; endAt: string }, reason: string): Promise<Result<{ slot: string }>> {
  try {
    await coreOp("jobs.assign", { jobId, technicianMembershipId, startAt: slot.startAt, endAt: slot.endAt, ...(reason.trim() ? { reason: reason.trim() } : {}) }, { write: true, expectedVersion: jobVersion });
    refresh();
    return { ok: true, value: { slot: showSpan(slot.startAt, slot.endAt, await coreDisplay()) } };
  } catch (e) {
    if (e instanceof CoreError) {
      if (e.error.code === "CONFLICT") refresh(); // overlap, changed state or another version: show the latest
      return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    }
    throw e;
  }
}
