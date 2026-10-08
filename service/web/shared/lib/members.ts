// HQ access and roles (FR-A03, DATA_SOURCE=api): memberships of HQ, contractor and technician users projected for
// /admin/settings/access, with the 38 canonical permissions (BR-A03, IR107) and the members.save rules (IR members.save
// item 1). Pure code shared by the Server Component and the client view.
import { klInstant, klLocal, klStamp } from "@ac/web/lib/energy";

export type Role = "admin" | "contractor" | "technician";
export type ScopeRef = { kind: "tenant" | "organization" | "property" | "unit"; id: string };
/** Membership of service-contracts.ts. */
export type ApiMember = {
  id: string; version: number; updatedAt: string; userId: string; displayName: string; organizationId: string; role: Role | "client"; employment: "internal" | "external" | null;
  permissions: string[]; scopes: ScopeRef[]; scopeVersion: number; validFrom: string; validUntil: string | null;
};
export type ApiOrganization = { id: string; name: string; kind: "customer" | "contractor" | "operator"; status: string };

/** The permission matrix: one row per resource with Read / Write and independent actions (IR107). */
export type PermissionRow = { res: string; read?: string; write?: string; actions?: string[] };
export const permissionRows: PermissionRow[] = [
  { res: "Dashboard", read: "dashboard.read" },
  { res: "Customers & units", read: "asset.read", write: "asset.write" },
  { res: "Users & roles", read: "identity.read", write: "identity.write" },
  { res: "Devices & models", read: "device.read", write: "device.write", actions: ["device.maintain"] },
  { res: "AC control", actions: ["control.execute", "control.diagnose"] },
  { res: "Alerts", read: "alert.read", actions: ["alert.resolve"] },
  { res: "Alert policies", read: "alert.policy.read", write: "alert.policy.write" },
  { res: "Maintenance jobs", read: "job.read", write: "job.write" },
  { res: "Contracts", read: "contract.read", write: "contract.write" },
  { res: "Billing", read: "billing.read", write: "billing.write", actions: ["billing.payment"] },
  { res: "Restrictions", read: "restriction.read", write: "restriction.write", actions: ["restriction.override"] },
  { res: "Automation policies", read: "automation.policy.read", write: "automation.policy.write" },
  { res: "Energy", read: "energy.read", write: "energy.write" },
  { res: "MRV", read: "mrv.read", write: "mrv.write", actions: ["mrv.review", "mrv.factors"] },
  { res: "Offsets", read: "offset.read", write: "offset.write" },
  { res: "Audit", read: "audit.read" },
  { res: "Partners", actions: ["partner.accept", "partner.assign", "partner.review"] },
];
export const allPermissions = permissionRows.flatMap((r) => [r.read, r.write, ...(r.actions ?? [])].filter((p): p is string => !!p));
/** Permissions each role may hold (members.save rules). */
export const allowedFor: Record<Role, string[]> = {
  admin: allPermissions, contractor: ["partner.accept", "partner.assign", "partner.review"], technician: ["alert.read", "alert.resolve", "control.diagnose", "device.maintain"],
};
/** The defaults a role choice pre-checks; every permission stays editable within the role's allowed set. */
const adminDefaults = allPermissions.filter((p) => !["partner.accept", "partner.assign", "partner.review", "device.maintain", "control.diagnose", "restriction.write", "restriction.override", "identity.write"].includes(p));
export const roleDefaults: Record<Role, string[]> = { admin: adminDefaults, contractor: allowedFor.contractor, technician: allowedFor.technician };
/** Granting these to one's own user is FORBIDDEN (another identity administrator must do it). */
export const selfBlocked = ["identity.write", "restriction.override"];

export const isActive = (m: Pick<ApiMember, "validFrom" | "validUntil">, now: Date) => Date.parse(m.validFrom) <= now.getTime() && (!m.validUntil || Date.parse(m.validUntil) > now.getTime());

export type MemberRow = { id: string; version: number; name: string; role: Role; label: string; org: string; active: boolean; m: ApiMember };
export function memberRows(ms: ApiMember[], orgs: ApiOrganization[], now: Date): MemberRow[] {
  const org = new Map(orgs.map((o) => [o.id, o.name]));
  return ms.filter((m): m is ApiMember & { role: Role } => m.role !== "client").map((m) => ({
    id: m.id, version: m.version, name: m.displayName, role: m.role, m, org: org.get(m.organizationId) ?? "organization", active: isActive(m, now),
    label: `${m.role === "admin" ? "Admin" : m.role === "contractor" ? "Contractor" : `Technician · ${m.employment}`} · ${org.get(m.organizationId) ?? "organization"}`,
  }));
}

export type MemberDraft = {
  userId: string; organizationId: string; role: Role; employment: "internal" | "external"; permissions: string[]; scopes: ScopeRef[]; validFrom: string; validUntil: string; reason: string;
};
export function memberDraft(m?: ApiMember, tenantId = ""): MemberDraft {
  if (!m || m.role === "client") return { userId: "", organizationId: "", role: "contractor", employment: "internal", permissions: roleDefaults.contractor, scopes: [], validFrom: "", validUntil: "", reason: "" };
  return {
    userId: m.userId, organizationId: m.organizationId, role: m.role, employment: m.employment ?? "internal", permissions: m.permissions, scopes: m.role === "admin" && m.scopes.length === 0 && tenantId ? [{ kind: "tenant", id: tenantId }] : m.scopes,
    validFrom: klLocal(m.validFrom), validUntil: m.validUntil ? klLocal(m.validUntil) : "", reason: "",
  };
}

/** The organization kind each role (and employment) belongs to. */
export const orgKind = (role: Role, employment: MemberDraft["employment"]): ApiOrganization["kind"] => (role === "contractor" || (role === "technician" && employment === "external") ? "contractor" : "operator");

export function memberErrors(d: MemberDraft, ctx: { selfUserId: string; current?: ApiMember }): Record<string, string> {
  const e: Record<string, string> = {};
  if (!d.userId) e.userId = "Choose a user";
  if (!d.organizationId) e.organizationId = "Choose the organization";
  const bad = d.permissions.filter((p) => !allowedFor[d.role].includes(p));
  if (bad.length) e.permissions = `Not allowed for this role: ${bad.join(", ")}`;
  for (const r of permissionRows) if (r.write && d.permissions.includes(r.write) && r.read && !d.permissions.includes(r.read)) e.permissions = `${r.write} needs ${r.read}`;
  const gained = d.permissions.filter((p) => selfBlocked.includes(p) && !(ctx.current?.permissions ?? []).includes(p));
  if (d.userId === ctx.selfUserId && gained.length) e.permissions = `Another identity administrator must grant ${gained.join(" and ")} to you`;
  if (d.role === "technician" && d.scopes.length === 0) e.scopes = "Add at least one unit, property or customer organization";
  if (!d.validFrom) e.validFrom = "Required";
  if (d.validUntil && d.validFrom && d.validUntil <= d.validFrom) e.validUntil = "Must be after valid from";
  if (d.role === "technician" && d.employment === "external" && !d.validUntil) e.validUntil = "External technicians need an end date (IR74)";
  if (d.reason.trim().length < 1 || d.reason.length > 1000) e.reason = "A change reason is required (1–1000 characters)";
  return e;
}

export function memberInput(d: MemberDraft, id?: string) {
  return {
    ...(id ? { id } : {}), userId: d.userId, organizationId: d.organizationId, role: d.role, employment: d.role === "technician" ? d.employment : null,
    permissions: d.permissions, scopes: d.scopes, validFrom: klInstant(d.validFrom), validUntil: d.validUntil ? klInstant(d.validUntil) : null, reason: d.reason.trim(),
  };
}

export const validity = (m: ApiMember) => `${klStamp(m.validFrom)} → ${m.validUntil ? klStamp(m.validUntil) : "open-ended"}`;
