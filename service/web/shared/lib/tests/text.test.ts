import { describe, expect, it } from "vitest";
import { isEmail } from "@ac/web/lib/text";

describe("isEmail", () => {
  it("accepts one @, a local part and a dot inside the domain", () => {
    expect(["a@b.c", "name@example.com", "a.b+c@d.e.f", "a@b..c"].map((a) => isEmail(a))).toEqual([true, true, true, true]);
  });

  it("refuses what the old pattern refused", () => {
    expect(["", "a", "a@b", "@b.c", "a@.c", "a@b.", "a@@b.c", "a@b@c.d", "a b@c.d", "a@b .c", "a@b.c\n"].map((a) => isEmail(a))).toEqual(Array.from({ length: 11 }, () => false));
  });
});
