// Sign-in page helpers (FR-X01, DDC-07, SCR-X-login, IR251; Figma Login frames): where Continue goes, the other services,
// and why a sign-in came back (the BFF callback's ?error=, or an expired session).
import { ROLES, type Role } from "@ac/web/lib/nav";

/** returnTo kept only inside this role's area (the BFF validates it again); otherwise the role home. */
export function safeReturnTo(role: Role, asked: string | undefined): string {
  const base = ROLES[role].base;
  return asked && (asked === base || asked.startsWith(base + "/")) && !asked.startsWith("//") ? asked : base;
}

/** API mode: the BFF OIDC sign-in with the demo identity as login hint; the browser demo opens the role home. */
export function continueHref(api: boolean, role: Role, returnTo: string): string {
  const cfg = ROLES[role];
  return api ? `/bff/auth/login?login_hint=${encodeURIComponent(cfg.loginUser)}&returnTo=${encodeURIComponent(returnTo)}` : cfg.base;
}

const ORDER: Role[] = ["client", "admin", "contractor", "technician"];
/** The other services in Figma's order (“Admin, Partner, and Technician”). */
export const otherServices = (role: Role): Role[] => ORDER.filter((r) => r !== role);

const ERRORS: Record<string, string> = {
  state: "The sign-in expired or was opened in another tab — start again.",
  token: "The identity service did not accept the sign-in — try again.",
  membership: "This account has no active AC Project membership.",
  role: "This account belongs to another AC Project service — use that service's own sign-in.",
  expired: "Your session ended — sign in again.",
};
/** Why the browser is back on sign-in, or null for a plain visit. */
export const loginError = (code: string | undefined): string | null => (code ? ERRORS[code] ?? "The sign-in did not complete — try again." : null);
