"use server";

// Server Actions of /customer/maintenance (FR-C09, FR-C17): a new request with 3 preferred times (jobs.create), the
// answer to a proposed time (jobs.respondProposal), another time for a plan visit (jobs.requestReschedule), cancel a
// request (jobs.cancel), a note for the coordinator (jobs.addNote, customer visibility), Confirm & rate (jobs.rate) and
// Report a problem with photos (jobs.reportProblem → a follow-up request); on Filter care (FR-C18) Mark cleaned
// (filterCare.markCleaned) and the owner's reminder settings (filterCare.saveSettings). Writes on a request carry its
// version as the expected version; CONFLICT refreshes the page.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Slot = { startAt: string; endAt: string };
type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const run = async <T,>(fn: () => Promise<T>): Promise<Result<T>> => {
  try {
    const value = await fn();
    refresh();
    return { ok: true, value };
  } catch (e) {
    if (e instanceof CoreError) {
      if (e.error.code === "CONFLICT") refresh();
      return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    }
    throw e;
  }
};
const write = (version: number) => ({ write: true, expectedVersion: version });

/** jobs.create: the 1st preferred time is the requested window, the 2nd and 3rd the alternatives (IR113). */
export async function createRequest(input: { unitId: string; type: "reactive" | "preventive"; symptom: string; slots: Slot[]; contactWindow: string }) {
  return run(async () => {
    const [first, ...alternativeSlots] = input.slots;
    const j = await coreOp<{ id: string }>("jobs.create", {
      unitId: input.unitId, type: input.type, symptom: input.symptom.trim(), requestedStart: first.startAt, requestedEnd: first.endAt, alternativeSlots,
      ...(input.contactWindow.trim() ? { contactWindow: input.contactWindow.trim() } : {}),
    }, { write: true });
    return { id: j.id };
  });
}

/** jobs.respondProposal: accept books the time; decline gives a reason, an optional comment and 0 or 3 new times. */
export async function respondProposal(jobId: string, version: number, proposalId: string, answer: { decision: "accept" } | { decision: "decline"; declineReason: "not_home" | "too_late" | "other"; comment: string; slots: Slot[] }) {
  return run(async () => {
    await coreOp("jobs.respondProposal", answer.decision === "accept" ? { jobId, proposalId, decision: "accept" }
      : { jobId, proposalId, decision: "decline", declineReason: answer.declineReason, ...(answer.comment.trim() ? { comment: answer.comment.trim() } : {}), ...(answer.slots.length ? { preferredSlots: answer.slots } : {}) }, write(version));
    return null;
  });
}

/** jobs.requestReschedule: 3 preferred times for a plan visit, at least 48 h before it. */
export async function requestReschedule(jobId: string, version: number, slots: Slot[], comment: string) {
  return run(async () => { await coreOp("jobs.requestReschedule", { jobId, preferredSlots: slots, ...(comment.trim() ? { comment: comment.trim() } : {}) }, write(version)); return null; });
}

/** jobs.cancel: a request that has no booked time yet, with a reason. */
export async function cancelRequest(jobId: string, version: number, reason: string) {
  return run(async () => { await coreOp("jobs.cancel", { jobId, cancelReason: reason.trim() }, write(version)); return null; });
}

/** jobs.addNote: a note for the coordinator (customer visibility; it never changes the time or the technician). */
export async function addNote(jobId: string, version: number, message: string) {
  return run(async () => { await coreOp("jobs.addNote", { jobId, message: message.trim(), visibility: "customer" }, write(version)); return null; });
}

/** jobs.rate: 1–5 stars, tags and an optional comment; editable for 7 days. */
export async function rateJob(jobId: string, version: number, stars: number, tags: string[], comment: string) {
  return run(async () => { await coreOp("jobs.rate", { jobId, stars, tags, ...(comment.trim() ? { comment: comment.trim() } : {}) }, write(version)); return null; });
}

/** jobs.reportProblem: a reason, details 10–2000, up to 5 JPEG/PNG photos and an optional preferred visit; returns the
 * follow-up request's id. The photos arrive as files in the form data. */
export async function reportProblem(form: FormData) {
  return run(async () => {
    const photos = await Promise.all(form.getAll("photos").filter((f): f is File => f instanceof File && f.size > 0).map(async (f) => ({
      name: f.name, mime: f.type, size: f.size, bytes: Buffer.from(await f.arrayBuffer()).toString("base64"),
    })));
    const start = String(form.get("visitStart") ?? ""), end = String(form.get("visitEnd") ?? "");
    const j = await coreOp<{ id: string }>("jobs.reportProblem", {
      jobId: String(form.get("jobId")), reasonCode: String(form.get("reasonCode")), details: String(form.get("details") ?? "").trim(), photos,
      preferredSlot: start && end ? { startAt: start, endAt: end } : null,
    }, write(Number(form.get("version"))));
    return { id: j.id };
  });
}

/** filterCare.markCleaned: the customer cleaned the filter; the counter restarts (lastCleanedBy customer, shown to the technician). */
export async function markCleaned(unitId: string) {
  return run(async () => { await coreOp("filterCare.markCleaned", { unitId }, { write: true }); return null; });
}

/** filterCare.saveSettings (owner only): remind at the model default or 50–2000 h, every 7–180 days when run time is
 * unknown, to the owners or all users, in the app and optionally by e-mail. */
export async function saveReminders(input: { thresholdHours: number | null; fallbackDays: number; recipients: "owners" | "all_users"; channels: ("inApp" | "email")[] }) {
  return run(async () => { await coreOp("filterCare.saveSettings", input, { write: true }); return null; });
}
