import { describe, expect, it } from "vitest";
import { isEmail, withoutNote, withoutYear } from "@ac/web/lib/text";

describe("isEmail", () => {
  it("accepts one @, a local part and a dot inside the domain", () => {
    expect(["a@b.c", "name@example.com", "a.b+c@d.e.f", "a@b..c"].map((a) => isEmail(a))).toStrictEqual([true, true, true, true]);
  });

  it("refuses what the old pattern refused", () => {
    expect(["", "a", "a@b", "@b.c", "a@.c", "a@b.", "a@@b.c", "a@b@c.d", "a b@c.d", "a@b .c", "a@b.c\n"].map((a) => isEmail(a))).toStrictEqual(Array.from({ length: 11 }, () => false));
  });
});

describe("withoutNote and withoutYear", () => {
  it("drop a trailing note or year as the old patterns did", () => {
    expect(["Power (kW)", "Power", "a (b) (c)", "a (b (c)", "(x)", "x )"].map((a) => withoutNote(a))).toStrictEqual(["Power", "Power", "a (b)", "a", "", "x )"]);
    expect(["Mar 2026", "Mar2026", "2026", "Mar 26", "12345"].map((a) => withoutYear(a))).toStrictEqual(["Mar", "Mar", "", "Mar 26", "1"]);
  });
});
