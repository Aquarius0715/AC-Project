// @vitest-environment node
// A certificate's uploaded document as a response (IR436): the file itself, named the RFC 5987 way, never stored by
// the browser; the API's refusals pass on.
import { describe, expect, it, vi } from "vitest";

const dal = vi.hoisted(() => {
  class CoreError extends Error {
    error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) { super(error.code, options); this.name = "CoreError"; this.error = error; }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { documentResponse } from "@ac/web/lib/documentResponse";
import { domainError, type DomainError } from "@ac/web/lib/domainError";

describe("a certificate document as a response", () => {
  it("is the file, inline, not stored", async () => {
    dal.coreOp.mockResolvedValueOnce({ name: "licence.pdf", mime: "application/pdf", bytes: new TextEncoder().encode("%PDF-1.4 x").toBase64() });
    const res = await documentResponse("c1");
    expect(dal.coreOp).toHaveBeenCalledWith("certificates.getContent", { certificateId: "c1" });
    expect([res.status, res.headers.get("content-type"), res.headers.get("cache-control"), res.headers.get("content-disposition")])
      .toStrictEqual([200, "application/pdf", "no-store", `inline; filename="licence.pdf"; filename*=UTF-8''licence.pdf`]);
    expect(Buffer.from(await res.arrayBuffer()).toString()).toBe("%PDF-1.4 x");
  });
  it("passes the API's refusals on", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError(domainError({ code: "NOT_FOUND", messageKey: "error.notFound" })));
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError(domainError({ code: "FORBIDDEN", messageKey: "error.forbidden" })));
    const missing = await documentResponse("x");
    const denied = await documentResponse("x");
    expect([missing.status, denied.status]).toStrictEqual([404, 403]);
  });
});
