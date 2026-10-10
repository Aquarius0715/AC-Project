// HQ access and roles (FR-A03, DATA_SOURCE=api): memberships of HQ, contractor and technician users projected for
// /admin/settings/access, with the 38 canonical permissions (BR-A03, IR107) and the members.save rules (IR members.save
// item 1). Pure code shared by the Server Component and the client view. Texts in the display language; the valid period
// is shown and typed in the display time zone (`t` / `i` / `zone`, IR302).
import { EN, showTime, translator, zonedInstant, zonedParts, type I18n, type T } from "@ac/web/lib/i18n";
import type { OpInput } from "@ac/web/lib/opTypes";
import type { Permission } from "@ac/web/lib/contracts.gen";

const en = translator("en");

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
/** A matrix resource as the screen words it. */
export function resourceWord(res: string, t: T = en): string {
  const words: Record<string, string> = {
    Dashboard: t("Dashboard"), "Customers & units": t("Customers & units"), "Users & roles": t("Users & roles"), "Devices & models": t("Devices & models"), "AC control": t("AC control"),
    Alerts: t("Alerts"), "Alert policies": t("Alert policies"), "Maintenance jobs": t("Maintenance jobs"), Contracts: t("Contracts"), Billing: t("Billing"), Restrictions: t("Restrictions"),
    "Automation policies": t("Automation policies"), Energy: t("Energy"), MRV: t("MRV"), Offsets: t("Offsets"), Audit: t("Audit"), Partners: t("Partners"),
  };
  return words[res] ?? res;
}
/** A role (and a technician's employment) as the screen words it. */
export const roleWord = (role: Role, employment: ApiMember["employment"], t: T = en) =>
  role === "admin" ? t("Admin") : role === "contractor" ? t("Contractor") : employment === "external" ? t("Technician · external") : t("Technician · internal");
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

export type MemberRow = { id: string; version: number; name: string; role: Role; label: string; org: string; active: boolean; validity: string; m: ApiMember };
/** The memberships (clients are managed elsewhere), with the valid period in the display time zone. */
export function memberRows(ms: ApiMember[], orgs: ApiOrganization[], now: Date, i: I18n = EN): MemberRow[] {
  const { t } = i;
  const org = new Map(orgs.map((o) => [o.id, o.name]));
  return ms.filter((m): m is ApiMember & { role: Role } => m.role !== "client").map((m) => {
    const orgName = org.get(m.organizationId) ?? t("organization");
    return {
      id: m.id, version: m.version, name: m.displayName, role: m.role, m, org: orgName, active: isActive(m, now), validity: validity(m, i),
      label: `${roleWord(m.role, m.employment, t)} · ${orgName}`,
    };
  });
}

export type MemberDraft = {
  userId: string; organizationId: string; role: Role; employment: "internal" | "external"; permissions: string[]; scopes: ScopeRef[]; validFrom: string; validUntil: string; reason: string;
};
/** An instant as the datetime-local value of the form in a time zone ("2026-09-14T08:00"). */
export const zonedLocal = (iso: string, zone: string) => {
  const p = zonedParts(iso, zone);
  return `${p.date}T${p.time}`;
};
const zonedFrom = (local: string, zone: string) => zonedInstant(local.slice(0, 10), local.slice(11, 16), zone);

/** The form of a membership; the valid period is typed in the display time zone (NFR-08). */
export function memberDraft(m?: ApiMember, tenantId = "", zone = "Asia/Kuala_Lumpur"): MemberDraft {
  if (!m || m.role === "client") return { userId: "", organizationId: "", role: "contractor", employment: "internal", permissions: roleDefaults.contractor, scopes: [], validFrom: "", validUntil: "", reason: "" };
  return {
    userId: m.userId, organizationId: m.organizationId, role: m.role, employment: m.employment ?? "internal", permissions: m.permissions, scopes: m.role === "admin" && m.scopes.length === 0 && tenantId ? [{ kind: "tenant", id: tenantId }] : m.scopes,
    validFrom: zonedLocal(m.validFrom, zone), validUntil: m.validUntil ? zonedLocal(m.validUntil, zone) : "", reason: "",
  };
}

/** The organization kind each role (and employment) belongs to. */
export const orgKind = (role: Role, employment: MemberDraft["employment"]): ApiOrganization["kind"] => (role === "contractor" || (role === "technician" && employment === "external") ? "contractor" : "operator");

export function memberErrors(d: MemberDraft, ctx: { selfUserId: string; current?: ApiMember }, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (!d.userId) e.userId = t("Choose a user");
  if (!d.organizationId) e.organizationId = t("Choose the organization");
  const bad = d.permissions.filter((p) => !allowedFor[d.role].includes(p));
  if (bad.length) e.permissions = t("Not allowed for this role: {list}", { list: bad.join(", ") });
  for (const r of permissionRows) if (r.write && d.permissions.includes(r.write) && r.read && !d.permissions.includes(r.read)) e.permissions = t("{write} needs {read}", { write: r.write, read: r.read });
  const gained = d.permissions.filter((p) => selfBlocked.includes(p) && !(ctx.current?.permissions ?? []).includes(p));
  if (d.userId === ctx.selfUserId && gained.length) e.permissions = t("Another identity administrator must grant {list} to you", { list: gained.join(", ") });
  if (d.role === "technician" && d.scopes.length === 0) e.scopes = t("Add at least one unit, property or customer organization");
  if (!d.validFrom) e.validFrom = t("Required");
  if (d.validUntil && d.validFrom && d.validUntil <= d.validFrom) e.validUntil = t("Must be after valid from");
  if (d.role === "technician" && d.employment === "external" && !d.validUntil) e.validUntil = t("External technicians need an end date (IR74)");
  if (d.reason.trim().length < 1 || d.reason.length > 1000) e.reason = t("A change reason is required (1–1000 characters)");
  return e;
}

/** members.save input; the valid period typed in `zone` becomes instants. */
export function memberInput(d: MemberDraft, id?: string, zone = "Asia/Kuala_Lumpur"): OpInput<"members.save"> {
  return {
    ...(id ? { id } : {}), userId: d.userId, organizationId: d.organizationId, role: d.role, employment: d.role === "technician" ? d.employment : null,
    permissions: d.permissions as Permission[], scopes: d.scopes, validFrom: zonedFrom(d.validFrom, zone), validUntil: d.validUntil ? zonedFrom(d.validUntil, zone) : null, reason: d.reason.trim(),
  };
}

/** The valid period in the display time zone. */
export const validity = (m: Pick<ApiMember, "validFrom" | "validUntil">, i: I18n = EN) => `${showTime(m.validFrom, i.display)} → ${m.validUntil ? showTime(m.validUntil, i.display) : i.t("open-ended")}`;
