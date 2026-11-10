"use server";

// Server Action of the contractor's job communication (FR-P07, DD-P07): save the note (jobs.addNote with the job version
// as the expected version) and render the notification preview for one recipient of the job (notifications.preview —
// nothing is stored or sent, deliveryState stays preview). Completed and cancelled jobs take no notes (IR123), so
// only the preview is made then. A refused note keeps the form; a refused preview reports that the note was saved.
import { refresh } from "next/cache";
import { coreDisplay, coreOp, CoreError } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import { showTime } from "@ac/web/lib/i18n";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { OpInput } from "@ac/web/lib/opTypes";

export interface Communication {
  jobId: string; version: number | null; saveNote: boolean; message: string; visibility: "internal" | "customer";
  templateKey: OpInput<"notifications.preview">["templateKey"]; channel: OpInput<"notifications.preview">["channel"]; recipientMembershipId: string;
}
interface Preview { id: string; templateKey: string; channel: string; recipientMembershipId: string; deliveryState: string; occurredAt: string; at: string }
export type CommunicationFailure = { ok: false; stage: "note" | "preview"; noteSaved: boolean } & ActionFailure;
const failure = (e: unknown, stage: CommunicationFailure["stage"], noteSaved: boolean): CommunicationFailure => ({ ...actionFailure(e), stage, noteSaved });

export async function communicate(c: Communication): Promise<CommunicationFailure | { ok: true; value: { noteSaved: boolean; preview: Preview } }> {
  const message = c.message.trim();
  let isNoteSaved = false;
  if (c.saveNote) {
    try {
      await coreOp("jobs.addNote", { jobId: c.jobId, visibility: c.visibility, message }, { write: true, expectedVersion: c.version ?? undefined });
      isNoteSaved = true;
    } catch (error) {
      if (error instanceof CoreError && error.error.code === "CONFLICT") refresh(); // another version or a closed job: show the latest
      return failure(error, "note", false);
    }
  }
  try {
    const p = await coreOp<Omit<Preview, "at">>("notifications.preview", { target: { kind: "job", id: c.jobId }, templateKey: c.templateKey, channel: c.channel, recipientMembershipId: c.recipientMembershipId, message });
    if (isNoteSaved) refresh();
    const at = showTime(p.occurredAt, await coreDisplay()); // in the user's display time zone (IR278)
    return { ok: true, value: { noteSaved: isNoteSaved, preview: { id: p.id, templateKey: p.templateKey, channel: p.channel, recipientMembershipId: p.recipientMembershipId, deliveryState: p.deliveryState, occurredAt: p.occurredAt, at } } };
  } catch (error) {
    if (isNoteSaved) refresh();
    return failure(error, "preview", isNoteSaved);
  }
}
