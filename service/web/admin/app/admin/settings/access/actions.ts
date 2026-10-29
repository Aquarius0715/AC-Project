"use server";

// Server Action of the HQ access manager (FR-A03): members.save. A public endpoint: the DAL verifies the session and the
// Core API authorizes it (identity.write), rejects self-grants of identity.write / restriction.override and checks the
// membership version. Revoking access is a save with validUntil at the business clock when HQ confirms (IR492), never a delete. Signing a user
// out everywhere is members.revokeSessions: every session ends, the access stays (IR384, IR385).
import { refresh } from "next/cache";
import { coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

export async function saveMember(input: OpInput<"members.save">, version?: number): Promise<ActionResult<{ id: string; version: number }>> {
  try {
    const m = await coreOp<{ id: string; version: number }>("members.save", input, { write: true, expectedVersion: input.id ? version : undefined });
    return { ok: true, value: { id: m.id, version: m.version } };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {}, retryAfterSeconds: e.error.retryAfterSeconds };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}

/** Revoke access: members.save with the access ending now — the Core API's clock when HQ confirms, not the time the
 * page loaded, so the record says when the access really ended (IR492). */
export async function revokeMember(input: OpInput<"members.save">, version: number): Promise<ActionResult<{ id: string; version: number }>> {
  return saveMember({ ...input, validUntil: (await coreNow()).toISOString() }, version);
}

/** members.revokeSessions: ends every session of the membership's user in this tenant at once (Figma Admin 03-8). */
export async function signOutEverywhere(membershipId: string, reason: string): Promise<ActionResult> {
  try {
    await coreOp("members.revokeSessions", { membershipId, reason: reason.trim() }, { write: true });
    return { ok: true, value: null };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {}, retryAfterSeconds: e.error.retryAfterSeconds };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}
