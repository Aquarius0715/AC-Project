"use server";

// Server Actions of the customer Units & locations screen (FR-C02, FR-C14). Each one is a public endpoint: the DAL
// verifies the session and the Core API authorizes it (client:self-customer for renames, client:control.execute for
// commands) and checks the version.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiCommand, ApiUnitDetail, UnitAction } from "@ac/web/lib/units";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };
const failure = (e: unknown): ActionResult<never> => (e instanceof CoreError
  ? { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} }
  : { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} });

/** locations.rename: only the name changes (trimmed 1–120, unique among siblings, IR109). */
export async function renameLocation(kind: "property" | "space" | "unit", id: string, version: number, name: string): Promise<ActionResult> {
  try {
    await coreOp("locations.rename", { target: { kind, id }, name: name.trim() }, { write: true, expectedVersion: version });
    return { ok: true, value: null };
  } catch (e) {
    return failure(e);
  } finally {
    refresh();
  }
}
/** One commands.create for one AC and setting with its current unit version and its own idempotency key (FR-C14);
 * the browser follows the command with commands.get through the BFF. */
export async function sendCommand(unitId: string, action: UnitAction): Promise<ActionResult<ApiCommand>> {
  try {
    const unit = await coreOp<ApiUnitDetail>("units.get", { id: unitId });
    return { ok: true, value: await coreOp<ApiCommand>("commands.create", { unitId, action, expectedUnitVersion: unit.version }, { write: true }) };
  } catch (e) {
    return failure(e);
  }
}
