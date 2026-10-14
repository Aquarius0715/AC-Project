// Money in the screens: two decimals and thousands separators, as Figma shows them (“2,110.00 MYR”).
import { describe, expect, it } from "vitest";
import { amount, money } from "@ac/web/lib/money";

describe("money", () => {
  it("shows minor units with two decimals and thousands separators", () => {
    expect(money(211000, "MYR")).toBe("2,110.00 MYR");
    expect(money(12000, "MYR")).toBe("120.00 MYR");
    expect(money(5, "USD")).toBe("0.05 USD");
    expect(amount(123456789)).toBe("1,234,567.89");
    expect(amount(-12000)).toBe("-120.00");
  });
});
