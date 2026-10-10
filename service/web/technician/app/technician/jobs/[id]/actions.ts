"use server";

// Server Actions of the technician job workspace (FR-T04–T06, T08, T09, T13–T15): every write carries the expected
// version of the write-version catalog (job version for the job transitions, report version for the draft, photos and
// sign-off). Draft saves, photos and the sign-off return the new report state to the client instead of refreshing,
// so unsaved checklist edits survive; job transitions refresh the page.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiTechReport } from "@ac/web/lib/techJob";
import { uploadType } from "@ac/web/lib/files";
import type { OpInput } from "@ac/web/lib/opTypes";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const failed = (e: unknown): Result<never> => {
  if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
  throw e;
};
const run = async <T,>(fn: () => Promise<T>, reload = false): Promise<Result<T>> => {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    return failed(e);
  } finally {
    if (reload) refresh();
  }
};
type Report = Pick<ApiTechReport, "id" | "version" | "attachmentRefs" | "signOff">;
const reportState = (r: ApiTechReport): Report => ({ id: r.id, version: r.version, attachmentRefs: r.attachmentRefs, signOff: r.signOff });

/** jobs.acknowledgeAssignment: accept, or "can't make this time" with a reason and an optional other slot. */
export async function acknowledge(jobId: string, version: number, decision: "accept" | "cant_make", reason: string | null, alternativeSlot: { startAt: string; endAt: string } | null) {
  return run(() => coreOp("jobs.acknowledgeAssignment", { jobId, decision, ...(decision === "cant_make" ? { ...(reason ? { reason } : {}), alternativeSlot } : {}) }, { write: true, expectedVersion: version }), true);
}

/** jobs.checkIn (IR111): location ≤ 200 m with the scanned unit QR, or manual with a reason; checking in starts the
 * work. The QR code is resolved with units.resolveQr first (the label's unit must be the job's unit). */
export async function checkIn(jobId: string, version: number, method: "location_qr" | "manual", distanceMeters: number | null, qrCode: string | null, reason: string | null) {
  return run(async () => {
    let qrUnitId: string | null = null;
    if (method === "location_qr" && qrCode) qrUnitId = (await coreOp<{ unitId: string }>("units.resolveQr", { code: qrCode })).unitId;
    return coreOp("jobs.checkIn", { jobId, method, distanceMeters: method === "location_qr" ? distanceMeters : null, qrUnitId, ...(method === "manual" && reason !== null ? { reason } : {}) }, { write: true, expectedVersion: version });
  }, true);
}

/** jobs.pauseWork: pause or resume the time on site. */
export async function pauseWork(jobId: string, version: number, paused: boolean) {
  return run(() => coreOp("jobs.pauseWork", { jobId, paused }, { write: true, expectedVersion: version }), true);
}

/** jobs.saveDraft: the first save creates the draft (no version); later saves carry the draft version. */
export async function saveDraft(body: OpInput<"jobs.saveDraft">, reportVersion: number | null) {
  return run(async () => reportState(await coreOp<ApiTechReport>("jobs.saveDraft", body, { write: true, ...(body.reportId ? { expectedVersion: reportVersion ?? 0 } : {}) })));
}

/** attachments.add: one JPEG/PNG photo (≤ 5 MiB) from a form field; the draft version moves on and the sign-off is cleared. */
export async function uploadPhoto(form: FormData) {
  const jobId = String(form.get("jobId")), reportId = String(form.get("reportId")), version = Number(form.get("reportVersion"));
  const file = form.get("file");
  if (!(file instanceof File)) return { ok: false as const, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { file: "error.required" } };
  const buf = Buffer.from(await file.arrayBuffer());
  return run(async () => {
    await coreOp("attachments.add", { jobId, reportId, file: { name: file.name, mime: uploadType(buf, file.type) as OpInput<"attachments.add">["file"]["mime"], size: file.size, bytes: buf.toString("base64") } }, { write: true, expectedVersion: version });
    return reportState(await coreOp<ApiTechReport>("reports.get", { jobId, reportId, reportVersion: version + 1 }));
  });
}

/** reports.signOff (FR-T15): the customer's signature (PNG drawn on the device), or an absence reason with a site
 * photo, bound to this draft version (a later save clears it). */
export async function signOff(form: FormData) {
  const jobId = String(form.get("jobId")), reportId = String(form.get("reportId")), reportVersion = Number(form.get("reportVersion"));
  const signerName = String(form.get("signerName") ?? "").trim();
  const png = form.get("signature"), photo = form.get("sitePhoto"), absent = String(form.get("absentReason") ?? "").trim();
  const signature = typeof png === "string" && png ? { name: "signature.png", mime: "image/png" as const, size: Buffer.from(png, "base64").length, bytes: png } : null;
  const photoBytes = photo instanceof File && photo.size > 0 ? Buffer.from(await photo.arrayBuffer()) : null;
  const sitePhoto = photo instanceof File && photoBytes ? { name: photo.name, mime: uploadType(photoBytes, photo.type) as OpInput<"attachments.add">["file"]["mime"], size: photo.size, bytes: photoBytes.toString("base64") } : undefined;
  return run(async () => reportState(await coreOp<ApiTechReport>("reports.signOff", {
    jobId, reportId, reportVersion, signerName, signature, ...(signature ? {} : { absentReason: absent, sitePhoto }),
  }, { write: true, expectedVersion: reportVersion })));
}

/** jobs.submit: the current draft version; the job version is the expected version. */
export async function submitReport(jobId: string, jobVersion: number, reportVersion: number) {
  return run(() => coreOp("jobs.submit", { jobId, reportVersion }, { write: true, expectedVersion: jobVersion }), true);
}

/** jobs.resumeRework: a returned report continues as a new draft version. */
export async function resumeRework(jobId: string, version: number) {
  return run(() => coreOp("jobs.resumeRework", { jobId }, { write: true, expectedVersion: version }), true);
}
