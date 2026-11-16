"use server";

// The customer's pinned ACs (IR814) are kept with the user's preferences (IR819), so they follow the user to another
// device: read with preferences.get, written with preferences.update, which keeps the stored language and time zone.
import { z } from "zod";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import { MAX_PINS, togglePin } from "@ac/web/lib/pinnedUnits";
import type { OpInput } from "@ac/web/lib/opTypes";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };
const prefsSchema = z.object({ locale: z.enum(["en", "ms", "ja", "id"]), timezone: z.string(), pinnedUnitIds: z.array(z.string()).max(MAX_PINS).optional() });

/** Pins or unpins one AC and returns the pins as saved (a new pin goes first; the oldest drops beyond MAX_PINS). */
export async function togglePinned(unitId: string): Promise<ActionResult<string[]>> {
  try {
    const prefs = prefsSchema.parse(await coreOp("preferences.get", {}));
    const next = togglePin(prefs.pinnedUnitIds ?? [], unitId);
    const input: OpInput<"preferences.update"> = { locale: prefs.locale, timezone: prefs.timezone, pinnedUnitIds: next };
    await coreOp("preferences.update", input, { write: true });
    return { ok: true, value: next };
  } catch (error) {
    return actionFailure(error);
  }
}
