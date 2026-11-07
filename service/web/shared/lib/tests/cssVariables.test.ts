import { describe, expect, it } from "vitest";

import { cssVariables } from "@ac/web/lib/cssVariables";

describe("inline CSS custom properties", () => {
  it("passes them on as a style object", () => {
    expect(cssVariables({ "--gap": 2, "--color-bg": "red" })).toEqual({ "--gap": 2, "--color-bg": "red" });
  });
});
