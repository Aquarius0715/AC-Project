import { describe, expect, it } from "vitest";

import { z } from "@ac/web/lib/zod";

describe("zod for the browser", () => {
  it("has the JIT compiler off, so the CSP never sees a new Function probe", () => {
    expect(z.core.globalConfig.jitless).toBe(true);
  });

  it("still parses objects", () => {
    const schema = z.object({ id: z.string(), n: z.number().optional() });
    expect(schema.parse({ id: "a" })).toStrictEqual({ id: "a" });
    expect(schema.safeParse({ id: 1 }).success).toBe(false);
  });
});
