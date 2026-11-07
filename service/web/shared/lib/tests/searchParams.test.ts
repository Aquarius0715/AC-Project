import { describe, expect, it } from "vitest";

import { queryReader, setParams, firstOf } from "@ac/web/lib/searchParams";

describe("a page's URL parameters", () => {
  it("reads a text value, and nothing for a missing, empty or repeated one", () => {
    const read = queryReader({ a: "x", empty: "", twice: ["1", "2"] });
    expect([read("a"), read("empty"), read("twice"), read("missing")]).toStrictEqual(["x", undefined, undefined, undefined]);
  });

  it("keeps the set parameters of a filter, in order", () => {
    expect(setParams({ customerId: "c-1", propertyId: "", limit: 5 })).toStrictEqual([["customerId", "c-1"]]);
    expect(setParams({})).toStrictEqual([]);
  });
});

describe("firstOf", () => {
  it("is the value, the first of several, or undefined", () => {
    expect(firstOf("a")).toBe("a");
    expect(firstOf(["a", "b"])).toBe("a");
    expect(firstOf([])).toBeUndefined();
    expect(firstOf(undefined)).toBeUndefined();
  });
});
