"use server";

// Server Action of the HQ access manager (FR-A03): members.save. A public endpoint: the DAL verifies the session and the
// Core API authorizes it (identity.write), rejects self-grants of identity.write / restriction.override and checks the
// membership version. Revoking access is a save with validUntil at the business clock, never a delete.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

export async function saveMember(input: Record<string, unknown> & { id?: string }, version?: number): Promise<ActionResult<{ id: string; version: number }>> {
  try {
    const m = await coreOp<{ id: string; version: number }>("members.save", input, { write: true, expectedVersion: input.id ? version : undefined });
    return { ok: true, value: { id: m.id, version: m.version } };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}
