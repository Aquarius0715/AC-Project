"use server";

// Server Actions of the customer Installations page (FR-C20, DEC-107): an owner asks HQ to install new aircons and
// cancels a request that is still submitted. The Core API keeps every rule (own customer's property, 1–10 units, the 3
// preferred times, one decision by HQ).
import { refresh } from "next/cache";
import { coreOp } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { InstallationRequest } from "@ac/web/lib/installationRequests";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string>; retryAfterSeconds?: number | null };

async function run<T>(fn: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (error) {
    return actionFailure(error);
  } finally {
    refresh();
  }
}

export interface NewRequest {
  propertyId: string; spaceId: string; quantity: number; modelId: string; withIot: boolean;
  slots: InstallationRequest["preferredSlots"]; contactWindow: string; note: string;
}

/** installationRequests.create: an empty space or model is left out (any space, model not decided). */
export async function createInstallationRequest(input: NewRequest) {
  return run(async () => {
    const r = await coreOp<InstallationRequest>("installationRequests.create", {
      propertyId: input.propertyId, quantity: input.quantity, withIot: input.withIot, preferredSlots: input.slots,
      ...(input.spaceId && { spaceId: input.spaceId }), ...(input.modelId && { modelId: input.modelId }),
      ...(input.contactWindow.trim() && { contactWindow: input.contactWindow.trim() }), ...(input.note.trim() && { note: input.note.trim() }),
    }, { write: true });
    return { id: r.id };
  });
}

export async function cancelInstallationRequest(id: string, version: number) {
  return run(async () => { await coreOp("installationRequests.cancel", { id }, { write: true, expectedVersion: version }); return null; });
}
