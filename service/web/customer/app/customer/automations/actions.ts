"use server";

// Server Actions of the customer automations page (FR-C04, FR-C05). Each one is a public endpoint: the DAL verifies
// the session and the Core API authorizes it (automations.*: client:control.execute:self; consents.*: own membership).
import { refresh } from "next/cache";
import { coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiAutomation, ApiConsent, ApiDecision } from "@ac/web/lib/clientAutomations";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run<T>(fn: () => Promise<T>, write = true): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    if (write) refresh();
  }
}

/** automations.save (create without a version, update with the rule's version); saving sends no Command (AT-C04-N ②). */
export async function saveAutomation(input: OpInput<"automations.save">, version: number | null) {
  return run(() => coreOp<ApiAutomation>("automations.save", input, { write: true, ...(version !== null ? { expectedVersion: version } : {}) }));
}
/** automations.delete: the rule stops; its Commands and run log stay (IR214). */
export async function deleteAutomation(id: string, version: number) {
  return run(() => coreOp<{ id: string; deleted: true }>("automations.delete", { id }, { write: true, expectedVersion: version }));
}
/** consents.update (location_automation): withdrawing disables the location rules (IR53); granting re-enables none. */
export async function updateConsent(granted: boolean, version: number) {
  return run(() => coreOp<ApiConsent>("consents.update", { purpose: "location_automation", granted }, { write: true, expectedVersion: version }));
}
/** automations.simulate of the AC for a synthetic event at the current tick of the business clock (IR21): the saved
 * rules decide, nothing is stored or sent. */
export async function testEvent(unitId: string, facts: Omit<OpInput<"automations.simulate">["facts"][number], "unitId" | "observedAt">[]) {
  return run(async () => {
    const at = new Date(Math.floor((await coreNow()).getTime() / 60_000) * 60_000).toISOString();
    const r = await coreOp<{ results: ApiDecision[] }>("automations.simulate", {
      eventId: crypto.randomUUID(), occurredAt: at, unitIds: [unitId], phase: "condition", facts: facts.map((f) => ({ ...f, unitId, observedAt: at })),
    });
    return { at, decision: r.results.find((d) => d.unitId === unitId) ?? null };
  }, false);
}
