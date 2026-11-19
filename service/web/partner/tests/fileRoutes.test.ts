// The partner app's two file routes: a certificate's uploaded document (FR-P09, IR436) and a problem photo of a
// follow-up job (FR-P02, IR437). Each passes its URL ids to its Core API operation and answers with the file itself, not
// stored by the browser; the API's refusals become 404 / 403, and any other failure of the Core API a 502.
import { beforeEach, describe, expect, it, vi } from "vitest";

const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(code: string, options?: ErrorOptions) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code, messageKey: `error.${code.toLowerCase()}` };
    }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { GET as certificateDocument } from "../app/partner/certificates/[id]/document/route";
import { GET as jobPhoto } from "../app/partner/jobs/[id]/photos/[attachmentId]/route";

const request = new Request("http://partner.test/");
const documentCtx = (id: string): Parameters<typeof certificateDocument>[1] => ({ params: Promise.resolve({ id }) });
const photoCtx = (id: string, attachmentId: string): Parameters<typeof jobPhoto>[1] => ({ params: Promise.resolve({ id, attachmentId }) });
const file = (name: string, mime: string, text: string) => ({ name, mime, bytes: new TextEncoder().encode(text).toBase64() });
// the status, the body and the file headers of each of three calls: a refusal sends its message key and no file
const statusesOf = async (call: () => Promise<Response>) => {
  const answers: [number, string, string | null][] = [];
  for (let i = 0; i < 3; i++) {
    const res = await call();
    const body = await res.text();
    answers.push([res.status, body, res.headers.get("content-disposition")]);
  }
  return answers;
};
const refusals = [[404, "error.not_found", null], [403, "error.forbidden", null], [502, "error.unavailable", null]];

beforeEach(() => { dal.coreOp.mockReset(); });

describe("the certificate document route", () => {
  it("reads the certificate of the URL and sends its file inline, not stored", async () => {
    dal.coreOp.mockResolvedValueOnce(file("licence.pdf", "application/pdf", "%PDF-1.4 x"));
    const res = await certificateDocument(request, documentCtx("cert-7"));
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("certificates.getContent", { certificateId: "cert-7" });
    expect([res.status, res.headers.get("content-type"), res.headers.get("cache-control"), res.headers.get("content-disposition")])
      .toStrictEqual([200, "application/pdf", "no-store", `inline; filename="licence.pdf"; filename*=UTF-8''licence.pdf`]);
    await expect(res.text()).resolves.toBe("%PDF-1.4 x");
  });

  it("answers another company's certificate with 404, a forbidden one with 403 and an outage with 502", async () => {
    dal.coreOp
      .mockRejectedValueOnce(new dal.CoreError("NOT_FOUND"))
      .mockRejectedValueOnce(new dal.CoreError("FORBIDDEN"))
      .mockRejectedValueOnce(new dal.CoreError("UNAVAILABLE"));
    await expect(statusesOf(() => certificateDocument(request, documentCtx("other")))).resolves.toStrictEqual(refusals);
  });

  it("lets an error that is not the Core API's through", async () => {
    const redirect = new Error("NEXT_REDIRECT");
    dal.coreOp.mockRejectedValueOnce(redirect);
    await expect(certificateDocument(request, documentCtx("cert-7"))).rejects.toBe(redirect);
  });
});

describe("the job photo route", () => {
  it("reads the photo of the URL's job and sends it inline, not stored, its name the RFC 5987 way", async () => {
    dal.coreOp.mockResolvedValueOnce(file("写真.jpg", "image/jpeg", "jpeg bytes"));
    const res = await jobPhoto(request, photoCtx("job-p02", "att-3"));
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("jobs.getAttachment", { jobId: "job-p02", attachmentId: "att-3" });
    expect([res.status, res.headers.get("content-type"), res.headers.get("cache-control"), res.headers.get("content-disposition")])
      .toStrictEqual([200, "image/jpeg", "no-store", `inline; filename="__.jpg"; filename*=UTF-8''%E5%86%99%E7%9C%9F.jpg`]);
    await expect(res.text()).resolves.toBe("jpeg bytes");
  });

  it("answers another company's job with 404, a forbidden one with 403 and an outage with 502", async () => {
    dal.coreOp
      .mockRejectedValueOnce(new dal.CoreError("NOT_FOUND"))
      .mockRejectedValueOnce(new dal.CoreError("FORBIDDEN"))
      .mockRejectedValueOnce(new dal.CoreError("UNAVAILABLE"));
    await expect(statusesOf(() => jobPhoto(request, photoCtx("job-x", "att-1")))).resolves.toStrictEqual(refusals);
  });

  it("lets an error that is not the Core API's through", async () => {
    const redirect = new Error("NEXT_REDIRECT");
    dal.coreOp.mockRejectedValueOnce(redirect);
    await expect(jobPhoto(request, photoCtx("job-p02", "att-3"))).rejects.toBe(redirect);
  });
});
