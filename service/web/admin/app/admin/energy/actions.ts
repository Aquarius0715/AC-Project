"use server";

// Server Action of the HQ energy analysis (FR-A13): baselines.save. A public endpoint: the DAL verifies the session and
// the Core API authorizes it (energy.write) and checks the version a new baseline version was read at.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

/** A new baseline (no id) or the next version of one (id + the version it was read at). */
export async function saveBaseline(input: OpInput<"baselines.save">, version?: number): Promise<ActionResult<{ id: string; version: number }>> {
  try {
    const b = await coreOp<{ id: string; version: number }>("baselines.save", input, { write: true, expectedVersion: input.id ? version : undefined });
    return { ok: true, value: { id: b.id, version: b.version } };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}
