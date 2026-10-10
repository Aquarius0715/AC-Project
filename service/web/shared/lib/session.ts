// BFF session (backend architecture §5): the OIDC tokens and the selected tenant / membership live in an httpOnly,
// HMAC-signed cookie that only the server reads. Browsers never see the access token.
import "server-only";
import { createHmac, timingSafeEqual } from "node:crypto";
import { cookies } from "next/headers";

export type Session = {
  accessToken: string;
  refreshToken: string | null;
  tenantId: string;
  membershipId: string;
  role: "client" | "contractor" | "technician" | "admin";
  expiresAt: number; // epoch seconds of the access token
};

// One web app per role (IR178). AC_APP_ROLE is set at build time by each app's next.config; the cookie names carry
// it because local apps share the host "localhost" (cookies are not separated by port).
export const APP_ROLE = (process.env.AC_APP_ROLE ?? "client") as Session["role"];
export const SESSION_COOKIE = `ac_session_${APP_ROLE}`;
export const OIDC_COOKIE = `ac_oidc_${APP_ROLE}`;

function secret(): string {
  return process.env.SESSION_SECRET ?? "local-dev-session-secret-change-me";
}

export function sign(value: object): string {
  const body = Buffer.from(JSON.stringify(value)).toString("base64url");
  const mac = createHmac("sha256", secret()).update(body).digest("base64url");
  return `${body}.${mac}`;
}

export function verify<T>(token: string | undefined): T | null {
  if (!token) return null;
  const parts = token.split(".");
  if (parts.length !== 2) return null;
  const [body, mac] = parts;
  if (!body || !mac) return null;
  const want = createHmac("sha256", secret()).update(body).digest();
  const got = Buffer.from(mac, "base64url");
  if (got.length !== want.length || !timingSafeEqual(got, want)) return null;
  try {
    return JSON.parse(Buffer.from(body, "base64url").toString()) as T;
  } catch {
    return null;
  }
}

// The cookie outlives the access token; readSession refreshes the token shortly before it expires (refresh_token
// grant) and rewrites the cookie, so a session lasts as long as the identity provider's SSO session.
const SESSION_MAX_AGE = 30 * 60;
const REFRESH_LEEWAY_SECONDS = 30;

export function cookieOptions() {
  return { httpOnly: true, sameSite: "lax" as const, secure: oidcConfig().appUrl.startsWith("https"), path: "/", maxAge: SESSION_MAX_AGE };
}

/** Builds a session from a token response; null when the token lacks the tenant / membership / role claims. */
export function sessionFromTokens(t: { access_token: string; refresh_token?: string; expires_in: number }, previousRefresh: string | null = null): Session | null {
  const claims = jwtClaims(t.access_token);
  const role = claims.role as Session["role"] | undefined;
  // own keys only: `in` would also accept inherited names such as "constructor"
  if (!claims.tenant_id || !claims.membership_id || typeof role !== "string" || !Object.hasOwn(roleHome, role)) return null;
  return {
    accessToken: t.access_token, refreshToken: t.refresh_token ?? previousRefresh, tenantId: String(claims.tenant_id), membershipId: String(claims.membership_id),
    role, expiresAt: Math.floor(Date.now() / 1000) + t.expires_in,
  };
}

/** Exchanges the refresh token for a new access token (same tenant and membership), or null. */
export async function refresh(s: Session): Promise<Session | null> {
  if (!s.refreshToken) return null;
  const cfg = oidcConfig();
  try {
    const r = await fetch(`${cfg.internalIssuer}/protocol/openid-connect/token`, {
      method: "POST", cache: "no-store",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ grant_type: "refresh_token", refresh_token: s.refreshToken, client_id: cfg.clientId, client_secret: cfg.clientSecret }),
    });
    if (!r.ok) return null;
    const next = sessionFromTokens(await r.json(), s.refreshToken);
    // a refreshed token must keep the selected tenant and membership
    return next && next.tenantId === s.tenantId && next.membershipId === s.membershipId ? next : null;
  } catch {
    return null;
  }
}

/** True when the access token expires within the refresh leeway. */
export function needsRefresh(s: Session): boolean {
  return s.expiresAt - REFRESH_LEEWAY_SECONDS <= Date.now() / 1000;
}

/** The signed-in session (route handlers only: refreshing rewrites or clears the cookie). */
export async function readSession(): Promise<Session | null> {
  const jar = await cookies();
  const s = verify<Session>(jar.get(SESSION_COOKIE)?.value);
  if (!s) return null;
  if (!needsRefresh(s)) return s;
  const next = await refresh(s);
  if (!next) {
    jar.delete(SESSION_COOKIE);
    return null;
  }
  jar.set(SESSION_COOKIE, sign(next), cookieOptions());
  return next;
}

/** A same-origin path to return to after sign-in; rejects absolute, protocol-relative ("//host") and backslash forms. */
export function safeReturnTo(v: string | null | undefined): string | null {
  if (!v || !v.startsWith("/") || v.startsWith("//") || v.includes("\\") || /[\u0000-\u001f]/.test(v)) return null;
  return v;
}

export const roleHome: Record<Session["role"], string> = { client: "/customer", contractor: "/partner", technician: "/technician", admin: "/admin" };

/** Decodes a JWT payload without verifying it (the Core API verifies every token against the JWKS). */
export function jwtClaims(token: string): Record<string, unknown> {
  const part = token.split(".")[1] ?? "";
  try {
    return JSON.parse(Buffer.from(part, "base64url").toString()) as Record<string, unknown>;
  } catch {
    return {};
  }
}

export function oidcConfig() {
  const issuer = process.env.OIDC_ISSUER ?? "http://localhost:8081/realms/ac";
  return {
    issuer, // browser-facing issuer (authorize redirect)
    internalIssuer: process.env.OIDC_INTERNAL_ISSUER ?? issuer, // server-to-server token endpoint
    clientId: process.env.OIDC_APP_CLIENT_ID ?? "ac-web",
    clientSecret: process.env.OIDC_APP_CLIENT_SECRET ?? "local-ac-web-secret",
    appUrl: process.env.APP_URL ?? "http://localhost:3000",
  };
}
