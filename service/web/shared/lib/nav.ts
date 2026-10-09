export type Role = "client" | "admin" | "contractor" | "technician";

export type NavItem = { href: string; label: string; icon: string; badge?: string; match?: string[] };
/** The shell in API mode (lib/shell.ts, IR241): the signed-in user, the membership's organization and the sidebar badge
 * counts by href. The demo keeps the fixed scope, chip and badges below. */
export type ShellLive = { user: string; organization: string; badges: Record<string, number> };

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
    nav: [
      { href: "/customer", label: "Overview", icon: "⌂" },
      { href: "/customer/properties", label: "Units & locations", icon: "▤", match: ["/customer/units"] },
      { href: "/customer/automations", label: "Automations & schedules", icon: "◷" },
      { href: "/customer/energy", label: "Energy & cost", icon: "ⓘ" },
      { href: "/customer/air-quality", label: "Air quality", icon: "≋" },
      { href: "/customer/alerts", label: "Alerts", icon: "🔔", badge: "2" },
      { href: "/customer/maintenance", label: "Maintenance", icon: "🔧" },
      { href: "/customer/payments", label: "Contracts & payments", icon: "▣" },
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
    loginDesc: "Answers HQ offers, assigns technicians, reviews work reports",
    nav: [
      { href: "/partner", label: "Overview", icon: "⌂" },
      { href: "/partner/jobs", label: "Jobs", icon: "▤", badge: "1" },
      { href: "/partner/schedule", label: "Schedule & assignments", icon: "◷" },
      { href: "/partner/team", label: "Team & capacity", icon: "☻" },
      { href: "/partner/history", label: "Job history", icon: "↺" },
      { href: "/partner/payouts", label: "Payouts", icon: "▣" },
    ],
  },
  technician: {
    role: "technician",
    base: "/technician",
    app: "AC Project",
    sub: "Technician",
    scope: "TECH-EXTERNAL-A",
    chip: "Technician — tech-external-a",
    loginUser: "tech-external-a",
    loginDesc: "Runs assigned jobs, records inspections, maintains devices",
    nav: [
      { href: "/technician", label: "Overview", icon: "⌂" },
      { href: "/technician?tab=all", label: "Assigned jobs", icon: "▤", match: ["/technician/jobs"] },
      { href: "/technician/devices", label: "Devices", icon: "▥" },
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
    loginDesc: "Operates customers, devices, alerts, billing and reports",
    nav: [
      { href: "/admin", label: "Overview", icon: "⌂" },
      { href: "/admin/units", label: "Customers & units", icon: "▤" },
      { href: "/admin/settings/access", label: "Access & roles", icon: "⚿" },
      { href: "/admin/devices", label: "Devices & models", icon: "▥" },
      { href: "/admin/alerts", label: "Alert policies", icon: "🔔", badge: "1" },
      { href: "/admin/jobs", label: "Maintenance jobs", icon: "🔧" },
      { href: "/admin/billing/contracts", label: "Contracts", icon: "❒" },
      { href: "/admin/billing", label: "Billing", icon: "▣" },
      { href: "/admin/restrictions", label: "Restrictions", icon: "⛔" },
      { href: "/admin/settings/automation", label: "Automation policies", icon: "◷" },
      { href: "/admin/energy", label: "Energy analysis", icon: "ⓘ" },
      { href: "/admin/mrv", label: "MRV", icon: "≋" },
      { href: "/admin/offsets", label: "Offsets", icon: "❀" },
      { href: "/admin/audit", label: "Audit", icon: "☰" },
    ],
  },
};

export const ROLE_KEY = "ac-role";
export const roleFromPath = (p: string): Role | null =>
  p.startsWith("/customer") ? "client" : p.startsWith("/partner") ? "contractor" : p.startsWith("/technician") ? "technician" : p.startsWith("/admin") ? "admin" : null;
