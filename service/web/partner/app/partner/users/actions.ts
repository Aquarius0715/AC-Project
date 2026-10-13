"use server";

// Server Actions of the partner Users page (FR-P11, IR416): the contractor invites contractors and technicians of its
// company, changes or resends a pending invite, disables / enables and removes accounts. The Core API keeps every
// rule (own company, own account, last active contractor, fixed e-mail and role once accepted).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { PartnerRole } from "@ac/web/lib/partnerUsers";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

async function run<T>(fn: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {}, retryAfterSeconds: e.error.retryAfterSeconds };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}

export async function invitePartnerUser(email: string, partnerRole: PartnerRole) {
  return run(() => coreOp("partnerUsers.save", { email: email.trim(), partnerRole }, { write: true }).then(() => null));
}
/** A pending invite's e-mail and role. */
export async function editPartnerInvite(id: string, version: number, email: string, partnerRole: PartnerRole, reason: string) {
  return run(() => coreOp("partnerUsers.save", { id, email: email.trim(), partnerRole, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
/** Disable or enable an account (its e-mail and role are sent unchanged). */
export async function setPartnerUserStatus(id: string, version: number, email: string, partnerRole: PartnerRole, status: "active" | "disabled", reason: string) {
  return run(() => coreOp("partnerUsers.save", { id, email, partnerRole, status, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
export async function removePartnerUser(id: string, version: number, reason: string) {
  return run(() => coreOp("partnerUsers.remove", { id, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
/** Builds the invite preview again; nothing is sent in the demo. */
export async function resendPartnerInvite(id: string) {
  return run(() => coreOp("partnerUsers.resendInvite", { id }).then(() => null));
}
