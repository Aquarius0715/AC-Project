// BFF session (backend architecture §5): the OIDC tokens and the selected tenant / membership live in an httpOnly,
// HMAC-signed cookie that only the server reads. Browsers never see the access token.
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

export const SESSION_COOKIE = "ac_session";
export const OIDC_COOKIE = "ac_oidc";

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
  const [body, mac] = token.split(".");
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

export async function readSession(): Promise<Session | null> {
  const s = verify<Session>((await cookies()).get(SESSION_COOKIE)?.value);
  if (!s || s.expiresAt * 1000 <= Date.now()) return null;
  return s;
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
