"use server";

// Server Actions of /customer/alerts (FR-C08, FR-C15): opening an alert marks the signed-in membership's notifications
// about it read (notifications.markRead with each row version; the alert itself is never changed, DD-C08); the Alert
// policies tab saves and deletes the customer's policies (policies.save / policies.delete with the policy version) and
// switches default rules for the customer (policies.setDefaultRule with the setting version, owner only, IR115).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { OpInput } from "@ac/web/lib/opTypes";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const run = async <T,>(fn: () => Promise<T>): Promise<Result<T>> => {
  try {
    const value = await fn();
    refresh();
    return { ok: true, value };
  } catch (e) {
    if (e instanceof CoreError) {
      if (e.error.code === "CONFLICT") refresh();
      return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    }
    throw e;
  }
};

/** Marks the alert's unread notifications read; returns an error message or null. */
export async function readAlert(notes: { id: string; version: number }[]): Promise<string | null> {
  try {
    for (const n of notes) await coreOp("notifications.markRead", { id: n.id }, { write: true, expectedVersion: n.version });
    return null;
  } catch (e) {
    return e instanceof CoreError ? `Not marked as read (${e.error.code}).` : "Not marked as read.";
  } finally {
    refresh();
  }
}

/** policies.save: a new policy, or an own policy at its version (the input of customerPolicies.policyInput). */
export async function savePolicy(input: OpInput<"policies.save">, version: number | null) {
  return run(async () => (await coreOp<{ id: string }>("policies.save", input, version === null ? { write: true } : { write: true, expectedVersion: version })).id);
}

/** policies.delete at the policy's version: detaches it from every AC and deletes it (IR108). */
export async function deletePolicy(policyId: string, version: number) {
  return run(async () => { await coreOp("policies.delete", { policyId }, { write: true, expectedVersion: version }); return null; });
}

/** policies.setDefaultRule for the customer at the setting's version (0 before the first change; owner only). */
export async function setDefaultRule(policyId: string, ruleKey: string, customerId: string, enabled: boolean, version: number) {
  return run(async () => { await coreOp("policies.setDefaultRule", { policyId, ruleKey, customerId, enabled }, { write: true, expectedVersion: version }); return null; });
}
