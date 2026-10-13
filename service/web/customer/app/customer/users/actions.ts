"use server";

// Server Actions of the customer Users page (FR-C19, IR417): an owner invites members and owners, changes a pending
// invite, a user's role and access, removes users and resends invites. The Core API keeps every rule (own customer,
// own account, last active owner, fixed e-mail once accepted).
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

export async function inviteUser(customerId: string, email: string, clientRole: "owner" | "member") {
  return run(() => coreOp<{ id: string }>("clientUsers.save", { customerId, email: email.trim(), clientRole }, { write: true }).then(() => null));
}
/** A pending invite's e-mail and role, or a user's role and access (status), with a reason. */
export async function updateUser(id: string, version: number, customerId: string, email: string, clientRole: "owner" | "member", status: "active" | "disabled" | null, reason: string) {
  return run(() => coreOp("clientUsers.save", { id, customerId, email: email.trim(), clientRole, ...(status ? { status } : {}), reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
export async function removeUser(id: string, version: number, reason: string) {
  return run(() => coreOp("clientUsers.remove", { id, reason: reason.trim() }, { write: true, expectedVersion: version }).then(() => null));
}
/** Builds the invite preview again; nothing is sent in the demo. */
export async function resendInvite(id: string) {
  return run(() => coreOp("clientUsers.resendInvite", { id }).then(() => null));
}
