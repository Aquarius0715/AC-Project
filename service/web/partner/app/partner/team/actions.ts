"use server";

// Server Action of the team screen (FR-P team): members.setUnavailability for one technician or the whole company.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";

export async function setUnavailability(input: { membershipId: string | null; from: string; to: string; type: string; note?: string }): Promise<{ ok: true } | { ok: false; message: string }> {
  try {
    await coreOp("members.setUnavailability", input, { write: true });
    return { ok: true };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, message: Object.values(e.error.fieldErrors)[0] ?? e.error.messageKey };
    return { ok: false, message: "Not saved" };
  } finally {
    refresh();
  }
}
