// A Server Action call that gets no answer at all (the browser offline, the app server restarting) rejects in the
// browser. Every screen that runs one reads that as the failure a Core API without an answer gives (IR683): unavailable,
// with the screen and what the user typed kept, instead of the error boundary or a screen stuck in its pending state
// (IR717, IR718). Next's own control flow (a redirect, not found) goes on.
import { unstable_rethrow } from "next/navigation";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Failed = { ok: false } & ActionFailure;

/** The failure of a Server Action call that got no answer. */
export const unreachableFailure = (): Failed => ({ ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} });

/** The action's answer, or `fallback` of the unavailable failure when the call got none: for an action whose answer is
not `{ ok }`-shaped (a failure or null, counts with a failure). */
export async function orUnreachable<R>(call: () => Promise<R>, fallback: (failure: Failed) => R): Promise<R> {
  try {
    return await call();
  } catch (error) {
    unstable_rethrow(error);
    return fallback(unreachableFailure());
  }
}

/** The answer of an `{ ok }`-shaped action, or the unavailable failure when the call got none. */
export function reached<R>(call: () => Promise<R>): Promise<R | Failed> {
  return orUnreachable<R | Failed>(call, (failure) => failure);
}
