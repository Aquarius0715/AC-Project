import { beforeEach, describe, expect, it, vi } from "vitest";

// the job workspace's Server Actions other than the photo upload (photoAction.test.ts): each write carries the
// expected version, the job transitions refresh the page whether or not they succeed, a refused call comes back as the
// action's failure, and the check-in resolves the scanned QR label first (IR486)
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
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("@ac/web/lib/dal", () => dal);

import { draftFrom, draftInput } from "@ac/web/lib/techJob";
import { acknowledge, checkIn, pauseWork, resumeRework, saveDraft, signOff, submitReport } from "../app/technician/jobs/[id]/actions";

const refusal = (code: string) =>
  new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, correlationId: "c-1", retryAfterSeconds: null }, { status: 409 });
const signOffForm = (fields: Record<string, string | File>) => {
  const f = new FormData();
  f.set("jobId", "job-1"); f.set("reportId", "report-1"); f.set("reportVersion", "4");
  for (const [k, v] of Object.entries(fields)) f.set(k, v);
  return f;
};
const conflict = { ok: false, code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {}, retryAfterSeconds: null };
const png = new Uint8Array([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0]);
const report = { id: "report-1", version: 5, attachmentRefs: [{ id: "att-1", name: "unit.png" }], signOff: { signerName: "Aisha" }, workText: "dropped from the state" };
const state = { id: "report-1", version: 5, attachmentRefs: report.attachmentRefs, signOff: report.signOff };

beforeEach(() => { dal.coreOp.mockReset(); cache.refresh.mockReset(); });

describe("acknowledge", () => {
  it("accepts with the job version and no reason or other time", async () => {
    dal.coreOp.mockResolvedValueOnce({ ok: 1 });
    await expect(acknowledge("job-1", 7, "accept", "ignored", { startAt: "a", endAt: "b" })).resolves.toStrictEqual({ ok: true, value: { ok: 1 } });
    expect(dal.coreOp).toHaveBeenCalledWith("jobs.acknowledgeAssignment", { jobId: "job-1", decision: "accept" }, { write: true, expectedVersion: 7 });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("sends can't-make with its reason and other time, and an empty reason as none", async () => {
    dal.coreOp.mockResolvedValue(null);
    const slot = { startAt: "2026-11-20T01:00:00Z", endAt: "2026-11-20T03:00:00Z" };
    await acknowledge("job-1", 7, "cant_make", "Stuck in traffic", slot);
    await acknowledge("job-1", 7, "cant_make", "", null);
    const options = { write: true, expectedVersion: 7 };
    expect(dal.coreOp).toHaveBeenNthCalledWith(1, "jobs.acknowledgeAssignment", { alternativeSlot: slot, jobId: "job-1", decision: "cant_make", reason: "Stuck in traffic" }, options);
    expect(dal.coreOp).toHaveBeenNthCalledWith(2, "jobs.acknowledgeAssignment", { jobId: "job-1", decision: "cant_make", alternativeSlot: null }, options);
  });
});

describe("checkIn", () => {
  it("resolves the scanned label to its unit and checks in with the device's fix", async () => {
    dal.coreOp.mockResolvedValueOnce({ unitId: "unit-9" }).mockResolvedValueOnce({ ok: 1 });
    const position = { latitude: 3.1, longitude: 101.6, accuracyMeters: 12 };
    await expect(checkIn("job-1", 4, "location_qr", position, "QR-ABC", "not sent")).resolves.toStrictEqual({ ok: true, value: { ok: 1 } });
    expect(dal.coreOp).toHaveBeenNthCalledWith(1, "units.resolveQr", { code: "QR-ABC" });
    expect(dal.coreOp).toHaveBeenNthCalledWith(2, "jobs.checkIn", { position, jobId: "job-1", method: "location_qr", qrUnitId: "unit-9" }, { write: true, expectedVersion: 4 });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("refuses a label it does not find as the check-in refuses another unit's label, without checking in", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("NOT_FOUND"));
    await expect(checkIn("job-1", 4, "location_qr", null, "QR-OTHER", null)).resolves.toStrictEqual({
      ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { qrUnitId: "errors.qr_unit_mismatch" },
    });
    expect(dal.coreOp).toHaveBeenCalledOnce();
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("returns another refusal of the label lookup as it is", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT"));
    await expect(checkIn("job-1", 4, "location_qr", null, "QR-ABC", null)).resolves.toStrictEqual(conflict);
    expect(dal.coreOp).toHaveBeenCalledOnce();
  });

  it("checks in manually with the reason, drops a malformed fix and looks no label up", async () => {
    dal.coreOp.mockResolvedValue(null);
    const malformed = { latitude: "3.1", longitude: 101.6, accuracyMeters: 12 };
    // @ts-expect-error -- a Server Action's argument comes from the network: a malformed fix must not reach the Core API
    await checkIn("job-1", 4, "manual", malformed, "QR-ABC", "Label torn off");
    await checkIn("job-1", 4, "location_qr", null, null, "Label torn off");
    expect(dal.coreOp.mock.calls).toStrictEqual([
      ["jobs.checkIn", { jobId: "job-1", method: "manual", position: null, qrUnitId: null, reason: "Label torn off" }, { write: true, expectedVersion: 4 }],
      ["jobs.checkIn", { jobId: "job-1", method: "location_qr", position: null, qrUnitId: null }, { write: true, expectedVersion: 4 }],
    ]);
  });
});

describe("the job transitions", () => {
  it.each([
    ["pauseWork", () => pauseWork("job-1", 3, true), "jobs.pauseWork", { jobId: "job-1", paused: true }],
    ["submitReport", () => submitReport("job-1", 3, 6), "jobs.submit", { jobId: "job-1", reportVersion: 6 }],
    ["resumeRework", () => resumeRework("job-1", 3), "jobs.resumeRework", { jobId: "job-1" }],
  ])("%s writes at the job version and refreshes even when refused", async (_name, action, op, input) => {
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT"));
    await expect(action()).resolves.toStrictEqual(conflict);
    expect(dal.coreOp).toHaveBeenCalledWith(op, input, { write: true, expectedVersion: 3 });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });
});

describe("saveDraft", () => {
  it("creates the first draft without a version and saves a later one at its version, returning the report state", async () => {
    dal.coreOp.mockResolvedValue(report);
    const first = draftInput("job-1", null, draftFrom(null, []));
    const later = draftInput("job-1", "report-1", draftFrom(null, []));
    await expect(saveDraft(first, null)).resolves.toStrictEqual({ ok: true, value: state });
    await saveDraft(later, 4);
    await saveDraft(later, null); // a draft without a known version is sent at 0
    expect(dal.coreOp.mock.calls).toStrictEqual([
      ["jobs.saveDraft", first, { write: true }],
      ["jobs.saveDraft", later, { write: true, expectedVersion: 4 }],
      ["jobs.saveDraft", later, { write: true, expectedVersion: 0 }],
    ]);
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("returns a refused save as its failure and an unreachable Core API as unavailable, without refreshing", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT")).mockRejectedValueOnce(new TypeError("fetch failed"));
    const body = draftInput("job-1", "report-1", draftFrom(null, []));
    await expect(saveDraft(body, 4)).resolves.toStrictEqual(conflict);
    await expect(saveDraft(body, 4)).resolves.toStrictEqual({ ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} });
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});

describe("signOff", () => {
  const form = signOffForm;

  it("signs with the drawn signature at the draft version and sends no absence", async () => {
    dal.coreOp.mockResolvedValueOnce(report);
    const signature = png.toBase64();
    await expect(signOff(form({ signature, signerName: "  Aisha  ", absentReason: "not used", sitePhoto: new File([png], "site.png", { type: "image/png" }) }))).resolves.toStrictEqual({ ok: true, value: state });
    expect(dal.coreOp).toHaveBeenCalledWith("reports.signOff", {
      jobId: "job-1", reportId: "report-1", reportVersion: 4, signerName: "Aisha", signature: { name: "signature.png", mime: "image/png", size: png.length, bytes: signature },
    }, { write: true, expectedVersion: 4 });
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("sends the absence reason with the site photo, and without a photo when the file is empty", async () => {
    dal.coreOp.mockResolvedValue(report);
    await signOff(form({ signerName: "", absentReason: " Customer away ", sitePhoto: new File([png], "site.png", { type: "image/png" }) }));
    await signOff(form({ signerName: "", absentReason: "Customer away", sitePhoto: new File([], "empty.png", { type: "image/png" }) }));
    const absence = { jobId: "job-1", reportId: "report-1", reportVersion: 4, signerName: "", signature: null, absentReason: "Customer away" };
    const options = { write: true, expectedVersion: 4 };
    expect(dal.coreOp).toHaveBeenNthCalledWith(1, "reports.signOff", { ...absence, sitePhoto: { name: "site.png", mime: "image/png", size: png.length, bytes: png.toBase64() } }, options);
    // an empty file is no photo: the call matches only with sitePhoto absent or undefined
    expect(dal.coreOp).toHaveBeenNthCalledWith(2, "reports.signOff", absence, options);
  });

  it("returns a site photo that is not an image as a VALIDATION failure on it, and sends nothing", async () => {
    const pdf = new File([new TextEncoder().encode("%PDF-1.7")], "site.pdf", { type: "application/pdf" });
    await expect(signOff(form({ signerName: "", absentReason: "Customer away", sitePhoto: pdf }))).resolves.toStrictEqual({
      ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { sitePhoto: "error.invalid" }, retryAfterSeconds: null,
    });
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("returns a sign-off refused at a stale draft version as its failure, without refreshing", async () => {
    dal.coreOp.mockRejectedValueOnce(refusal("CONFLICT"));
    await expect(signOff(form({ signature: png.toBase64(), signerName: "Aisha" }))).resolves.toStrictEqual(conflict);
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});
