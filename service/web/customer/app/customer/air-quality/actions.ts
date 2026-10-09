"use server";

// Server Actions of the customer air-quality page (FR-C07). Each one is a public endpoint: the DAL verifies the session
// and the Core API authorizes it (ventilation.log: client:self, the room of the caller's organization).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiVentilationLog, VentMethod } from "@ac/web/lib/air";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

/** ventilation.log: a manual record of the room (and the unit shown) with the CO2 reading of logging time — it creates
 * no Command and is not sent to HQ (IR110); 1–240 minutes, otherwise VALIDATION with the input kept in the modal. */
export async function logVentilation(input: { spaceId: string; unitId: string; method: VentMethod; durationMinutes: number }): Promise<ActionResult<ApiVentilationLog>> {
  try {
    return { ok: true, value: await coreOp<ApiVentilationLog>("ventilation.log", input, { write: true }) };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}
