// Company users of a contractor (FR-P11, DD-P11, IR416): the PartnerUser rows of partnerUsers.list as table rows, the
// invite form's checks and the actions each row offers. Pure code, shared by the partner Users page in API mode and
// its Phase 1A demo (partnerUsersDemo.ts). Texts in the display language, times in the display time zone (IR267).
import { EN, showDate, showTime, type I18n } from "@ac/web/lib/i18n";

export type PartnerRole = "contractor" | "technician";
export type PartnerStatus = "invited" | "active" | "disabled";
/** PartnerUser of service-contracts.ts. */
export type ApiPartnerUser = {
  id: string; version: number; organizationId: string; membershipId: string | null; email: string; displayName: string | null; partnerRole: PartnerRole;
  status: PartnerStatus; lastSignInAt: string | null; accessUntil: string | null; invitedAt: string; invitedByMembershipId: string;
  /** an active technician who holds an assigned job (IR432); absent in the demo data */
  booked?: boolean;
};
/** What a row may do: nothing on the caller's own account; the last active contractor, and a technician who holds an
 * assigned job (IR421, IR432), cannot be disabled or removed. */
export type RowActions = { resend: boolean; edit: boolean; disable: boolean; enable: boolean; remove: boolean };
export type PartnerUserRow = {
  id: string; version: number; email: string; name: string | null; role: PartnerRole; status: PartnerStatus; sub: string; access: string; lastSignIn: string;
  you: boolean; lastContractor: boolean; booked: boolean; ended: boolean; actions: RowActions;
};

export const PARTNER_ROLES: PartnerRole[] = ["contractor", "technician"];
export const roleLabel: Record<PartnerRole, string> = { contractor: "Contractor", technician: "Technician" };
export const statusLabel: Record<PartnerStatus, string> = { invited: "Invite pending", active: "Active", disabled: "Disabled" };
export const PARTNER_STATUSES: PartnerStatus[] = ["active", "invited", "disabled"];

/** The rows by name (an invite by its e-mail), the caller's own account marked and without actions. An active account
 * whose access end has passed by `now` (the business clock) reads as ended: nothing to disable, only remove. */
export function partnerUserRows(us: ApiPartnerUser[], self: string | null, i: I18n = EN, now?: Date): PartnerUserRow[] {
  const { t } = i;
  const contractors = us.filter((u) => u.partnerRole === "contractor" && u.status === "active").length;
  return [...us].sort((a, b) => (a.displayName ?? a.email).localeCompare(b.displayName ?? b.email)).map((u) => {
    const you = !!self && u.membershipId === self;
    const lastContractor = u.partnerRole === "contractor" && u.status === "active" && contractors === 1;
    const invited = u.status === "invited";
    const ended = u.status === "active" && !!now && !!u.accessUntil && Date.parse(u.accessUntil) <= now.getTime();
    const booked = !!u.booked && u.status === "active";
    return {
      id: u.id, version: u.version, email: u.email, name: u.displayName, role: u.partnerRole, status: u.status, you, lastContractor, booked, ended,
      sub: u.displayName ? u.email : t("invited {date}", { date: showDate(u.invitedAt, i.display) }),
      access: u.accessUntil ? showDate(u.accessUntil, i.display) : invited ? "—" : t("No end date"),
      lastSignIn: u.lastSignInAt ? showTime(u.lastSignInAt, i.display) : "—",
      actions: you
        ? { resend: false, edit: false, disable: false, enable: false, remove: false }
        : { resend: invited, edit: invited, disable: u.status === "active" && !lastContractor && !booked && !ended, enable: u.status === "disabled", remove: !lastContractor && !booked },
    };
  });
}

/** A plain address up to 254 characters, not yet used by another account of the company (case-insensitive). */
export function partnerInviteError(email: string, us: { id: string; email: string }[], t = EN.t, self?: string): string | undefined {
  const e = email.trim();
  if (e.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e)) return t("A valid email address (up to 254 characters)");
  if (us.some((u) => u.id !== self && u.email.toLowerCase() === e.toLowerCase())) return t("Already a user of your company");
  return undefined;
}

/** A reason of 1–1000 characters (trimmed), as partnerUsers.save and partnerUsers.remove require. */
export function reasonError(reason: string, t = EN.t): string | undefined {
  const n = [...reason.trim()].length;
  return n < 1 || n > 1000 ? t("A reason of 1–1000 characters") : undefined;
}

/** The status filter of the URL (`?status=`), or all. */
export const statusFilter = (v: string | undefined): PartnerStatus | null => (PARTNER_STATUSES as string[]).includes(v ?? "") ? (v as PartnerStatus) : null;
