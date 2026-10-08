"use server";

// Server Actions of the customer unit screen (FR-C03, FR-C15). Each one is a public endpoint: the DAL verifies the
// session and the Core API authorizes the call (client:control.execute, own customer policies).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@/lib/dal";
import type { ApiCommand, ApiUnitDetail, UnitAction } from "@/lib/units";

export type ActionResult<T> = { ok: true; value: T } | { ok: false; messageKey: string; code: string };

function failed(e: unknown): { ok: false; messageKey: string; code: string } {
  if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code };
  return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE" };
}

/** commands.create with the current unit version; the client then polls commands.get until the device answers. */
export async function createCommand(unitId: string, action: UnitAction): Promise<ActionResult<ApiCommand>> {
  try {
    const unit = await coreOp<ApiUnitDetail>("units.get", { id: unitId });
    const command = await coreOp<ApiCommand>("commands.create", { unitId, action, expectedUnitVersion: unit.version }, { write: true });
    return { ok: true, value: command };
  } catch (e) {
    return failed(e);
  }
}

/** units.setAlertPolicies with the unit version, then re-renders the route. */
export async function setUnitAlertPolicies(unitId: string, alertPolicyIds: string[]): Promise<ActionResult<null>> {
  try {
    const unit = await coreOp<ApiUnitDetail>("units.get", { id: unitId });
    await coreOp("units.setAlertPolicies", { unitId, alertPolicyIds }, { write: true, expectedVersion: unit.version });
    return { ok: true, value: null };
  } catch (e) {
    return failed(e);
  } finally {
    refresh();
  }
}
