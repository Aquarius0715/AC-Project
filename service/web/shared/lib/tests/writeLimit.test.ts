import { describe, expect, it, vi } from "vitest";
import { sendPaced } from "@ac/web/lib/writeLimit";
import { must } from "./must";

interface R { ok: boolean; code?: string; retryAfterSeconds?: number | null; n?: number }
const limited = (retryAfterSeconds: number | null): R => ({ ok: false, code: "RATE_LIMITED", retryAfterSeconds });

// IR380: Group control sends more commands than the write limit of a minute allows (IR375); a refused one waits and is
// sent again
describe("a write paced by the write limit", () => {
  it("waits the advised seconds and sends again until it goes through", async () => {
    const answers: R[] = [limited(12), limited(null), { ok: true, n: 3 }];
    const send = vi.fn(() => Promise.resolve(must(answers.shift())));
    const onWait = vi.fn(), pause = vi.fn(() => Promise.resolve());
    expect(await sendPaced(send, onWait, 5, pause)).toStrictEqual({ ok: true, n: 3 });
    expect([send.mock.calls.length, onWait.mock.calls, pause.mock.calls]).toStrictEqual([3, [[12], [5]], [[12_000], [5000]]]);
  });
  it("keeps the wait within 1–60 s", async () => {
    const answers: R[] = [limited(0), limited(600), { ok: true }];
    const onWait = vi.fn();
    await sendPaced(() => Promise.resolve(must(answers.shift())), onWait, 5, () => Promise.resolve());
    expect(onWait.mock.calls).toStrictEqual([[1], [60]]);
  });
  it("gives up after the last try and returns the refusal", async () => {
    const send = vi.fn(() => Promise.resolve(limited(2)));
    expect(await sendPaced(send, vi.fn(), 2, () => Promise.resolve())).toStrictEqual(limited(2));
    expect(send).toHaveBeenCalledTimes(3);
  });
  it("returns any other answer at once", async () => {
    const send = vi.fn((): Promise<R> => Promise.resolve({ ok: false, code: "CONFLICT" }));
    const onWait = vi.fn();
    expect(await sendPaced(send, onWait, 5, () => Promise.resolve())).toStrictEqual({ ok: false, code: "CONFLICT" });
    expect([send.mock.calls.length, onWait.mock.calls.length]).toStrictEqual([1, 0]);
  });
});
