"use server";

// Server Actions of the customer Users page (FR-C19): an owner invites members and resends pending invites. The Core
// API refuses anything else from a client session (owner role, saves with id, removal: FORBIDDEN, IR114).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run<T>(fn: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}

export async function inviteMember(customerId: string, email: string) {
  return run(() => coreOp<{ id: string }>("clientUsers.save", { customerId, email: email.trim(), clientRole: "member" }, { write: true }).then((u) => u.id));
}
/** Builds the invite preview again; nothing is sent in the demo. */
export async function resendInvite(id: string) {
  return run(() => coreOp("clientUsers.resendInvite", { id }).then(() => null));
}
