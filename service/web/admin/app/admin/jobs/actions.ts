"use server";

// Server Actions of the HQ Jobs tab (FR-A06, DD-A06): book an agreed time internally (jobs.assign) or as an offer to a
// contractor (jobs.offer with the time locked, IR113), propose another time to the client and withdraw it, answer a
// contractor's time change (jobs.resolvePartnerSlot), hold / resume / cancel (IR56), reassign an internal job and
// classify a follow-up (IR114). Each carries the job version as the expected version; CONFLICT refreshes the page.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Slot = { startAt: string; endAt: string };
type Result = { ok: true; value: null } | ({ ok: false } & ActionFailure);
const run = async (fn: () => Promise<unknown>): Promise<Result> => {
  try {
    await fn();
    refresh();
    return { ok: true, value: null };
  } catch (e) {
    if (e instanceof CoreError) {
      if (e.error.code === "CONFLICT") refresh();
      return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    }
    throw e;
  }
};
const write = (version: number) => ({ write: true, expectedVersion: version });

/** jobs.assign: an internal technician for the agreed time (also a reassignment at the same slot with a reason). */
export async function assignInternal(jobId: string, version: number, technicianMembershipId: string, slot: Slot, reason?: string) {
  return run(() => coreOp("jobs.assign", { jobId, technicianMembershipId, startAt: slot.startAt, endAt: slot.endAt, ...(reason?.trim() ? { reason: reason.trim() } : {}) }, write(version)));
}

/** jobs.offer: the agreed time offered to a contractor with the offer and access windows and the terms version. */
export async function offerToContractor(jobId: string, version: number, input: { contractorOrgId: string; visitSlot: Slot; offerExpiresAt: string; accessValidFrom: string; accessValidUntil: string; termsVersion: string }) {
  return run(() => coreOp("jobs.offer", { jobId, ...input }, write(version)));
}

/** jobs.proposeSlot: one other time with the held capacity, a message (1–1000) and a reply deadline (≤ 7 days). */
export async function proposeSlot(jobId: string, version: number, slot: Slot, hold: { kind: "internal"; membershipId: string } | { kind: "contractor"; contractorOrgId: string; technicianMembershipId: null }, message: string, replyBy: string) {
  return run(() => coreOp("jobs.proposeSlot", { jobId, slot, hold, message: message.trim(), replyBy }, write(version)));
}

/** jobs.withdrawProposal: take the pending proposal back. */
export async function withdrawProposal(jobId: string, version: number, proposalId: string) {
  return run(() => coreOp("jobs.withdrawProposal", { jobId, proposalId }, write(version)));
}

/** jobs.resolvePartnerSlot: send the contractor's time to the client (reply deadline) or keep the agreed time. */
export async function resolvePartnerSlot(jobId: string, version: number, proposalId: string, decision: "send_to_client" | "keep", replyBy?: string) {
  return run(() => coreOp("jobs.resolvePartnerSlot", { jobId, proposalId, decision, ...(decision === "send_to_client" ? { replyBy } : {}) }, write(version)));
}

/** jobs.hold / jobs.resumeHold / jobs.cancel with their reason (1–1000). */
export async function holdJob(jobId: string, version: number, reason: string) { return run(() => coreOp("jobs.hold", { jobId, reason: reason.trim() }, write(version))); }
export async function resumeJob(jobId: string, version: number, reason: string) { return run(() => coreOp("jobs.resumeHold", { jobId, reason: reason.trim() }, write(version))); }
export async function cancelJob(jobId: string, version: number, cancelReason: string) { return run(() => coreOp("jobs.cancel", { jobId, cancelReason: cancelReason.trim() }, write(version))); }

/** jobs.classifyFollowUp: rework (free) or a new request, once, with a reason. */
export async function classifyFollowUp(jobId: string, version: number, classification: "rework" | "new_request", reason: string) {
  return run(() => coreOp("jobs.classifyFollowUp", { jobId, classification, reason: reason.trim() }, write(version)));
}
