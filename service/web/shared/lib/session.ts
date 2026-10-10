// BFF session (backend architecture §5, §6): the OIDC tokens and the selected tenant / membership live in an httpOnly
// cookie sealed with AES-256-GCM, so the browser keeps an opaque value that only this server can open and that any
// altered byte invalidates (IR333). The production step is a server-side store (ElastiCache) with revocation.
import "server-only";
import { createCipheriv, createDecipheriv, hkdfSync, randomBytes } from "node:crypto";
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

/** The cookie key: 32 bytes derived from SESSION_SECRET with HKDF-SHA256 (the secret itself is never the key). */
export function cookieKey(): Buffer {
  const secret = process.env.SESSION_SECRET ?? "local-dev-session-secret-change-me";
  return Buffer.from(hkdfSync("sha256", secret, "", "ac bff session cookie", 32));
}

const IV_BYTES = 12, TAG_BYTES = 16;

/** Seals a value for a cookie: "<iv>.<ciphertext and tag>", base64url; a fresh IV every time. */
export function seal(value: object): string {
  const iv = randomBytes(IV_BYTES);
  const c = createCipheriv("aes-256-gcm", cookieKey(), iv);
  const body = Buffer.concat([c.update(JSON.stringify(value), "utf8"), c.final(), c.getAuthTag()]);
  return `${iv.toString("base64url")}.${body.toString("base64url")}`;
}

/** Opens a sealed cookie value; null when it is missing, altered, sealed with another key, or not JSON. */
export function unseal<T>(token: string | undefined): T | null {
  const [iv64, body64, extra] = (token ?? "").split(".");
  if (!iv64 || !body64 || extra !== undefined) return null;
  const iv = Buffer.from(iv64, "base64url"), body = Buffer.from(body64, "base64url");
  if (iv.length !== IV_BYTES || body.length <= TAG_BYTES) return null;
  try {
    const d = createDecipheriv("aes-256-gcm", cookieKey(), iv);
    d.setAuthTag(body.subarray(body.length - TAG_BYTES));
    return JSON.parse(Buffer.concat([d.update(body.subarray(0, body.length - TAG_BYTES)), d.final()]).toString("utf8")) as T;
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
  const s = unseal<Session>(jar.get(SESSION_COOKIE)?.value);
  if (!s) return null;
  if (!needsRefresh(s)) return s;
  const next = await refresh(s);
  if (!next) {
    jar.delete(SESSION_COOKIE);
    return null;
  }
  jar.set(SESSION_COOKIE, seal(next), cookieOptions());
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
