// Pacing for a screen that sends many writes in a row (IR380): Group control sends one command per AC and changed
// setting, which can be more than the member's write limit of a minute allows (IR375). A write refused as RATE_LIMITED
// changed nothing, so it is sent again after the wait the API advises (Retry-After), a few times at most; any other
// answer is returned as it is.

export type Paced = { ok: boolean; code?: string; retryAfterSeconds?: number | null };

/** Sends `send()`; while it is refused as RATE_LIMITED, calls `onWait(seconds)`, waits and sends again, up to `tries`
 * more times. The wait is the advised one, kept within 1–60 s (5 s when none is given). */
export async function sendPaced<R extends Paced>(send: () => Promise<R>, onWait: (seconds: number) => void, tries = 5,
  pause: (ms: number) => Promise<void> = (ms) => new Promise((done) => setTimeout(done, ms))): Promise<R> {
  let r = await send();
  for (let n = 0; !r.ok && r.code === "RATE_LIMITED" && n < tries; n++) {
    const seconds = Math.min(Math.max(r.retryAfterSeconds ?? 5, 1), 60);
    onWait(seconds);
    await pause(seconds * 1000);
    r = await send();
  }
  return r;
}
