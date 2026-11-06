// HQ access and roles (FR-A03, DATA_SOURCE=API): memberships of HQ, contractor and technician users projected for
// /admin/settings/access, with the 38 canonical permissions (BR-A03, IR107) and the members.save rules (IR members.save
// item 1). Pure code shared by the Server Component and the client view. Texts in the display language; the valid period
// is shown and typed in the display time zone (`t` / `i` / `zone`, IR302).
import { EN, showTime, translator, zonedInstant, zonedParts, type I18n, type T } from "@ac/web/lib/i18n";
import type { OpInput } from "@ac/web/lib/opTypes";
import type { Permission } from "@ac/web/lib/contracts.gen";
import { codePoints, isEmail } from "@ac/web/lib/text";
import { isAfter } from "@ac/web/lib/order";
import { lookup } from "@ac/web/lib/lookup";

const CONTRACTOR = "contractor";
const TECHNICIAN = "technician";
const IDENTITY_WRITE = "identity.write";
const DEVICE_MAINTAIN = "device.maintain";
const CONTROL_DIAGNOSE = "control.diagnose";
const RESTRICTION_OVERRIDE = "restriction.override";
const NO_USER = "00000000-0000-0000-0000-000000000000";

const en = translator("en");

export type Role = "admin" | "contractor" | "technician";
export interface ScopeRef { kind: "tenant" | "organization" | "property" | "unit"; id: string }
/** Membership of service-contracts.ts. */
export interface ApiMember {
  id: string; version: number; updatedAt: string; userId: string; displayName: string; organizationId: string; role: Role | "client"; employment: "internal" | "external" | null;
  permissions: string[]; scopes: ScopeRef[]; scopeVersion: number; validFrom: string; validUntil: string | null;
  /** HQ revoked the access: the sign-in is off until HQ gives the access back (IR508) */
  revokedAt?: string | null;
}
export interface ApiOrganization { id: string; name: string; kind: "customer" | "contractor" | "operator"; status: string }

const PARTNER_ACTIONS = ["partner.accept", "partner.assign", "partner.review"];
/** The permission matrix: one row per resource with Read / Write and independent actions (IR107). */
export interface PermissionRow { res: string; read?: string; write?: string; actions?: string[] }
export const permissionRows: PermissionRow[] = [
  { res: "Dashboard", read: "dashboard.read" },
  { res: "Customers & units", read: "asset.read", write: "asset.write" },
  { res: "Users & roles", read: "identity.read", write: IDENTITY_WRITE },
  { res: "Devices & models", read: "device.read", write: "device.write", actions: [DEVICE_MAINTAIN] },
  { res: "AC control", actions: ["control.execute", CONTROL_DIAGNOSE] },
  { res: "Alerts", read: "alert.read", actions: ["alert.resolve"] },
  { res: "Alert policies", read: "alert.policy.read", write: "alert.policy.write" },
  { res: "Maintenance jobs", read: "job.read", write: "job.write" },
  { res: "Contracts", read: "contract.read", write: "contract.write" },
  { res: "Billing", read: "billing.read", write: "billing.write", actions: ["billing.payment"] },
  { res: "Restrictions", read: "restriction.read", write: "restriction.write", actions: [RESTRICTION_OVERRIDE] },
  { res: "Automation policies", read: "automation.policy.read", write: "automation.policy.write" },
  { res: "Energy", read: "energy.read", write: "energy.write" },
  { res: "MRV", read: "mrv.read", write: "mrv.write", actions: ["mrv.review", "mrv.factors"] },
  { res: "Offsets", read: "offset.read", write: "offset.write" },
  { res: "Audit", read: "audit.read" },
  { res: "Partners", actions: [...PARTNER_ACTIONS] },
];
/** A matrix resource as the screen words it. */
export function resourceWord(res: string, t: T = en): string {
  const words: Record<string, string> = {
    Dashboard: t("Dashboard"), "Customers & units": t("Customers & units"), "Users & roles": t("Users & roles"), "Devices & models": t("Devices & models"), "AC control": t("AC control"),
    Alerts: t("Alerts"), "Alert policies": t("Alert policies"), "Maintenance jobs": t("Maintenance jobs"), Contracts: t("Contracts"), Billing: t("Billing"), Restrictions: t("Restrictions"),
    "Automation policies": t("Automation policies"), Energy: t("Energy"), MRV: t("MRV"), Offsets: t("Offsets"), Audit: t("Audit"), Partners: t("Partners"),
  };
  return lookup(words, res) ?? res;
}
/** A role (and a technician's employment) as the screen words it. */
export const roleWord = (role: Role, employment: ApiMember["employment"], t: T = en) => {
  if (role === "admin") return t("Admin");
  if (role === CONTRACTOR) return t("Contractor");
  return t(employment === "external" ? "Technician · external" : "Technician · internal");
};
export const allPermissions = permissionRows.flatMap((r) => [r.read, r.write, ...(r.actions ?? [])].filter((p): p is string => !!p));
/** A permission this app knows (the matrix is the contract's list); anything else is not sent. */
const isPermission = (p: string): p is Permission => allPermissions.includes(p);
/** Permissions each role may hold (members.save rules). */
export const allowedFor: Record<Role, string[]> = {
  admin: allPermissions, contractor: [...PARTNER_ACTIONS], technician: ["alert.read", "alert.resolve", CONTROL_DIAGNOSE, DEVICE_MAINTAIN],
};
/** The defaults a role choice pre-checks; every permission stays editable within the role's allowed set. */
const adminDefaults = allPermissions.filter((p) => ![...PARTNER_ACTIONS, DEVICE_MAINTAIN, CONTROL_DIAGNOSE, "restriction.write", RESTRICTION_OVERRIDE, IDENTITY_WRITE].includes(p));
export const roleDefaults: Record<Role, string[]> = { admin: adminDefaults, contractor: allowedFor.contractor, technician: allowedFor.technician };
/** Granting these to one's own user is FORBIDDEN (another identity administrator must do it). */
export const selfBlocked = [IDENTITY_WRITE, RESTRICTION_OVERRIDE];

export const isActive = (m: Pick<ApiMember, "validFrom" | "validUntil">, now: Date) => Date.parse(m.validFrom) <= now.getTime() && (!m.validUntil || Date.parse(m.validUntil) > now.getTime());

export interface MemberRow { id: string; version: number; name: string; role: Role; label: string; org: string; active: boolean; revoked: boolean; validity: string; m: ApiMember }
/** The memberships (clients are managed elsewhere), with the valid period in the display time zone. */
export function memberRows(ms: ApiMember[], orgs: ApiOrganization[], now: Date, i: I18n = EN): MemberRow[] {
  const { t } = i;
  const org = new Map(orgs.map((o) => [o.id, o.name]));
  return ms.filter((m): m is ApiMember & { role: Role } => m.role !== "client").map((m) => {
    const orgName = org.get(m.organizationId) ?? t("organization");
    return {
      id: m.id,
      version: m.version,
      name: m.displayName,
      role: m.role,
      org: orgName,
      active: isActive(m, now),
      revoked: !!m.revokedAt,
      validity: validity(m, i),
      label: `${roleWord(m.role, m.employment, t)} · ${orgName}`,
      m,
    };
  });
}

export interface MemberDraft {
  userId: string; organizationId: string; role: Role; employment: "internal" | "external"; permissions: string[]; scopes: ScopeRef[]; validFrom: string; validUntil: string; reason: string;
  /** a person created with the membership instead of an existing user (IR461) */
  creatingUser: boolean; inviteEmail: string; inviteName: string;
}
/** An instant as the datetime-local value of the form in a time zone ("2026-09-14T08:00"). */
export const zonedLocal = (iso: string, zone: string) => {
  const p = zonedParts(iso, zone);
  return `${p.date}T${p.time}`;
};
const zonedFrom = (local: string, zone: string) => zonedInstant(local.slice(0, 10), local.slice(11, 16), zone);

/** The user ID members.save receives with newUser: none yet. */

/** The form of a membership; the valid period is typed in the display time zone (NFR-08). A new one starts `now` (the
business clock), as most do, so the period needs only its end (IR622). */
export function memberDraft(m?: ApiMember, tenantId = "", zone = "Asia/Kuala_Lumpur", now?: string): MemberDraft {
  if (!m || m.role === "client") return { userId: "", organizationId: "", role: CONTRACTOR, employment: "internal", permissions: roleDefaults.contractor, scopes: [], validFrom: now ? zonedLocal(now, zone) : "", validUntil: "", reason: "", creatingUser: false, inviteEmail: "", inviteName: "" };
  return {
    creatingUser: false, inviteEmail: "", inviteName: "",
    userId: m.userId, organizationId: m.organizationId, role: m.role, employment: m.employment ?? "internal", permissions: m.permissions, scopes: m.role === "admin" && m.scopes.length === 0 && tenantId ? [{ kind: "tenant", id: tenantId }] : m.scopes,
    validFrom: zonedLocal(m.validFrom, zone), validUntil: m.validUntil ? zonedLocal(m.validUntil, zone) : "", reason: "",
  };
}

/** The organization kind each role (and employment) belongs to. */
export const orgKind = (role: Role, employment: MemberDraft["employment"]): ApiOrganization["kind"] => (role === CONTRACTOR || (role === TECHNICIAN && employment === "external") ? CONTRACTOR : "operator");

/** The fields of the form with a check, top to bottom: a refused save focuses the first one with an error (IR622). */
export const MEMBER_FIELDS = ["newUser.displayName", "newUser.email", "userId", "organizationId", "scopes", "validFrom", "validUntil", "permissions", "reason"] as const;
/** The element id of a checked field of the form. */
export const memberFieldId = (k: string) => `member-${k.replace(".", "-")}`;

export function memberErrors(d: MemberDraft, ctx: { selfUserId: string; current?: ApiMember }, t: T = en): Record<string, string> {
  const e: Record<string, string> = {};
  if (d.creatingUser) { // IR461: the person is created with the membership
    const email = d.inviteEmail.trim();
    if (email.length > 254 || !isEmail(email)) e["newUser.email"] = t("A valid email address (up to 254 characters)");
    const n = codePoints(d.inviteName.trim());
    if (n < 1 || n > 120) e["newUser.displayName"] = t("A name of 1–120 characters");
  }
  if (!d.creatingUser && !d.userId) e.userId = t("Choose a user");
  if (!d.organizationId) e.organizationId = t("Choose the organization");
  const bad = d.permissions.filter((p) => !allowedFor[d.role].includes(p));
  if (bad.length > 0) e.permissions = t("Not allowed for this role: {list}", { list: bad.join(", ") });
  for (const r of permissionRows) if (r.write && d.permissions.includes(r.write) && r.read && !d.permissions.includes(r.read)) e.permissions = t("{write} needs {read}", { write: r.write, read: r.read });
  const gained = d.permissions.filter((p) => selfBlocked.includes(p) && !(ctx.current?.permissions ?? []).includes(p));
  if (!d.creatingUser && d.userId === ctx.selfUserId && gained.length > 0) e.permissions = t("Another identity administrator must grant {list} to you", { list: gained.join(", ") });
  if (d.role === TECHNICIAN && d.scopes.length === 0) e.scopes = t("Add at least one unit, property or customer organization");
  if (!d.validFrom) e.validFrom = t("Required");
  if (d.validUntil && d.validFrom && !isAfter(d.validUntil, d.validFrom)) e.validUntil = t("Must be after valid from");
  if (d.role === TECHNICIAN && d.employment === "external" && !d.validUntil) e.validUntil = t("External technicians need an end date");
  if (codePoints(d.reason.trim()) === 0 || d.reason.length > 1000) e.reason = t("A change reason is required (1–1000 characters)");
  return e;
}

/** members.save input; the valid period typed in `zone` becomes instants. */
export function memberInput(d: MemberDraft, id?: string, zone = "Asia/Kuala_Lumpur"): OpInput<"members.save"> {
  return {
    ...(id && { id }), organizationId: d.organizationId, role: d.role, employment: d.role === TECHNICIAN ? d.employment : null,
    // a new person goes as newUser, with no user ID yet (IR461)
    ...(!id && d.creatingUser ? { userId: NO_USER, "newUser": { email: d.inviteEmail.trim(), displayName: d.inviteName.trim() } } : { userId: d.userId }),
    permissions: d.permissions.filter(isPermission), scopes: d.scopes, validFrom: zonedFrom(d.validFrom, zone), validUntil: d.validUntil ? zonedFrom(d.validUntil, zone) : null, reason: d.reason.trim(),
  };
}

/** The valid period in the display time zone. */
export const validity = (m: Pick<ApiMember, "validFrom" | "validUntil">, i: I18n = EN) => `${showTime(m.validFrom, i.display)} → ${m.validUntil ? showTime(m.validUntil, i.display) : i.t("open-ended")}`;
