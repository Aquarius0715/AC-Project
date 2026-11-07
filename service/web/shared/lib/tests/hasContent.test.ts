import { createElement, type ReactNode } from "react";
import { describe, expect, it } from "vitest";

import { hasContent } from "@ac/web/lib/hasContent";

describe("hasContent", () => {
  it("is false for what React renders as nothing", () => {
    expect([null, undefined, false, true, ""].map((node) => hasContent(node))).toStrictEqual([false, false, false, false, false]);
  });

  it("is true for text, numbers (0 included) and elements", () => {
    const nodes: ReactNode[] = ["a", 0, 12, [], createElement("span")];

    expect(nodes.map((node) => hasContent(node))).toStrictEqual([true, true, true, true, true]);
  });
});
