"use server";

// Server Action of the team screen (FR-P06, DD-P06 Figma 04-8): members.setUnavailability for one technician or the
// whole company. Confirmed assignments inside the days are kept; the action returns how many the Core API found
// (conflictingAssignmentIds) so the confirmation can name them (IR276).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);

export async function setUnavailability(input: { membershipId: string | null; from: string; to: string; type: string; note?: string }): Promise<Result<{ conflicts: number }>> {
  try {
    const u = await coreOp<{ conflictingAssignmentIds?: string[] }>("members.setUnavailability", input, { write: true });
    refresh();
    return { ok: true, value: { conflicts: u.conflictingAssignmentIds?.length ?? 0 } };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    throw e;
  }
}
