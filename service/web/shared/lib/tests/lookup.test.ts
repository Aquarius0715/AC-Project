import { describe, expect, it } from "vitest";
import { lookup } from "@ac/web/lib/lookup";

describe("lookup", () => {
  const table = { a: "A", b: "B" };

  it("reads an entry of the table", () => {
    expect([lookup(table, "a"), lookup(table, "b")]).toEqual(["A", "B"]);
  });

  it("finds nothing for a name the table does not have, including names an object inherits", () => {
    expect([lookup(table, "c"), lookup(table, "constructor"), lookup(table, "__proto__"), lookup(table, "toString")]).toEqual([undefined, undefined, undefined, undefined]);
  });
});
