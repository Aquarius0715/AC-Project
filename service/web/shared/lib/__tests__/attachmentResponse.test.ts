// @vitest-environment node
import { describe, expect, it, vi } from "vitest";

// the photo is read through the DAL: answer with a stored photo whose name is the user's
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    constructor(public error: { code: string; messageKey: string }) { super(error.code); }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { attachmentResponse } from "@ac/web/lib/attachmentResponse";

const open = () => attachmentResponse({ jobId: "j1", reportId: "r1", reportVersion: 2, attachmentId: "a1" });

// IR368: a photo whose name is not plain ASCII still opens, named in the header the RFC 5987 way
describe("a stored report photo as a response", () => {
  it("is served under any name", async () => {
    dal.coreOp.mockResolvedValueOnce({ name: "写真 1.jpg", mime: "image/jpeg", bytes: Buffer.from([0xff, 0xd8, 0xff, 0x01]).toString("base64") });
    const res = await open();
    expect(dal.coreOp).toHaveBeenCalledWith("attachments.getContent", { jobId: "j1", reportId: "r1", reportVersion: 2, attachmentId: "a1" });
    expect([res.status, res.headers.get("content-type"), res.headers.get("content-disposition")])
      .toEqual([200, "image/jpeg", `inline; filename="__ 1.jpg"; filename*=UTF-8''%E5%86%99%E7%9C%9F%201.jpg`]);
    expect(new Uint8Array(await res.arrayBuffer())).toEqual(new Uint8Array([0xff, 0xd8, 0xff, 0x01]));
    expect(res.headers.get("cache-control")).toBe("no-store"); // a signature is personal data (IR434)
  });
  it("passes the API's refusals on", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "NOT_FOUND", messageKey: "error.notFound" }));
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "FORBIDDEN", messageKey: "errors.assignment_ended" }));
    expect([(await open()).status, (await open()).status]).toEqual([404, 403]);
  });
});
