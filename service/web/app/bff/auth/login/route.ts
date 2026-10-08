import { createHash, randomBytes } from "node:crypto";
import { NextResponse, type NextRequest } from "next/server";
import { OIDC_COOKIE, oidcConfig, sign } from "@/lib/session";

// GET /bff/auth/login?login_hint=<demo user>&returnTo=<path>: starts the OIDC authorization-code flow with PKCE.
export async function GET(req: NextRequest) {
  const cfg = oidcConfig();
  const state = randomBytes(16).toString("base64url");
  const verifier = randomBytes(32).toString("base64url");
  const challenge = createHash("sha256").update(verifier).digest("base64url");
  const returnTo = req.nextUrl.searchParams.get("returnTo");
  const url = new URL(`${cfg.issuer}/protocol/openid-connect/auth`);
  url.search = new URLSearchParams({
    client_id: cfg.clientId,
    response_type: "code",
    scope: "openid",
    redirect_uri: `${cfg.appUrl}/bff/auth/callback`,
    state,
    code_challenge: challenge,
    code_challenge_method: "S256",
    ...(req.nextUrl.searchParams.get("login_hint") ? { login_hint: req.nextUrl.searchParams.get("login_hint")! } : {}),
  }).toString();
  const res = NextResponse.redirect(url);
  res.cookies.set(OIDC_COOKIE, sign({ state, verifier, returnTo: returnTo?.startsWith("/") ? returnTo : null }), {
    httpOnly: true, sameSite: "lax", secure: cfg.appUrl.startsWith("https"), path: "/bff/auth", maxAge: 600,
  });
  return res;
}
