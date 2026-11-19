import { describe, expect, it } from "vitest";
import { originOf } from "@ac/web/lib/origin";

describe("origin of a job (IR113, DEC-106)", () => {
  it("tells a plan, an HQ request and a client request apart", () => {
    expect(originOf("periodic_plan")).toBe("plan");
    expect(originOf("hq_request")).toBe("hq");
    expect(originOf("client_request")).toBe("request");
    expect(originOf("")).toBe("request");
  });
});
