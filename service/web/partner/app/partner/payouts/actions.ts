"use server";

// Server Action of the payouts screen (FR-P10): payouts.query asks HQ about one line of an approved statement; the
// statement version is the expected version (write-version catalog) and the page re-reads after it.
import { refresh } from "next/cache";
import { coreOp, CoreError } from "@ac/web/lib/dal";
import type { ActionFailure } from "@ac/web/lib/actionMessage";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const failed = (e: unknown): Result<never> => {
  if (e instanceof CoreError) return { ok: false, code: e.error.code, messageKey: e.error.messageKey, fieldErrors: e.error.fieldErrors };
  throw e;
};

export async function askHq(statementId: string, version: number, lineId: string, topic: string, message: string): Promise<Result<null>> {
  try {
    await coreOp("payouts.query", { statementId, lineId, topic, message }, { write: true, expectedVersion: version });
    return { ok: true, value: null };
  } catch (e) {
    return failed(e);
  } finally {
    refresh();
  }
}
