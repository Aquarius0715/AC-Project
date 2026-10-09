"use server";

// Server Actions of /demo in API mode (FR-X05, DDC-07, IR154, IR249; Next.js: Server Functions are public endpoints, so
// each one checks its input and calls the Core API through the DAL): the scenario clock moves forward by one minute or
// one hour (demo.advanceClock), a device the signed-in role can see loses its connection or gets it back
// (demo.trigger device communication_lost / restored, SR20), and the app session can be expired.
import { refresh } from "next/cache";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { coreClockFresh, coreOp, CoreError } from "@ac/web/lib/dal";
import { SESSION_COOKIE } from "@ac/web/lib/session";
import { advanceTarget, nextSequence, openFault, type ApiDevice, type ApiDeviceEvent } from "@ac/web/lib/demoPanel";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const run = async <T,>(fn: () => Promise<T>): Promise<Result<T>> => {
  try {
    const value = await fn();
    refresh();
    return { ok: true, value };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
    throw e;
  }
};
const invalid = (field: string): Result<never> => ({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { [field]: "error.invalid" } });

/** demo.advanceClock to the scenario clock now plus one minute or one hour (forward only, IR36). Every service reads the
 * shared offset at most a second late (IR168), so the action waits until session.get shows the jump before the page
 * renders again. */
export async function advanceClock(minutes: number) {
  if (minutes !== 1 && minutes !== 60) return invalid("minutes");
  return run(async () => {
    const to = advanceTarget(await coreClockFresh(), minutes);
    await coreOp("demo.advanceClock", { to }, { write: true });
    for (let i = 0; i < 12 && (await coreClockFresh()).getTime() < Date.parse(to); i++) await new Promise((r) => setTimeout(r, 250));
    return null;
  });
}

/** demo.trigger for the device's current binding: communication_lost, or restored naming the open fault as its source. */
export async function deviceSignal(deviceId: string, restore: boolean) {
  if (!/^[0-9a-f-]{36}$/i.test(deviceId)) return invalid("deviceId");
  return run(async () => {
    const [device, events, now] = await Promise.all([
      coreOp<ApiDevice>("devices.get", { id: deviceId }),
      coreOp<{ items: ApiDeviceEvent[] }>("devices.events", { id: deviceId, query: { limit: 100 } }),
      coreClockFresh(),
    ]);
    const fault = openFault(events.items);
    if (restore && !fault) throw new CoreError(409, { code: "CONFLICT", messageKey: "errors.recovery_source_not_current", fieldErrors: {}, correlationId: "", retryAfterSeconds: null });
    await coreOp("demo.trigger", {
      scenarioId: "demo-panel", eventId: crypto.randomUUID(), occurredAt: now.toISOString(), eventType: "device",
      deviceId, bindingId: device.bindingId, sequence: nextSequence(events.items),
      ...(restore && fault ? { kind: "restored", recovery: { axis: "connection", value: "online", sourceEventId: fault.id } } : { kind: "communication_lost" }),
    }, { write: true });
    return null;
  });
}

/** Ends this app's session as if it had expired (D09): the next page asks to sign in again. */
export async function expireSession() {
  (await cookies()).delete(SESSION_COOKIE);
  redirect("/login");
}
