"use server";

// Server Action of the HQ contract editor (FR-A07). A public endpoint: the DAL verifies the session and the Core API
// authorizes contracts.save (contract.write), checks the expected version of a revision and blocks restricted contracts.
import { refresh } from "next/cache";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { ApiContractFull, PlanType } from "@ac/web/lib/contracts";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

export interface ContractInput {
  id?: string; customerId: string; unitIds: string[]; planType: PlanType; startAt: string; endAt: string; priceMinor: number; currency: "MYR" | "USD";
  restrictionEligible: boolean; rulesVersion: string | null;
}

/** contracts.save: a new contract (no id) or a new version of one (id + the version it was read at). */
export async function saveContract(input: ContractInput, version?: number): Promise<ActionResult<{ id: string; version: number }>> {
  try {
    const k = await coreOp<ApiContractFull>("contracts.save", input, { write: true, expectedVersion: input.id ? version : undefined });
    return { ok: true, value: { id: k.id, version: k.version } };
  } catch (error) {
    return actionFailure(error);
  } finally {
    refresh();
  }
}
