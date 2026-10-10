"use server";

// Server Action of the customer assistant (FR-X02): the confirmed change becomes one commands.create. A public endpoint:
// the DAL verifies the session and the Core API authorizes it (client:control.execute, the same checks as Unit
// Control). The unit version is the one voice.resolveIntent read, so a unit changed in between is a CONFLICT and is
// asked again instead of sent (AT-X02-N ③, IR09). The command is labelled voice in the unit's history (IR306). Reading
// the intent and following the command go through the BFF.
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiCommand } from "@ac/web/lib/units";

export type ActionResult<T> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

/** commands.create set_temperature with the version the intent was resolved at, source voice; no job (AT-X02-N). */
export async function sendVoiceChange(unitId: string, celsius: number, expectedVersion: number): Promise<ActionResult<ApiCommand>> {
  try {
    const command = await coreOp<ApiCommand>("commands.create", { unitId, action: { kind: "set_temperature", celsius }, expectedUnitVersion: expectedVersion, source: "voice" }, { write: true });
    return { ok: true, value: command };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  }
}
