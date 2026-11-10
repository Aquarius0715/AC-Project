import { describe, expect, it } from "vitest";

import { answerOf } from "@ac/web/lib/answer";

describe("a Core API answer", () => {
  it("gives the data and the snapshot time of a well-formed body", () => {
    expect(answerOf<{ n: number }>({ data: { n: 1 }, meta: { snapshotAt: "2026-09-20T01:00:00Z" } })).toStrictEqual({ data: { n: 1 }, meta: { snapshotAt: "2026-09-20T01:00:00Z" } });
    expect(answerOf<null>({ data: null })).toStrictEqual({ data: null });
  });

  it("gives nothing for a body without data or with a malformed meta", () => {
    expect([answerOf({}), answerOf(null), answerOf("text"), answerOf({ data: 1, meta: { snapshotAt: 5 } })]).toStrictEqual([null, null, null, null]);
  });
});
