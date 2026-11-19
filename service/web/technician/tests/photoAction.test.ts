import { beforeEach, describe, expect, it, vi } from "vitest";

// the job workspace's photo upload (attachments.add): a refused value comes back as the action's failure naming the
// field, never as an exception out of the Server Action (R1030), and the new report state is read at the next version
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; correlationId: string; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    readonly status: number;
    constructor(error: DomainError, options: ErrorOptions & { status: number }) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
      this.status = options.status;
    }
  }
  return { coreOp: vi.fn(), CoreError };
});
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => ({ refresh: vi.fn() }));
vi.mock("@ac/web/lib/dal", () => dal);

import { uploadPhoto } from "../app/technician/jobs/[id]/actions";

const png = new Uint8Array([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0]);
const photoForm = (tag?: string) => {
  const form = new FormData();
  form.set("jobId", "job-1"); form.set("reportId", "report-1"); form.set("reportVersion", "3");
  form.set("file", new File([png], "unit.png", { type: "image/png" }));
  if (tag !== undefined) form.set("tag", tag);
  return form;
};
const report = { id: "report-1", version: 4, attachmentRefs: [{ id: "att-1", name: "unit.png" }], signOff: null, workText: "x" };

beforeEach(() => { dal.coreOp.mockReset(); });

describe("uploadPhoto", () => {
  it("returns a tag the contract does not know as a VALIDATION failure on the tag, and sends nothing", async () => {
    await expect(uploadPhoto(photoForm("selfie"))).resolves.toStrictEqual({
      ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { tag: "error.invalid" }, retryAfterSeconds: null,
    });
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("adds the photo with its tag at the draft version and reads the report at the next one", async () => {
    dal.coreOp.mockResolvedValueOnce(null).mockResolvedValueOnce(report);
    await expect(uploadPhoto(photoForm("outdoor_unit"))).resolves.toStrictEqual({
      ok: true, value: { id: "report-1", version: 4, attachmentRefs: report.attachmentRefs, signOff: null },
    });
    expect(dal.coreOp).toHaveBeenNthCalledWith(1, "attachments.add", {
      jobId: "job-1", reportId: "report-1", tag: "outdoor_unit",
      file: { name: "unit.png", mime: "image/png", size: png.length, bytes: png.toBase64() },
    }, { write: true, expectedVersion: 3 });
    expect(dal.coreOp).toHaveBeenNthCalledWith(2, "reports.get", { jobId: "job-1", reportId: "report-1", reportVersion: 4 });
  });

  it("sends no tag for an untagged photo", async () => {
    dal.coreOp.mockResolvedValueOnce(null).mockResolvedValueOnce(report);
    await uploadPhoto(photoForm());
    expect(dal.coreOp.mock.calls[0]?.[1]).not.toHaveProperty("tag");
  });

  it("asks for a file when the form has none", async () => {
    const form = photoForm();
    form.delete("file");
    await expect(uploadPhoto(form)).resolves.toStrictEqual({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { file: "error.required" } });
    expect(dal.coreOp).not.toHaveBeenCalled();
  });
});
