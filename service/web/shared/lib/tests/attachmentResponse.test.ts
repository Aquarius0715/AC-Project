// @vitest-environment node
import { describe, expect, it, vi } from "vitest";

// the photo is read through the DAL: answer with a stored photo whose name is the user's
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    constructor(public error: DomainError, options?: ErrorOptions) { super(error.code, options); this.name = "CoreError"; }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { attachmentResponse } from "@ac/web/lib/attachmentResponse";
import { domainError, type DomainError } from "@ac/web/lib/domainError";

const open = () => attachmentResponse({ jobId: "j1", reportId: "r1", reportVersion: 2, attachmentId: "a1" });

// IR368: a photo whose name is not plain ASCII still opens, named in the header the RFC 5987 way
describe("a stored report photo as a response", () => {
  it("is served under any name", async () => {
    dal.coreOp.mockResolvedValueOnce({ name: "写真 1.jpg", mime: "image/jpeg", bytes: Buffer.from([0xFF, 0xD8, 0xFF, 0x01]).toString("base64") });
    const res = await open();
    expect(dal.coreOp).toHaveBeenCalledWith("attachments.getContent", { jobId: "j1", reportId: "r1", reportVersion: 2, attachmentId: "a1" });
    expect([res.status, res.headers.get("content-type"), res.headers.get("content-disposition")])
      .toEqual([200, "image/jpeg", `inline; filename="__ 1.jpg"; filename*=UTF-8''%E5%86%99%E7%9C%9F%201.jpg`]);
    expect(new Uint8Array(await res.arrayBuffer())).toEqual(new Uint8Array([0xFF, 0xD8, 0xFF, 0x01]));
    expect(res.headers.get("cache-control")).toBe("no-store"); // a signature is personal data (IR434)
  });
  it("passes the API's refusals on", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError(domainError({ code: "NOT_FOUND", messageKey: "error.notFound" })));
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError(domainError({ code: "FORBIDDEN", messageKey: "errors.assignment_ended" })));
    const missing = await open();
    const denied = await open();
    expect([missing.status, denied.status]).toEqual([404, 403]);
  });
});
