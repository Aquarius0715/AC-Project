"use server";

// Server Actions of the HQ alerts screen (FR-A05). Each one is a public endpoint: the DAL verifies the session and the
// Core API authorizes the call (alert.resolve, alert.policy.write) and checks the expected version.
import { refresh } from "next/cache";
import { coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import { alertPolicyInput, defaultRulesInput, metricUnit, type AdminPolicy, type NotificationDecision } from "@ac/web/lib/adminAlerts";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult = { ok: true } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run(fn: () => Promise<unknown>): Promise<ActionResult> {
  try {
    await fn();
    return { ok: true };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}

export async function acknowledgeAlert(alertId: string, version: number): Promise<ActionResult> {
  return run(() => coreOp("alerts.acknowledge", { alertId }, { write: true, expectedVersion: version }));
}

/** Resolves with a reason and the picked evidence: candidates of alerts.evidence, at least one without a policy (IR327). */
export async function resolveAlert(alertId: string, version: number, reason: string, evidenceIds: string[]): Promise<ActionResult> {
  return run(() => coreOp("alerts.resolve", { alertId, resolutionReason: reason, resolutionEvidenceIds: evidenceIds }, { write: true, expectedVersion: version }));
}

/** Creates a new policy for its customer (no id, no expected version) or updates a saved one at its version; the
 * result names the saved policy so the editor can select a new one (IR345). */
export async function savePolicy(policy: AdminPolicy): Promise<ActionResult & { id?: string }> {
  let id: string | undefined;
  const r = await run(async () => {
    id = (await coreOp<{ id: string }>("policies.save", alertPolicyInput(policy), { write: true, ...(policy.id ? { expectedVersion: policy.version } : {}) })).id;
  });
  return r.ok ? { ...r, id } : r;
}

/** Deletes an alert policy: it is detached from every unit in the same change (IR108, IR120 item 6). */
export async function deletePolicy(policyId: string, version: number): Promise<ActionResult> {
  return run(() => coreOp("policies.delete", { policyId }, { write: true, expectedVersion: version }));
}

/** Saves the default policy's limits: all six rules at the policy's version; they change for every unit (IR120 item 2). */
export async function saveDefaultRules(policy: AdminPolicy): Promise<ActionResult> {
  return run(() => coreOp("policies.save", defaultRulesInput(policy), { write: true, expectedVersion: policy.version }));
}

/** Switches one default rule on or off for one customer with an optional reason (0–500), at the version of that
 * customer's setting (0 when the rule has none yet, IR120 item 7). */
export async function switchDefaultRule(policyId: string, ruleKey: string, customerId: string, enabled: boolean, reason: string, version: number): Promise<ActionResult> {
  return run(() => coreOp("policies.setDefaultRule", { policyId, ruleKey, customerId, enabled, ...(reason.trim() ? { reason: reason.trim() } : {}) }, { write: true, expectedVersion: version }));
}

/** “Test with demo data” (DD-A05 item 7, IR348): automations.simulate with one synthetic, valid reading of the policy's
 * metric on one of its units, at the Core API's clock. A read: nothing is stored and the route is not re-rendered. The
 * result is this policy's decision for the unit, or null when it did not run there. */
export async function testPolicy(policyId: string, unitId: string, metric: string, value: number): Promise<{ ok: true; decision: NotificationDecision | null } | Exclude<ActionResult, { ok: true }>> {
  try {
    const at = (await coreNow()).toISOString();
    const input: OpInput<"automations.simulate"> = {
      eventId: crypto.randomUUID(), occurredAt: at, phase: "condition", unitIds: [unitId],
      facts: [{ unitId, metric: metric as OpInput<"automations.simulate">["facts"][number]["metric"], value, unit: metricUnit[metric] ?? "", observedAt: at, quality: "valid" }],
    };
    const r = await coreOp<{ notifications: NotificationDecision[] }>("automations.simulate", input);
    return { ok: true, decision: r.notifications.find((n) => n.policyId === policyId && n.unitId === unitId) ?? null };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  }
}
