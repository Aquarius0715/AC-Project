"use server";

// Server Action of the HQ access manager (FR-A03): members.save. A public endpoint: the DAL verifies the session and the
// Core API authorizes it (identity.write), rejects self-grants of identity.write / restriction.override and checks the
// membership version. Revoking access is a save with validUntil at the business clock when HQ confirms (IR492); an account made by mistake that never acted is deleted with members.delete (DEC-103). Signing a user
// out everywhere is members.revokeSessions: every session ends, the access stays (IR384, IR385).
import { refresh } from "next/cache";
import { coreNow, coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

/** The refusal a Core API error carries, or the unavailable answer for anything else. */
const failure = (error: unknown): ActionResult<never> => actionFailure(error);

export async function saveMember(input: OpInput<"members.save">, version?: number): Promise<ActionResult<{ id: string; version: number }>> {
  try {
    const m = await coreOp<{ id: string; version: number }>("members.save", input, { write: true, expectedVersion: input.id ? version : undefined });
    return { ok: true, value: { id: m.id, version: m.version } };
  } catch (error) {
    return failure(error);
  } finally {
    refresh();
  }
}

/** Revoke access: members.save with the access ending now — the Core API's clock when HQ confirms, not the time the
page loaded, so the record says when the access really ended (IR492). */
export async function revokeMember(input: OpInput<"members.save">, version: number): Promise<ActionResult<{ id: string; version: number }>> {
  const now = await coreNow();
  return saveMember({ ...input, validUntil: now.toISOString() }, version);
}

/** members.delete: an account that never acted, with its person and its provider account (DEC-103). `force` (the
data.forceDelete holder) also removes one that has history; the Core API refuses it without the permission. */
export async function deleteMember(id: string, version: number, reason: string, force = false): Promise<ActionResult> {
  try {
    await coreOp("members.delete", { id, force, reason: reason.trim() }, { write: true, expectedVersion: version });
    return { ok: true, value: null };
  } catch (error) {
    return actionFailure(error);
  } finally {
    refresh();
  }
}

/** members.revokeSessions: ends every session of the membership's user in this tenant at once (Figma Admin 03-8). */
export async function signOutEverywhere(membershipId: string, reason: string): Promise<ActionResult> {
  try {
    await coreOp("members.revokeSessions", { membershipId, reason: reason.trim() }, { write: true });
    return { ok: true, value: null };
  } catch (error) {
    return failure(error);
  } finally {
    refresh();
  }
}
