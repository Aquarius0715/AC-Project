// Sign-in page helpers (FR-X01, DDC-07, SCR-X-login, IR251, IR790; Figma Login frames): where a sign-in returns to, and
// why it came back (the BFF callback's ?error=, or an expired session).
import { roleConfig, type Role } from "@ac/web/lib/nav";
import { lookup } from "@ac/web/lib/lookup";

/** returnTo kept only inside this role's area (the BFF validates it again); otherwise the role home. */
export function safeReturnTo(role: Role, asked: string | undefined): string {
  const base = roleConfig(role).base;
  return asked && (asked === base || asked.startsWith(base + "/")) && !asked.startsWith("//") ? asked : base;
}

/** The browser demo (no identity service in that build) opens the role home; API mode signs in through the BFF form. */
export const demoHref = (role: Role): string => roleConfig(role).base;

const ERRORS: Record<string, string> = {
  state: "The sign-in expired or was opened in another tab — start again.",
  token: "The identity service did not accept the sign-in — try again.",
  membership: "This account has no active AC Project membership.",
  role: "This account belongs to another AC Project service — use that service's own sign-in.",
  expired: "Your session ended — sign in again.",
  revoked: "You were signed out everywhere — sign in again with your password.",
};
/** Why the Core API refused the session: HQ signed the user out everywhere (IR384), or it ended otherwise. */
export const signedOutReason = (messageKey?: string): "revoked" | "expired" => (messageKey === "errors.session_revoked" ? "revoked" : "expired");

/** Sign-in again after a refused session, back to the page it was on (IR385). */
export const signInAgainHref = (reason: "revoked" | "expired", path: string): string => `/login?error=${reason}&returnTo=${encodeURIComponent(path)}`;

/** Why the browser is back on sign-in, or null for a plain visit. */
export const loginError = (code: string | undefined): string | null => (code ? lookup(ERRORS, code) ?? "The sign-in did not complete — try again." : null);
