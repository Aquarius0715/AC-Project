// Job writes against the Core API (DATA_SOURCE=api). Each action reads the caller's projection with jobs.get for the
// version (and offer IDs), sends the write with a fresh Idempotency-Key and invalidates the screens' reads.
import { bffSession, invalidate } from "@ac/web/lib/useOp";
import { callOp, type WriteOptions } from "@ac/web/lib/ops";
import type { OpArgs, OpInput, OpName } from "@ac/web/lib/opTypes";

type Projection = { projection?: string; version?: number; jobVersion?: number; offerId?: string; termsVersion?: string; reportVersion?: number };

export async function apiMode(): Promise<boolean> {
  return (await bffSession()).dataSource === "api";
}

async function current(jobId: string): Promise<Projection> {
  return callOp<Projection>("jobs.get", { jobId });
}

/** One job write: the input of the operation without its jobId, typed by the contract (IR313). */
async function write<K extends OpName>(operation: K, jobId: string, input: Omit<OpInput<K>, "jobId">, version?: number) {
  const p = version === undefined ? await current(jobId) : null;
  const v = version ?? p?.version ?? p?.jobVersion;
  await callOp(...([operation, { jobId, ...input }, { write: true, expectedVersion: v }] as OpArgs<WriteOptions & { write?: boolean }>));
  invalidate();
}

export const jobsApi = {
  cancel: (jobId: string, reason: string) => write("jobs.cancel", jobId, { cancelReason: reason }),
  hold: (jobId: string, reason: string) => write("jobs.hold", jobId, { reason }),
  resumeHold: (jobId: string, reason: string) => write("jobs.resumeHold", jobId, { reason }),
  addNote: (jobId: string, message: string, visibility: "internal" | "customer" = "customer") => write("jobs.addNote", jobId, { message, visibility }),
  start: (jobId: string) => write("jobs.start", jobId, { startConfirmed: true }),
  techAccept: (jobId: string) => write("jobs.acknowledgeAssignment", jobId, { decision: "accept" }),
  techCantMake: (jobId: string, reason: string) => write("jobs.acknowledgeAssignment", jobId, { decision: "cant_make", reason, alternativeSlot: null }),
  async partnerAccept(jobId: string) {
    const o = await current(jobId); // the open Offer projection carries offerId, termsVersion and jobVersion (IR123)
    if (!o.offerId || !o.termsVersion) throw new Error("no open offer for this job");
    await write("jobs.accept", jobId, { offerId: o.offerId, termsVersion: o.termsVersion }, o.jobVersion ?? o.version);
  },
  async partnerDecline(jobId: string, reason: string) {
    const o = await current(jobId);
    if (!o.offerId) throw new Error("no open offer for this job");
    await write("jobs.decline", jobId, { offerId: o.offerId, reason }, o.jobVersion ?? o.version);
  },
};
