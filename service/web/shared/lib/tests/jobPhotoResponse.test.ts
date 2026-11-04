// @vitest-environment node
// A problem photo as a response (IR437): the file itself, named the RFC 5987 way, never stored by
// the browser; the API's refusals pass on.
import { describe, expect, it, vi } from "vitest";

const dal = vi.hoisted(() => {
  class CoreError extends Error {
    constructor(public error: { code: string; messageKey: string }) { super(error.code); }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { jobPhotoResponse } from "@ac/web/lib/jobPhotoResponse";

describe("a problem photo as a response", () => {
  it("is the file, inline, not stored", async () => {
    dal.coreOp.mockResolvedValueOnce({ name: "licence.pdf", mime: "application/pdf", bytes: Buffer.from("%PDF-1.4 x").toString("base64") });
    const res = await jobPhotoResponse("j1", "a1");
    expect(dal.coreOp).toHaveBeenCalledWith("jobs.getAttachment", { jobId: "j1", attachmentId: "a1" });
    expect([res.status, res.headers.get("content-type"), res.headers.get("cache-control"), res.headers.get("content-disposition")])
      .toEqual([200, "application/pdf", "no-store", `inline; filename="licence.pdf"; filename*=UTF-8''licence.pdf`]);
    expect(Buffer.from(await res.arrayBuffer()).toString()).toBe("%PDF-1.4 x");
  });
  it("passes the API's refusals on", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "NOT_FOUND", messageKey: "error.notFound" }));
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "FORBIDDEN", messageKey: "error.forbidden" }));
    expect([(await jobPhotoResponse("x", "y")).status, (await jobPhotoResponse("x", "y")).status]).toEqual([404, 403]);
  });
});
