// Sign-in page helpers (FR-X01, DDC-07, SCR-X-login, IR251, IR646; Figma Login frames): the e-mail hint and language the
// BFF passes to the identity provider, the other services, and why a sign-in came back (the BFF callback's ?error=, or
// an expired session).
import { roleConfig, type Role } from "@ac/web/lib/nav";
import { lookup } from "@ac/web/lib/lookup";
import { isLocale, type Locale } from "@ac/web/lib/i18n";
import { isEmail } from "@ac/web/lib/text";

/** returnTo kept only inside this role's area (the BFF validates it again); otherwise the role home. */
export function safeReturnTo(role: Role, asked: string | undefined): string {
  const base = roleConfig(role).base;
  return asked && (asked === base || asked.startsWith(base + "/")) && !asked.startsWith("//") ? asked : base;
}

/** The browser demo (no identity service in that build) opens the role home; API mode signs in through the BFF form. */
export const demoHref = (role: Role): string => roleConfig(role).base;

/** The cookie that keeps the language chosen on the sign-in page, before there is a session with preferences. */
export const SIGN_IN_LOCALE_COOKIE = "ac_sign_in_locale";

/** Longest e-mail address (RFC 5321). */
const EMAIL_MAX = 320;

/** The address a person typed as the sign-in hint of the identity provider, or undefined when it is not an address. */
export function loginHint(asked: string | null): string | undefined {
  const text = asked?.trim();
  return text && text.length <= EMAIL_MAX && isEmail(text) ? text : undefined;
}

/** The language to ask the identity provider's pages in: one of the display languages, otherwise none. */
export const uiLocale = (asked: string | null | undefined): Locale | undefined => (isLocale(asked) ? asked : undefined);

const ORDER: Role[] = ["client", "admin", "contractor", "technician"];
/** The other services in Figma's order (“Admin, Partner, and Technician”). */
export const otherServices = (role: Role): Role[] => ORDER.filter((r) => r !== role);

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
