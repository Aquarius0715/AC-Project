import { describe, expect, it } from "vitest";
import { entryOf, lookup } from "@ac/web/lib/lookup";

describe("lookup", () => {
  const table = { a: "A", b: "B" };

  it("reads an entry of the table", () => {
    expect([lookup(table, "a"), lookup(table, "b")]).toStrictEqual(["A", "B"]);
  });

  it("finds nothing for a name the table does not have, including names an object inherits", () => {
    expect([lookup(table, "c"), lookup(table, "constructor"), lookup(table, "__proto__"), lookup(table, "toString")]).toStrictEqual([undefined, undefined, undefined, undefined]);
  });
});

describe("entryOf", () => {
  const table: Record<"a" | "b", string> = { a: "A", b: "B" };

  it("reads the entry of a key of the table's type", () => {
    expect([entryOf(table, "a"), entryOf(table, "b")]).toStrictEqual(["A", "B"]);
  });

  it("refuses a key the table does not have", () => {
    expect(() => entryOf<string, string>(table, "constructor")).toThrow("no entry for constructor");
  });
});
