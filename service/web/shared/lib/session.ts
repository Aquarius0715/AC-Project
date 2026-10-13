// BFF session (backend architecture §5, §6): the OIDC tokens and the selected tenant / membership. With SESSION_STORE_URL
// they live in the server-side store (Valkey locally, ElastiCache in production) and the httpOnly cookie holds only a
// sealed random session ID, so sign-out revokes the session at once (IR382). Without it the cookie carries the session
// itself, sealed with AES-256-GCM: an opaque value that only this server can open and that any altered byte invalidates
// (IR333).
import "server-only";
import { createCipheriv, createDecipheriv, createHash, hkdfSync, randomBytes } from "node:crypto";
import { cookies } from "next/headers";
import { createClient } from "@redis/client";

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

/** True when the app is served over HTTPS (APP_URL): its cookies are Secure and carry the cookie prefixes. */
const httpsApp = () => oidcConfig().appUrl.startsWith("https");

// On https the names carry the cookie prefixes: __Host- for the session (Secure, Path=/, no Domain) and __Secure- for
// the sign-in state (Secure, Path=/bff/auth). The browser then keeps them to this host and HTTPS, so a sibling
// subdomain cannot set or shadow them (cookie tossing, IR374). Plain-http local runs keep the plain names.
export const SESSION_COOKIE = `${httpsApp() ? "__Host-" : ""}ac_session_${APP_ROLE}`;
export const OIDC_COOKIE = `${httpsApp() ? "__Secure-" : ""}ac_oidc_${APP_ROLE}`;

/** Shortest SESSION_SECRET a deployment on https accepts. */
export const MIN_SECRET_LENGTH = 32;

/** The local default secret of plain-http runs (compose.yaml uses the same value). */
const LOCAL_SECRET = "local-dev-session-secret-change-me";

/** The cookie key: 32 bytes derived from SESSION_SECRET with HKDF-SHA256 (the secret itself is never the key). On https
 * a missing, empty or short secret is refused rather than replaced by the local one, and so is the public local value
 * (IR430: it is long enough, but it is in the repository), so a deployment without its own secret cannot seal or open
 * a session and fails closed (IR367). Plain-http local runs keep the local default. */
export function cookieKey(): Buffer {
  const secret = process.env.SESSION_SECRET || undefined;
  if (oidcConfig().appUrl.startsWith("https") && (secret === undefined || secret.length < MIN_SECRET_LENGTH || secret.includes("change-me"))) {
    throw new Error(`SESSION_SECRET of at least ${MIN_SECRET_LENGTH} characters, not the local value, is required on https`);
  }
  return Buffer.from(hkdfSync("sha256", secret ?? LOCAL_SECRET, "", "ac bff session cookie", 32));
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
  return { httpOnly: true, sameSite: "lax" as const, secure: httpsApp(), path: "/", maxAge: SESSION_MAX_AGE };
}

/** The attributes that remove the session cookie: the Secure flag and path it was set with. A browser ignores a
 * deletion of a __Host- cookie that lacks them, which is what cookies.delete(name) sends (IR374). */
export function clearedCookieOptions() {
  return { ...cookieOptions(), maxAge: 0 };
}

/** The sign-in state cookie (state, PKCE verifier, return path): on /bff/auth only, for 10 minutes; 0 removes it. */
export function oidcCookieOptions(maxAge = 600) {
  return { httpOnly: true, sameSite: "lax" as const, secure: httpsApp(), path: "/bff/auth", maxAge };
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

/** Ends the identity provider's session of a sign-in (backend architecture §6: sign-out revokes the IdP session and
 * its refresh token, IR357): Keycloak's back-channel logout, the refresh token posted with the client's credentials
 * to the internal logout endpoint. False when the provider refused or did not answer within 3 seconds. */
export async function endIdpSession(refreshToken: string): Promise<boolean> {
  const cfg = oidcConfig();
  try {
    const r = await fetch(`${cfg.internalIssuer}/protocol/openid-connect/logout`, {
      method: "POST", cache: "no-store",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ refresh_token: refreshToken, client_id: cfg.clientId, client_secret: cfg.clientSecret }),
      signal: AbortSignal.timeout(3000),
    });
    return r.ok;
  } catch {
    return false;
  }
}

/** True when the access token expires within the refresh leeway. */
export function needsRefresh(s: Session): boolean {
  return s.expiresAt - REFRESH_LEEWAY_SECONDS <= Date.now() / 1000;
}

// ---- the server-side session store (backend architecture §6, OPEN-CT-03, IR382) ----
// A record per sign-in under the SHA-256 of a random 256-bit session ID, so the keys reveal no cookie. The value is
// sealed with the cookie key, so the store never holds a token in clear. Each read extends the idle lifetime; the
// record ends 12 hours after the sign-in however active it is. The cookie is the sealed ID; a cookie of the other mode
// is refused, so a session sealed into a cookie before the store was configured cannot bypass revocation.

/** The absolute lifetime of a stored session: 12 hours from its sign-in (backend architecture §6). */
export const ABSOLUTE_SECONDS = 12 * 60 * 60;
const STORE_TIMEOUT_MS = 2000;

type Stored = { session: Session; startedAt: number }; // epoch seconds of the sign-in
type StoreClient = {
  getEx(key: string, options: { type: "EX"; value: number }): Promise<string | null>;
  set(key: string, value: string, options: { expiration: { type: "EX"; value: number }; condition?: "XX" }): Promise<string | null>;
  del(key: string): Promise<number>;
};

let client: Promise<StoreClient> | null = null;

/** The store's client, connected once per process; null without SESSION_STORE_URL (the cookie carries the session). A
 * failed connection is tried again by the next request; commands fail at once while it is down (no offline queue). */
function store(): Promise<StoreClient> | null {
  const url = process.env.SESSION_STORE_URL;
  if (!url) return null;
  if (!client) {
    const c = createClient({ url, disableOfflineQueue: true, socket: { connectTimeout: STORE_TIMEOUT_MS, reconnectStrategy: (n: number) => Math.min(100 * 2 ** n, 2000) } });
    c.on("error", (e: Error) => console.warn(`session store: ${e.message}`));
    client = c.connect().then(() => c as unknown as StoreClient, (e: unknown) => {
      client = null;
      throw e;
    });
  }
  return client;
}

/** A store call that gives up after 2 seconds: the session then counts as absent (fail closed). */
const timed = <T,>(p: Promise<T>): Promise<T> => {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const late = new Promise<never>((_, fail) => { timer = setTimeout(() => fail(new Error("session store timeout")), STORE_TIMEOUT_MS); });
  return Promise.race([p, late]).finally(() => clearTimeout(timer));
};
const keyOf = (id: string) => `ac:session:${createHash("sha256").update(id).digest("hex")}`;
const idOf = (cookie: string | undefined) => {
  const id = unseal<{ sid?: unknown }>(cookie)?.sid;
  return typeof id === "string" && id.length >= 43 ? id : null;
};

/** The session a cookie value names: in store mode the live record of its sealed ID (the read extends the idle
 * lifetime), else the sealed session itself. Null when the cookie is missing, altered or of the other mode, when the
 * record was deleted or has expired, or when the store does not answer. */
export async function loadSession(cookie: string | undefined): Promise<Session | null> {
  const c = store();
  if (!c) {
    const s = unseal<Session>(cookie);
    return s && typeof s.accessToken === "string" ? s : null;
  }
  const id = idOf(cookie);
  if (!id) return null;
  try {
    const rec = unseal<Stored>((await timed((await timed(c)).getEx(keyOf(id), { type: "EX", value: SESSION_MAX_AGE }))) ?? undefined);
    if (!rec) return null;
    if (Date.now() / 1000 - rec.startedAt >= ABSOLUTE_SECONDS) {
      await dropSession(cookie);
      return null;
    }
    return rec.session;
  } catch (e) {
    console.warn(`session store: ${(e as Error).message}`);
    return null;
  }
}

/** The cookie value of a new sign-in: store mode stores the session under a new random ID and deletes the record the
 * browser's previous cookie named; cookie mode seals the session. Throws when the store does not answer. */
export async function createSessionCookie(s: Session, previous?: string): Promise<string> {
  const c = store();
  if (!c) return seal(s);
  const id = randomBytes(32).toString("base64url");
  const rec: Stored = { session: s, startedAt: Math.floor(Date.now() / 1000) };
  await timed((await timed(c)).set(keyOf(id), seal(rec), { expiration: { type: "EX", value: SESSION_MAX_AGE } }));
  await dropSession(previous);
  return seal({ sid: id });
}

/** The cookie value after a token refresh: store mode rewrites the record only while it still exists, so a sign-out
 * in another tab stays a sign-out, and keeps the cookie; cookie mode seals the new session. Null when the record is
 * gone or the store does not answer. */
export async function renewSessionCookie(cookie: string | undefined, s: Session): Promise<string | null> {
  const c = store();
  if (!c) return seal(s);
  const id = idOf(cookie);
  if (!id || !cookie) return null;
  try {
    const sc = await timed(c);
    const rec = unseal<Stored>((await timed(sc.getEx(keyOf(id), { type: "EX", value: SESSION_MAX_AGE }))) ?? undefined);
    if (!rec) return null;
    const ok = await timed(sc.set(keyOf(id), seal({ session: s, startedAt: rec.startedAt } satisfies Stored), { expiration: { type: "EX", value: SESSION_MAX_AGE }, condition: "XX" }));
    return ok === null ? null : cookie;
  } catch (e) {
    console.warn(`session store: ${(e as Error).message}`);
    return null;
  }
}

/** Deletes the stored session a cookie names (sign-out, a failed refresh, a new sign-in); cookie mode has nothing to
 * delete. Best effort: when the store does not answer, the record ends with its idle lifetime. */
export async function dropSession(cookie: string | undefined): Promise<void> {
  const c = store();
  const id = c ? idOf(cookie) : null;
  if (!c || !id) return;
  try {
    await timed((await timed(c)).del(keyOf(id)));
  } catch (e) {
    console.warn(`session store: ${(e as Error).message}`);
  }
}

/** The signed-in session (route handlers only: refreshing rewrites or clears the cookie). */
export async function readSession(): Promise<Session | null> {
  const jar = await cookies();
  const cookie = jar.get(SESSION_COOKIE)?.value;
  const s = await loadSession(cookie);
  if (!s) return null;
  if (!needsRefresh(s)) return s;
  const next = await refresh(s);
  const renewed = next ? await renewSessionCookie(cookie, next) : null;
  if (!next || !renewed) {
    await dropSession(cookie);
    jar.set(SESSION_COOKIE, "", clearedCookieOptions());
    return null;
  }
  jar.set(SESSION_COOKIE, renewed, cookieOptions());
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
