"use server";

// Server Actions of the HQ customers & units screen (FR-A02). Each one is a public endpoint: the DAL verifies the
// session and the Core API authorizes it (asset.write; alert.policy.write for default rules) and checks the version of
// the record it changes.
import { refresh } from "next/cache";
import { coreAll, coreOp, CoreError } from "@ac/web/lib/dal";
import { actionFailure } from "@ac/web/lib/actionFailure";
import type { ApiCustomerRow, ApiImportPreview, ApiOrgRow, ApiUnitImport, Profile, Scope } from "@ac/web/lib/assets";
import type { OpInput } from "@ac/web/lib/opTypes";

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
const write = (version?: number) => (version === undefined ? { write: true } : { write: true, expectedVersion: version });

export interface CustomerInput { billingName: string; name: string; serviceProfile: Profile; status: "active" | "inactive" }
/** organizations.save (kind customer) and then customers.save. A billing name already used by an organization that
never got its customer (an earlier attempt that stopped half way) is reused instead of failing as a duplicate. */
export async function createCustomer(input: CustomerInput) {
  return run(async () => {
    const billingName = input.billingName.trim();
    let org: ApiOrgRow;
    try {
      org = await coreOp<ApiOrgRow>("organizations.save", { name: billingName, kind: "customer", status: "active" }, write());
    } catch (error) {
      if (!(error instanceof CoreError && error.error.messageKey === "error.duplicateName")) throw error;
      const customers = await coreAll<ApiOrgRow>("organizations.list", { filters: { kind: "customer" } });
      const same = customers.find((o) => o.name.trim().toLowerCase() === billingName.toLowerCase());
      const lists = same ? await Promise.all((["active", "inactive"] as const).map((status) => coreAll<ApiCustomerRow>("customers.list", { filters: { organizationId: same.id, status } }))) : [];
      const taken = lists.flat().length > 0;
      if (!same || taken) throw error;
      org = same;
    }
    const c = await coreOp<ApiCustomerRow>("customers.save", { name: input.name.trim(), organizationId: org.id, serviceProfile: input.serviceProfile, status: input.status }, write());
    return c.id;
  });
}
/** Renames the billing organization when it changed (organizations.save) and saves the customer (customers.save). */
export async function updateCustomer(id: string, version: number, org: { id: string; version: number; name: string; status: "active" | "inactive" }, input: CustomerInput) {
  return run(async () => {
    if (org.name.trim() !== input.billingName.trim()) {
      await coreOp("organizations.save", { id: org.id, name: input.billingName.trim(), kind: "customer", status: org.status }, write(org.version));
    }
    await coreOp("customers.save", { id, name: input.name.trim(), organizationId: org.id, serviceProfile: input.serviceProfile, status: input.status }, write(version));
    return null;
  });
}

/** customers.delete and then organizations.delete: a customer registered by mistake is removed as it was made (DEC-103);
a customer with units, jobs, contracts, invoices or users is refused with the reason. */
export async function deleteCustomer(id: string, version: number, org: { id: string; version: number }, reason: string) {
  return run(async () => {
    const why = reason.trim();
    await coreOp("customers.delete", { id, reason: why }, write(version));
    await coreOp("organizations.delete", { id: org.id, reason: why }, write(org.version));
    return null;
  });
}

export interface PropertyInput { id?: string; customerOrgId: string; kind: "home" | "office"; name: string; address: string | null; accessInstructions: string | null }
export async function saveProperty(input: PropertyInput, version?: number) {
  return run(async () => { const p = await coreOp<{ id: string }>("properties.save", input, write(version)); return p.id; });
}
export interface SpaceInput { id?: string; propertyId: string; parentSpaceId: string | null; kind: "area" | "floor" | "room" | "space"; name: string }
export async function saveSpace(input: SpaceInput, version?: number) {
  return run(async () => { const s = await coreOp<{ id: string }>("spaces.save", input, write(version)); return s.id; });
}
/** properties.archive / spaces.archive (CONFLICT while it still holds units or child spaces). */
export async function archiveLocation(kind: "property" | "space", id: string, version: number, reason: string) {
  return run(async () => { await coreOp(kind === "property" ? "properties.archive" : "spaces.archive", { id, reason }, write(version)); return null; });
}

export interface UnitInput {
  id?: string; customerOrgId: string; propertyId: string; spaceId: string | null; displayName: string; modelId: string; type: "split"; installedAt: string | null;
  serviceScope: Scope[]; warrantyEndsAt?: string | null; changeReason?: string;
}
export async function saveUnit(input: UnitInput, version?: number) {
  return run(async () => { const u = await coreOp<{ id: string }>("units.save", input, write(version)); return u.id; });
}
/** units.archive takes the unit out of use and keeps its ID and history (D05: CONFLICT while it is still in use). */
export async function archiveUnit(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("units.archive", { id, reason }, write(version)); return null; });
}
/** units.delete removes a unit registered by mistake (CONFLICT once it had contracts, jobs or IoT). */
export async function deleteUnit(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("units.delete", { id, reason }, write(version)); return null; });
}
export async function setUnitPolicies(unitId: string, version: number, alertPolicyIds: string[]) {
  return run(async () => { await coreOp("units.setAlertPolicies", { unitId, alertPolicyIds }, write(version)); return null; });
}
/** policies.setDefaultRule for this customer (version 0 creates the setting). */
export async function setDefaultRule(policyId: string, ruleKey: string, customerId: string, enabled: boolean, version: number) {
  return run(async () => { await coreOp("policies.setDefaultRule", { policyId, ruleKey, customerId, enabled }, write(version)); return null; });
}

// ---- client users (FR-A17): adding (the account at once, IR461), role / sign-in changes with a reason, removal, set-up
// e-mail again and reset previews ----
export async function inviteClientUser(customerId: string, email: string, clientRole: "owner" | "member", displayName: string) {
  return run(async () => { const u = await coreOp<{ id: string }>("clientUsers.save", { customerId, clientRole, email: email.trim(), displayName: displayName.trim() }, write()); return u.id; });
}
export async function updateClientUser(id: string, version: number, input: { customerId: string; email: string; clientRole: "owner" | "member"; status?: "active" | "disabled"; reason: string }) {
  return run(async () => { await coreOp("clientUsers.save", { id, ...input, reason: input.reason.trim() }, write(version)); return null; });
}
export async function removeClientUser(id: string, version: number, reason: string) {
  return run(async () => { await coreOp("clientUsers.remove", { id, reason: reason.trim() }, write(version)); return null; });
}
/** members.revokeSessions for the client user's membership: every session of the user ends at once; the access stays
(Figma Admin 02-15 → 03-8, IR384, IR385). */
export async function signOutEverywhere(membershipId: string, reason: string) {
  return run(async () => { await coreOp("members.revokeSessions", { membershipId, reason: reason.trim() }, write()); return null; });
}
/** clientUsers.resendInvite sends the set-up e-mail again, through the identity provider (IR461). */
export async function resendInvite(id: string) {
  return run(async () => { await coreOp("clientUsers.resendInvite", { id }, { write: true }); return null; });
}
/** auth.previewPasswordReset answers the same generic preview for every address (IR145). */
export async function previewPasswordReset(email: string) {
  return run(async () => { await coreOp("auth.previewPasswordReset", { demoEmail: email }); return null; });
}

// ---- CSV import (FR-A18): preview (writes nothing), import once, undo within 24 h ----
export async function importPreview(customerId: string, fileName: string, csvText: string, mapping: Record<string, string>) {
  return run(() => coreOp<ApiImportPreview>("units.importPreview", { customerId, fileName, csvText, mapping }));
}
export async function importCommit(previewId: string, customerId: string) {
  return run(() => coreOp<ApiUnitImport>("units.importCommit", { previewId, customerId }, write()));
}
export async function importUndo(importId: string, version: number, reason: string) {
  return run(() => coreOp<ApiUnitImport>("units.importUndo", { importId, reason: reason.trim() }, write(version)));
}

// ---- warranty (FR-A19) ----
export async function recordWarrantyClaim(jobId: string, version: number, partLabel: string, amountMinor: number, currency: OpInput<"jobs.recordWarrantyClaim">["currency"], reason: string) {
  return run(async () => { await coreOp("jobs.recordWarrantyClaim", { jobId, amountMinor, currency, partLabel: partLabel.trim(), reason: reason.trim() }, write(version)); return null; });
}

