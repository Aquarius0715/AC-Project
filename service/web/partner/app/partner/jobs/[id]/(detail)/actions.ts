"use server";

// Server Actions of the contractor's offer (FR-P02, DD-P02): accept, decline with a reason, propose another time and
// withdraw the proposal. Each carries the job version as the expected version (write-version catalog: target job).
// A decline takes the job away from the company, so it does not refresh (the page would be not found); the others do.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const run = async <T,>(fn: () => Promise<T>, reload: boolean): Promise<Result<T>> => {
  try {
    const value = await fn();
    if (reload) refresh();
    return { ok: true, value };
  } catch (e) {
    if (e instanceof CoreError) {
      if (e.error.code === "CONFLICT") refresh(); // expired, cancelled or changed elsewhere: show the latest state
      return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    }
    throw e;
  }
};

/** jobs.accept: the company takes the job at the fixed visit time (acceptance does not assign a technician). */
export async function acceptOffer(jobId: string, offerId: string, termsVersion: string, jobVersion: number) {
  return run(() => coreOp("jobs.accept", { jobId, offerId, termsVersion }, { write: true, expectedVersion: jobVersion }), true);
}

/** jobs.decline: the job returns to HQ with the reason (1–1000 characters). */
export async function declineOffer(jobId: string, offerId: string, jobVersion: number, reason: string) {
  return run(() => coreOp("jobs.decline", { jobId, offerId, reason }, { write: true, expectedVersion: jobVersion }), false);
}

/** jobs.proposePartnerSlot: another visit time with an own qualified technician and a reason; HQ asks the client. */
export async function proposeSlot(jobId: string, offerId: string, jobVersion: number, slot: { startAt: string; endAt: string }, technicianMembershipId: string, reason: string) {
  return run(() => coreOp("jobs.proposePartnerSlot", { jobId, offerId, slot, technicianMembershipId, reason }, { write: true, expectedVersion: jobVersion }), true);
}

/** jobs.withdrawPartnerSlot: take the pending time change back; the offer stays as it was. */
export async function withdrawProposal(jobId: string, proposalId: string, jobVersion: number) {
  return run(() => coreOp("jobs.withdrawPartnerSlot", { jobId, proposalId }, { write: true, expectedVersion: jobVersion }), true);
}
