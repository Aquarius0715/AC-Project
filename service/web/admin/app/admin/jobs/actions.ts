"use server";

// Server Actions of the HQ Jobs tab (FR-A06, DD-A06): book an agreed time internally (jobs.assign) or as an offer to a
// contractor (jobs.offer with the time locked, IR113), propose another time to the client and withdraw it, answer a
// contractor's time change (jobs.resolvePartnerSlot), hold / resume / cancel (IR56), reassign an internal job, classify
// a follow-up (IR114), review the submitted report (jobs.review), save the cost lines (jobs.saveCost), extend a
// contractor's access (jobs.extendAccess) and create a job on a customer's behalf (jobs.create); on the Plans tab save a
// plan (plans.save) and generate its next occurrence (plans.generateNext, D16). Each write on a job or plan carries its
// version as the expected version; CONFLICT refreshes the page.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { CostLine } from "@ac/web/lib/adminJobs";

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

/** jobs.review on the latest submitted report: normal for an internal job, hq_escalation with a reason for a
 * contractor's job (IR31, D06); a reviewer who contributed to the version is refused (no self-review). */
export async function reviewReport(jobId: string, version: number, reportVersion: number, decision: "accept" | "return", reviewMode: "normal" | "hq_escalation", reason: string) {
  return run(() => coreOp("jobs.review", { jobId, reportVersion, decision, reviewMode, ...(reason.trim() ? { reason: reason.trim() } : {}) }, write(version)));
}

/** jobs.saveCost: the whole list of cost lines replaces the stored one (estimate / actual, per currency). */
export async function saveCosts(jobId: string, version: number, costLines: CostLine[]) {
  return run(() => coreOp("jobs.saveCost", { jobId, costLines }, write(version)));
}

/** jobs.extendAccess: a later end of the accepted offer's access window, with a reason (1–1000). */
export async function extendAccess(jobId: string, version: number, accessValidUntil: string, reason: string) {
  return run(() => coreOp("jobs.extendAccess", { jobId, accessValidUntil, reason: reason.trim() }, write(version)));
}

/** jobs.create on a customer's behalf (DD-A06 item 8): the 1st preferred time is the requested window, up to two
 * more, due at or after the requested end, an optional contact window. Returns the new job's id. */
export async function createJob(input: { unitId: string; type: string; symptom: string; slots: Slot[]; dueAt: string | null; contactWindow: string }): Promise<{ ok: true; value: { id: string } } | ({ ok: false } & ActionFailure)> {
  const [first, ...alternatives] = input.slots;
  try {
    const job = await coreOp<{ id: string }>("jobs.create", {
      unitId: input.unitId, type: input.type, symptom: input.symptom.trim(), requestedStart: first.startAt, requestedEnd: first.endAt, alternativeSlots: alternatives,
      ...(input.dueAt ? { dueAt: input.dueAt } : {}), ...(input.contactWindow.trim() ? { contactWindow: input.contactWindow.trim() } : {}),
    }, { write: true });
    return { ok: true, value: { id: job.id } };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    throw e;
  }
}

type Failure = { ok: false } & ActionFailure;
const failure = (e: unknown, conflictRefresh = true): Failure => {
  if (e instanceof CoreError) {
    if (conflictRefresh && e.error.code === "CONFLICT") refresh();
    return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
  }
  throw e;
};

/** plans.save: a new plan for a unit, or the plan's recurrence and next date (monthly, every 1–12 months, the next date in
 * the future, stored in UTC with its UTC day as the anchor day). Returns the plan id. */
export async function savePlan(input: { id: string | null; version: number | null; unitId: string; intervalMonths: number; nextDueAt: string }): Promise<{ ok: true; value: { id: string } } | Failure> {
  try {
    const p = await coreOp<{ id: string }>("plans.save", { ...(input.id ? { id: input.id } : {}), unitId: input.unitId, recurrence: { kind: "monthly", intervalMonths: input.intervalMonths }, nextDueAt: input.nextDueAt },
      input.id ? write(input.version!) : { write: true });
    refresh();
    return { ok: true, value: { id: p.id } };
  } catch (e) {
    return failure(e);
  }
}

/** plans.generateNext: one periodic job (requested) for the saved next date; a date that already has its job, or a plan
 * that changed meanwhile, is CONFLICT and the page shows the latest plan. Returns the job id. */
export async function generateJob(planId: string, version: number, occurrenceDate: string): Promise<{ ok: true; value: { id: string } } | Failure> {
  try {
    const j = await coreOp<{ id: string }>("plans.generateNext", { id: planId, occurrenceDate }, write(version));
    refresh();
    return { ok: true, value: { id: j.id } };
  } catch (e) {
    return failure(e);
  }
}

