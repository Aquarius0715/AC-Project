import type { IconName } from "@ac/web/components/Icon";
import type { Display } from "@ac/web/lib/i18n";

export type Role = "client" | "admin" | "contractor" | "technician";

export type NavItem = { href: string; label: string; icon: IconName; badge?: string; match?: string[] };
/** The shell in API mode (lib/shell.ts, IR241): the signed-in user, the membership's organization, the sidebar badge
 * counts by href and the user's display language and time zone (IR258, IR259). The demo keeps the fixed scope, chip
 * and badges below. */
export type ShellLive = { user: string; organization: string; badges: Record<string, number>; display: Display };

export type RoleConfig = {
  role: Role;
  base: string;
  app: string;
  sub: string;
  scope: string;
  chip: string;
  nav: NavItem[];
  loginUser: string;
  loginDesc: string;
  loginTitle: string; // the service name on its sign-in page (Figma Login frames)
  loginSub: string;
};

export const ROLES: Record<Role, RoleConfig> = {
  client: {
    role: "client",
    base: "/customer",
    app: "AC Project",
    sub: "Customer app",
    scope: "CUSTOMER-A",
    chip: "Client — customer-a",
    loginUser: "customer-a",
    loginDesc: "Views units, controls temperature, requests maintenance",
    loginTitle: "Client",
    loginSub: "Sign in to manage your units and account",
    nav: [
      { href: "/customer", label: "Overview", icon: "overview" },
      { href: "/customer/properties", label: "Units & locations", icon: "units", match: ["/customer/units"] },
      { href: "/customer/automations", label: "Automations & schedules", icon: "automations" },
      { href: "/customer/energy", label: "Energy & cost", icon: "energy" },
      { href: "/customer/air-quality", label: "Air quality", icon: "airQuality" },
      { href: "/customer/alerts", label: "Alerts", icon: "alerts", badge: "2" },
      { href: "/customer/maintenance", label: "Maintenance", icon: "maintenance" },
      { href: "/customer/payments", label: "Contracts & payments", icon: "billing" },
    ],
  },
  contractor: {
    role: "contractor",
    base: "/partner",
    app: "AC Project",
    sub: "Partner portal",
    scope: "CONTRACTOR-A",
    chip: "Contractor — contractor-a",
    loginUser: "contractor-a",
    loginDesc: "Accepts jobs, assigns own technicians",
    loginTitle: "Partner",
    loginSub: "Sign in to manage jobs and your technicians",
    nav: [
      { href: "/partner", label: "Overview", icon: "overview" },
      { href: "/partner/jobs", label: "Jobs", icon: "jobs", badge: "1" },
      { href: "/partner/schedule", label: "Schedule & assignments", icon: "schedule" },
      { href: "/partner/team", label: "Team & capacity", icon: "team" },
      { href: "/partner/history", label: "Job history", icon: "history" },
      { href: "/partner/payouts", label: "Payouts", icon: "payouts" },
    ],
  },
  technician: {
    role: "technician",
    base: "/technician",
    app: "AC Project",
    sub: "Technician",
    scope: "TECH-EXTERNAL-A",
    chip: "Technician — tech-external-a",
    loginUser: "tech-internal-a",
    loginDesc: "Internal · inspections & work reports",
    loginTitle: "Technician",
    loginSub: "Sign in to view assigned jobs and devices",
    nav: [
      { href: "/technician", label: "Overview", icon: "overview" },
      { href: "/technician?tab=all", label: "Assigned jobs", icon: "jobs", match: ["/technician/jobs"] },
      { href: "/technician/devices", label: "Devices", icon: "devices" },
    ],
  },
  admin: {
    role: "admin",
    base: "/admin",
    app: "AC Project",
    sub: "HQ Admin",
    scope: "HQ TENANT",
    chip: "Admin — hq-operator",
    loginUser: "hq-operator",
    loginDesc: "Manages customers, contracts, restrictions",
    loginTitle: "Admin",
    loginSub: "Sign in to manage customers, contracts, and access",
    nav: [
      { href: "/admin", label: "Overview", icon: "overview" },
      { href: "/admin/units", label: "Customers & units", icon: "customers" },
      { href: "/admin/settings/access", label: "Access & roles", icon: "access" },
      { href: "/admin/devices", label: "Devices & models", icon: "devices" },
      { href: "/admin/alerts", label: "Alert policies", icon: "alertPolicies", badge: "1" },
      { href: "/admin/jobs", label: "Maintenance jobs", icon: "maintenance" },
      { href: "/admin/billing/contracts", label: "Contracts", icon: "contracts" },
      { href: "/admin/billing", label: "Billing", icon: "billing" },
      { href: "/admin/restrictions", label: "Restrictions", icon: "restrictions" },
      { href: "/admin/settings/automation", label: "Automation policies", icon: "automations" },
      { href: "/admin/energy", label: "Energy analysis", icon: "energy" },
      { href: "/admin/mrv", label: "MRV", icon: "mrv" },
      { href: "/admin/offsets", label: "Offsets", icon: "offsets" },
      { href: "/admin/audit", label: "Audit", icon: "audit" },
    ],
  },
};

export const ROLE_KEY = "ac-role";
export const roleFromPath = (p: string): Role | null =>
  p.startsWith("/customer") ? "client" : p.startsWith("/partner") ? "contractor" : p.startsWith("/technician") ? "technician" : p.startsWith("/admin") ? "admin" : null;
