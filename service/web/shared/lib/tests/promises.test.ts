import { describe, expect, it } from "vitest";
import { orNull, recover } from "@ac/web/lib/promises";

describe("recover", () => {
  it("is the value when the read succeeds, and never calls the handler", async () => {
    let calls = 0;
    const value = await recover(Promise.resolve(7), () => { calls++; return -1; });
    expect(value).toBe(7);
    expect(calls).toBe(0);
  });

  it("is what the handler returns for a failure it recovers from, also when the handler answers with a promise", async () => {
    expect(await recover(Promise.reject(new Error("gone")), () => null)).toBeNull();
    expect(await recover(Promise.reject(new Error("gone")), () => Promise.resolve("later"))).toBe("later");
  });

  it("passes the error to the handler, and rethrows what the handler throws", async () => {
    const boom = new Error("boom");
    await expect(recover(Promise.reject(boom), (error) => { throw error; })).rejects.toBe(boom);
  });
});

describe("orNull", () => {
  it("is the value, or null on any failure", async () => {
    expect(await orNull(Promise.resolve(1))).toBe(1);
    expect(await orNull(Promise.reject(new Error("x")))).toBeNull();
  });
});
