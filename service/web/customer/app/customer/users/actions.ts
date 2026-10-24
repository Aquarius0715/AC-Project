"use server";

// Server Actions of the customer Users page (FR-C19, IR417, IR461): an owner adds members and owners (the account is
// created at once and gets a set-up e-mail), changes a user's role and access, removes users and resends the set-up
// e-mail. The Core API keeps every rule (own customer, own account, last active owner, fixed e-mail).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";

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

export async function inviteUser(customerId: string, email: string, clientRole: "owner" | "member", displayName: string) {
  return run(() => coreOp<{ id: string }>("clientUsers.save", { customerId, email: email.trim(), clientRole, displayName: displayName.trim() }, { write: true }).then(() => null));
}
/** A user's role and access (status), with a reason; the e-mail is sent unchanged. */
export async function updateUser(id: string, version: number, customerId: string, email: string, clientRole: "owner" | "member", status: "active" | "disabled" | null, reason: string) {
  return run(() => coreOp("clientUsers.save", { id, customerId, email: email.trim(), clientRole, ...(status ? { status } : {}), reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
export async function removeUser(id: string, version: number, reason: string) {
  return run(() => coreOp("clientUsers.remove", { id, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
/** Sends the set-up e-mail again (IR461). */
export async function resendInvite(id: string) {
  return run(() => coreOp("clientUsers.resendInvite", { id }, { write: true }).then(() => null));
}
