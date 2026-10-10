"use server";

// Server Actions of the team screen (FR-P06, FR-P09, DD-P06 Figma 04-8, DD-P09 Figma 04-6/04-7):
// members.setUnavailability for one technician or the whole company — confirmed assignments inside the days are kept,
// and the action returns how many the Core API found (conflictingAssignmentIds, IR276); certificates.submit for a
// renewal or a new certificate, its file read from the form data and sent base64 (IR129, IR277); and
// certificates.requestTraining with the certificate version as the expected version (write-version catalog).
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { certDates } from "@ac/web/lib/partnerCertificates";
import { uploadType } from "@ac/web/lib/files";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const failure = (e: unknown): { ok: false } & ActionFailure => {
  if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
  throw e;
};

export async function setUnavailability(input: { membershipId: string | null; from: string; to: string; type: string; note?: string }): Promise<Result<{ conflicts: number }>> {
  try {
    const u = await coreOp<{ conflictingAssignmentIds?: string[] }>("members.setUnavailability", input, { write: true });
    refresh();
    return { ok: true, value: { conflicts: u.conflictingAssignmentIds?.length ?? 0 } };
  } catch (e) {
    return failure(e);
  }
}

/** certificates.submit: membershipId, code, name, number, the issue and expiry days (Kuala Lumpur), the file and, for a
 * renewal, renewalOf. The certificate starts pending HQ verification. */
export async function submitCertificate(form: FormData): Promise<Result<{ id: string }>> {
  const file = form.get("file");
  const text = (k: string) => String(form.get(k) ?? "").trim();
  try {
    const bytes = file instanceof File ? Buffer.from(await file.arrayBuffer()) : Buffer.alloc(0);
    const renewalOf = text("renewalOf");
    const c = await coreOp<{ id: string }>("certificates.submit", {
      membershipId: text("membershipId"), code: text("code"), name: text("name"), number: text("number"), ...certDates({ issued: text("issued"), expires: text("expires") }),
      file: { name: file instanceof File ? file.name : "", mime: file instanceof File ? uploadType(bytes, file.type) : "", size: bytes.length, bytes: bytes.toString("base64") },
      ...(renewalOf ? { renewalOf } : {}),
    }, { write: true });
    refresh();
    return { ok: true, value: { id: c.id } };
  } catch (e) {
    return failure(e);
  }
}

/** certificates.requestTraining: a note of 1–1000 characters for an expiring or expired certificate. */
export async function requestTraining(certificateId: string, version: number, note: string): Promise<Result<null>> {
  try {
    await coreOp("certificates.requestTraining", { certificateId, note: note.trim() }, { write: true, expectedVersion: version });
    refresh();
    return { ok: true, value: null };
  } catch (e) {
    if (e instanceof CoreError && e.error.code === "CONFLICT") refresh();
    return failure(e);
  }
}
