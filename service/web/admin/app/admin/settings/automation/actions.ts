"use server";

// Server Actions of the HQ automation policies (FR-A11): policies.save (kind automation), automations.simulate (a read,
// nothing stored) and automations.fire (demo-only, one-time key). Public endpoints: the DAL verifies the session and
// the Core API authorizes them (automation.policy.write) and checks the policy version.
import { refresh } from "next/cache";
import { coreNow, coreOp, CoreError } from "@ac/web/lib/dal";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };
type Decision = { unitId: string; decision: string; ruleId: string | null; reason: string | null; commandId?: string | null };

async function run<T>(fn: () => Promise<T>, rerender = true): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {}, retryAfterSeconds: e.error.retryAfterSeconds };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    if (rerender) refresh();
  }
}

export async function saveAutoPolicy(input: OpInput<"policies.save">, version?: number) {
  return run(() => coreOp<{ id: string; version: number }>("policies.save", input, { write: true, expectedVersion: input.id ? version : undefined }).then((p) => ({ id: p.id, version: p.version })));
}
type Evaluation = OpInput<"automations.simulate">;
/** The current minute of the Core API clock: an evaluation is for the current tick (IR21), so the time the page loaded
 * would be refused once a minute had passed (IR491). */
async function tick(): Promise<string> {
  return new Date(Math.floor((await coreNow()).getTime() / 60_000) * 60_000).toISOString();
}
const atTick = (input: Evaluation, at: string): Evaluation => ({ ...input, occurredAt: at, facts: input.facts.map((f) => ({ ...f, observedAt: at })) });

/** automations.simulate at the current tick: the selected rule or the suppression reason per unit; creates no Command. */
export async function simulateAuto(input: Evaluation) {
  return run(async () => coreOp<{ results: Decision[] }>("automations.simulate", atTick(input, await tick())).then((r) => r.results), false);
}
/** automations.fire (demo): the event ID is the one-time key. A new event takes the current tick; a repeat sends its
 * event's tick (`at`), so the Core API returns the stored result. */
export async function fireAuto(input: Evaluation, at?: string) {
  return run(async () => {
    const when = at ?? (await tick());
    const r = await coreOp<{ results: Decision[] }>("automations.fire", atTick(input, when), { write: true, idempotencyKey: input.eventId });
    return { results: r.results, at: when };
  });
}
