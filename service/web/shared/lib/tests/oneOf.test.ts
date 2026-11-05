import { describe, expect, it } from "vitest";

import { oneOf, oneOfOrThrow } from "@ac/web/lib/oneOf";

describe("narrowing a form control's text to its options", () => {
  const options = ["all", "overdue", "none"] as const;

  it("gives the matching option", () => {
    expect(oneOf(options, "overdue", "all")).toBe("overdue");
  });

  it("gives the fallback for text that is no option", () => {
    expect([oneOf(options, "", "all"), oneOf(options, "OVERDUE", "none"), oneOf(options, "toString", "all")]).toEqual(["all", "none", "all"]);
  });
});

describe("narrowing a value that must be an option", () => {
  const options = ["fault", "maintenance"] as const;

  it("gives the matching option", () => {
    expect(oneOfOrThrow(options, "fault", "category")).toBe("fault");
  });

  it("refuses anything else, naming the value and the options", () => {
    expect(() => oneOfOrThrow(options, "air", "category")).toThrow('category "air" is not one of fault, maintenance');
  });
});
