"use server";

// Server Actions of the team screen (FR-P06, FR-P09, DD-P06 Figma 04-8, DD-P09 Figma 04-6/04-7):
// members.setUnavailability for one technician or the whole company — confirmed assignments inside the days are kept,
// and the action returns how many the Core API found (conflictingAssignmentIds, IR276); members.clearUnavailability with
// the row's version, which removes the days from a chosen day on (IR612); certificates.submit for a
// renewal or a new certificate, its file read from the form data and sent base64 (IR129, IR277); and
// certificates.requestTraining with the certificate version as the expected version (write-version catalog).
import { isNone } from "@ac/web/lib/isNone";
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { certDates, certificateCode } from "@ac/web/lib/partnerCertificates";
import { formValue } from "@ac/web/lib/formValue";
import { documentMime } from "@ac/web/lib/uploads";
import { uploadName, uploadType } from "@ac/web/lib/files";
import type { OpInput } from "@ac/web/lib/opTypes";
import { formText } from "@ac/web/lib/formData";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const failure = (e: unknown): { ok: false } & ActionFailure => {
  if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors, retryAfterSeconds: e.error.retryAfterSeconds };
  throw e;
};

export async function setUnavailability(input: OpInput<"members.setUnavailability">): Promise<Result<{ conflicts: number }>> {
  try {
    const u = await coreOp<{ conflictingAssignmentIds?: string[] }>("members.setUnavailability", input, { write: true });
    refresh();
    return { ok: true, value: { conflicts: u.conflictingAssignmentIds?.length ?? 0 } };
  } catch (error) {
    return failure(error);
  }
}

/** members.clearUnavailability: the row's days from `from` (a Kuala Lumpur day) on are removed; `kept` is false when
the whole row went. A CONFLICT or NOT_FOUND reloads the list, since someone else changed it. */
export async function clearUnavailability(id: string, version: number, from: string): Promise<Result<{ kept: boolean }>> {
  try {
    const r = await coreOp<{ remaining: unknown }>("members.clearUnavailability", { id, from }, { write: true, expectedVersion: version });
    refresh();
    return { ok: true, value: { kept: !isNone(r.remaining) } };
  } catch (error) {
    if (error instanceof CoreError && (error.error.code === "CONFLICT" || error.error.code === "NOT_FOUND")) refresh();
    return failure(error);
  }
}

/** certificates.submit: membershipId, code, name, number, the issue and expiry days (Kuala Lumpur), the file and, for a
renewal, renewalOf. The certificate starts pending HQ verification. */
export async function submitCertificate(form: FormData): Promise<Result<{ id: string }>> {
  const file = form.get("file");
  const text = (k: string) => formText(form, k).trim();
  try {
    const bytes = file instanceof File ? Buffer.from(await file.arrayBuffer()) : Buffer.alloc(0);
    const renewalOf = text("renewalOf");
    const c = await coreOp<{ id: string }>("certificates.submit", {
      membershipId: text("membershipId"), code: formValue(certificateCode, text("code"), "code"), name: text("name"), number: text("number"), ...certDates({ issued: text("issued"), expires: text("expires") }),
      file: { name: file instanceof File ? uploadName(file.name) : "", mime: formValue(documentMime, file instanceof File ? uploadType(bytes, file.type) : "", "file"), size: bytes.length, bytes: bytes.toString("base64") },
      ...(renewalOf && { renewalOf }),
    }, { write: true });
    refresh();
    return { ok: true, value: { id: c.id } };
  } catch (error) {
    return failure(error);
  }
}

/** certificates.requestTraining: a note of 1–1000 characters for an expiring or expired certificate. */
export async function requestTraining(certificateId: string, version: number, note: string): Promise<Result<null>> {
  try {
    await coreOp("certificates.requestTraining", { certificateId, note: note.trim() }, { write: true, expectedVersion: version });
    refresh();
    return { ok: true, value: null };
  } catch (error) {
    if (error instanceof CoreError && error.error.code === "CONFLICT") refresh();
    return failure(error);
  }
}
