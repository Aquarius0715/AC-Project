import { describe, expect, it, vi } from "vitest";
import { sendPaced } from "@ac/web/lib/writeLimit";

type R = { ok: boolean; code?: string; retryAfterSeconds?: number | null; n?: number };
const limited = (retryAfterSeconds: number | null): R => ({ ok: false, code: "RATE_LIMITED", retryAfterSeconds });

// IR380: Group control sends more commands than the write limit of a minute allows (IR375); a refused one waits and is
// sent again
describe("a write paced by the write limit", () => {
  it("waits the advised seconds and sends again until it goes through", async () => {
    const answers: R[] = [limited(12), limited(null), { ok: true, n: 3 }];
    const send = vi.fn(async () => answers.shift()!);
    const onWait = vi.fn(), pause = vi.fn(async () => {});
    expect(await sendPaced(send, onWait, 5, pause)).toEqual({ ok: true, n: 3 });
    expect([send.mock.calls.length, onWait.mock.calls, pause.mock.calls]).toEqual([3, [[12], [5]], [[12000], [5000]]]);
  });
  it("keeps the wait within 1–60 s", async () => {
    const answers: R[] = [limited(0), limited(600), { ok: true }];
    const onWait = vi.fn();
    await sendPaced(async () => answers.shift()!, onWait, 5, async () => {});
    expect(onWait.mock.calls).toEqual([[1], [60]]);
  });
  it("gives up after the last try and returns the refusal", async () => {
    const send = vi.fn(async () => limited(2));
    expect(await sendPaced(send, () => {}, 2, async () => {})).toEqual(limited(2));
    expect(send).toHaveBeenCalledTimes(3);
  });
  it("returns any other answer at once", async () => {
    const send = vi.fn(async (): Promise<R> => ({ ok: false, code: "CONFLICT" }));
    const onWait = vi.fn();
    expect(await sendPaced(send, onWait, 5, async () => {})).toEqual({ ok: false, code: "CONFLICT" });
    expect([send.mock.calls.length, onWait.mock.calls.length]).toEqual([1, 0]);
  });
});
