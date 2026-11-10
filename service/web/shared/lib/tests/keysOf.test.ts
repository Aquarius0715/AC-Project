import { describe, expect, it } from "vitest";

import { keysOf } from "@ac/web/lib/keysOf";

describe("the keys of a record", () => {
  it("lists them in order, typed as the record's key type", () => {
    const labels: Record<"a" | "b", number> = { a: 1, b: 2 };
    const keys: ("a" | "b")[] = keysOf(labels);
    expect(keys).toStrictEqual(["a", "b"]);
  });

  it("is empty for an empty record", () => {
    expect(keysOf({})).toStrictEqual([]);
  });
});
