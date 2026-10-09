"use server";

// Server Action of the contractor's job communication (FR-P07, DD-P07): save the note (jobs.addNote with the job version
// as the expected version) and render the notification preview for one recipient of the job (notifications.preview —
// nothing is stored or sent, deliveryState stays preview). Completed and cancelled jobs take no notes (IR123), so
// only the preview is made then. A refused note keeps the form; a refused preview reports that the note was saved.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

export type Communication = {
  jobId: string; version: number | null; saveNote: boolean; message: string; visibility: "internal" | "customer";
  templateKey: string; channel: string; recipientMembershipId: string;
};
type Preview = { id: string; templateKey: string; channel: string; recipientMembershipId: string; deliveryState: string; occurredAt: string };
export type CommunicationFailure = { ok: false; stage: "note" | "preview"; noteSaved: boolean } & ActionFailure;
type Failure = CommunicationFailure;
const failure = (e: CoreError, stage: Failure["stage"], noteSaved: boolean): Failure => ({ ok: false, stage, noteSaved, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors });

export async function communicate(c: Communication): Promise<{ ok: true; value: { noteSaved: boolean; preview: Preview } } | Failure> {
  const message = c.message.trim();
  let noteSaved = false;
  if (c.saveNote) {
    try {
      await coreOp("jobs.addNote", { jobId: c.jobId, message, visibility: c.visibility }, { write: true, expectedVersion: c.version ?? undefined });
      noteSaved = true;
    } catch (e) {
      if (!(e instanceof CoreError)) throw e;
      if (e.error.code === "CONFLICT") refresh(); // another version or a closed job: show the latest
      return failure(e, "note", false);
    }
  }
  try {
    const p = await coreOp<Preview>("notifications.preview", { target: { kind: "job", id: c.jobId }, templateKey: c.templateKey, channel: c.channel, recipientMembershipId: c.recipientMembershipId, message });
    if (noteSaved) refresh();
    return { ok: true, value: { noteSaved, preview: { id: p.id, templateKey: p.templateKey, channel: p.channel, recipientMembershipId: p.recipientMembershipId, deliveryState: p.deliveryState, occurredAt: p.occurredAt } } };
  } catch (e) {
    if (!(e instanceof CoreError)) throw e;
    if (noteSaved) refresh();
    return failure(e, "preview", noteSaved);
  }
}
