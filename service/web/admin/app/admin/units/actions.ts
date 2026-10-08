"use server";

// Server Actions of the HQ customers & units screen (FR-A02). Each one is a public endpoint: the DAL verifies the
// session and the Core API authorizes it (asset.write; alert.policy.write for default rules) and checks the version of
// the record it changes.
import { refresh } from "next/cache";
import { coreAll, coreOp, CoreError } from "@ac/web/lib/dal";
import type { ApiCustomerRow, ApiOrgRow, Profile, Scope } from "@ac/web/lib/assets";

export type ActionResult<T = null> = { ok: true; value: T } | { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> };

async function run<T>(fn: () => Promise<T>): Promise<ActionResult<T>> {
  try {
    return { ok: true, value: await fn() };
  } catch (e) {
    if (e instanceof CoreError) return { ok: false, messageKey: e.error.messageKey, code: e.error.code, fieldErrors: e.error.fieldErrors ?? {} };
    return { ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} };
  } finally {
    refresh();
  }
}
const write = (version?: number) => (version === undefined ? { write: true } : { write: true, expectedVersion: version });

export type CustomerInput = { billingName: string; name: string; serviceProfile: Profile; status: "active" | "inactive" };
/** organizations.save (kind customer) and then customers.save. A billing name already used by an organization that
 * never got its customer (an earlier attempt that stopped half way) is reused instead of failing as a duplicate. */
export async function createCustomer(input: CustomerInput) {
  return run(async () => {
    const billingName = input.billingName.trim();
    let org: ApiOrgRow;
    try {
      org = await coreOp<ApiOrgRow>("organizations.save", { name: billingName, kind: "customer", status: "active" }, write());
    } catch (e) {
      if (!(e instanceof CoreError && e.error.messageKey === "error.duplicateName")) throw e;
      const same = (await coreAll<ApiOrgRow>("organizations.list", { filters: { kind: "customer" } })).find((o) => o.name.trim().toLowerCase() === billingName.toLowerCase());
      const taken = same && (await Promise.all((["active", "inactive"] as const).map((status) => coreAll<ApiCustomerRow>("customers.list", { filters: { organizationId: same.id, status } })))).flat().length > 0;
      if (!same || taken) throw e;
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

export type PropertyInput = { id?: string; customerOrgId: string; kind: "home" | "office"; name: string; address: string | null; accessInstructions: string | null };
export async function saveProperty(input: PropertyInput, version?: number) {
  return run(() => coreOp<{ id: string }>("properties.save", input, write(version)).then((p) => p.id));
}
export type SpaceInput = { id?: string; propertyId: string; parentSpaceId: string | null; kind: "area" | "floor" | "room" | "space"; name: string };
export async function saveSpace(input: SpaceInput, version?: number) {
  return run(() => coreOp<{ id: string }>("spaces.save", input, write(version)).then((s) => s.id));
}
/** properties.archive / spaces.archive (CONFLICT while it still holds units or child spaces). */
export async function archiveLocation(kind: "property" | "space", id: string, version: number, reason: string) {
  return run(() => coreOp(kind === "property" ? "properties.archive" : "spaces.archive", { id, reason }, write(version)).then(() => null));
}

export type UnitInput = {
  id?: string; customerOrgId: string; propertyId: string; spaceId: string | null; displayName: string; modelId: string; type: "split"; installedAt: string | null;
  serviceScope: Scope[]; changeReason?: string;
};
export async function saveUnit(input: UnitInput, version?: number) {
  return run(() => coreOp<{ id: string }>("units.save", input, write(version)).then((u) => u.id));
}
/** units.archive takes the unit out of use and keeps its ID and history (D05: CONFLICT while it is still in use). */
export async function archiveUnit(id: string, version: number, reason: string) {
  return run(() => coreOp("units.archive", { id, reason }, write(version)).then(() => null));
}
/** units.delete removes a unit registered by mistake (CONFLICT once it had contracts, jobs or IoT). */
export async function deleteUnit(id: string, version: number, reason: string) {
  return run(() => coreOp("units.delete", { id, reason }, write(version)).then(() => null));
}
export async function setUnitPolicies(unitId: string, version: number, alertPolicyIds: string[]) {
  return run(() => coreOp("units.setAlertPolicies", { unitId, alertPolicyIds }, write(version)).then(() => null));
}
/** policies.setDefaultRule for this customer (version 0 creates the setting). */
export async function setDefaultRule(policyId: string, ruleKey: string, customerId: string, enabled: boolean, version: number) {
  return run(() => coreOp("policies.setDefaultRule", { policyId, ruleKey, customerId, enabled }, write(version)).then(() => null));
}
