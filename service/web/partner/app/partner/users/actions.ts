"use server";

// Server Actions of the partner Users page (FR-P11, IR416, IR461): the contractor adds contractors and technicians of
// its company (the account is created at once and gets a set-up e-mail), sets a technician's access end, resends the
// set-up e-mail, disables / enables and removes accounts. The Core API keeps every rule (own company, own account, last
// active contractor, fixed e-mail and role, an access end at most a year ahead).
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

/** Adds a user; a technician needs an access end (IR461). */
export async function invitePartnerUser(email: string, partnerRole: PartnerRole, displayName: string, accessUntil: string | null) {
  return run(() => coreOp("partnerUsers.save", { email: email.trim(), partnerRole, displayName: displayName.trim(), ...(accessUntil ? { accessUntil } : {}) }, { write: true }).then(() => null));
}
/** A technician's access end (its e-mail and role are sent unchanged). */
export async function setPartnerAccess(id: string, version: number, email: string, partnerRole: PartnerRole, accessUntil: string, reason: string) {
  return run(() => coreOp("partnerUsers.save", { id, email, partnerRole, accessUntil, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
/** Disable or enable an account (its e-mail and role are sent unchanged). */
export async function setPartnerUserStatus(id: string, version: number, email: string, partnerRole: PartnerRole, status: "active" | "disabled", reason: string) {
  return run(() => coreOp("partnerUsers.save", { id, email, partnerRole, status, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
export async function removePartnerUser(id: string, version: number, reason: string) {
  return run(() => coreOp("partnerUsers.remove", { id, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
/** Sends the set-up e-mail again (IR461). */
export async function resendPartnerInvite(id: string) {
  return run(() => coreOp("partnerUsers.resendInvite", { id }, { write: true }).then(() => null));
}
