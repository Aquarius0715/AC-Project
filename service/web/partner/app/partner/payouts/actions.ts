"use server";

// Server Action of the payouts screen (FR-P10): payouts.query asks HQ about one line of an approved statement; the
// statement version is the expected version (write-version catalog) and the page re-reads after it.
import { refresh } from "next/cache";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { OpInput } from "@ac/web/lib/opTypes";

type Result<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const failed = (e: unknown): Result<never> => actionFailure(e);

export async function askHq(statementId: string, version: number, lineId: string, topic: OpInput<"payouts.query">["topic"], message: string): Promise<Result<null>> {
  try {
    await coreOp("payouts.query", { statementId, lineId, topic, message }, { write: true, expectedVersion: version });
    return { ok: true, value: null };
  } catch (error) {
    return failed(error);
  } finally {
    refresh();
  }
}
