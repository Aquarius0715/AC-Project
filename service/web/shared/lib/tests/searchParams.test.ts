import { describe, expect, it } from "vitest";

import { queryReader, setParams } from "@ac/web/lib/searchParams";

describe("a page's URL parameters", () => {
  it("reads a text value, and nothing for a missing, empty or repeated one", () => {
    const read = queryReader({ a: "x", empty: "", twice: ["1", "2"] });
    expect([read("a"), read("empty"), read("twice"), read("missing")]).toEqual(["x", undefined, undefined, undefined]);
  });

  it("keeps the set parameters of a filter, in order", () => {
    expect(setParams({ customerId: "c-1", propertyId: "", limit: 5 })).toEqual([["customerId", "c-1"]]);
    expect(setParams({})).toEqual([]);
  });
});
