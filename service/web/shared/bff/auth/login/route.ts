import { createHash, randomBytes } from "node:crypto";
import { NextResponse, type NextRequest } from "next/server";
import { OIDC_COOKIE, oidcConfig, oidcCookieOptions, safeReturnTo, seal } from "@ac/web/lib/session";

// GET /bff/auth/login?login_hint=<demo user>&returnTo=<path>[&reauth=1]: starts the OIDC authorization-code flow with
// PKCE. reauth=1 asks the identity provider for the password again (prompt=login): after HQ signed the user out
// everywhere, its single sign-on would answer with the old sign-in, which the Core API refuses (IR385).
export async function GET(req: NextRequest) {
  const cfg = oidcConfig();
  const state = randomBytes(16).toString("base64url");
  const verifier = randomBytes(32).toString("base64url");
  const challenge = createHash("sha256").update(verifier).digest("base64url");
  const returnTo = req.nextUrl.searchParams.get("returnTo");
  const reauth = req.nextUrl.searchParams.get("reauth") === "1";
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
    ...(reauth ? { prompt: "login" } : {}),
  }).toString();
  const res = NextResponse.redirect(url);
  res.cookies.set(OIDC_COOKIE, seal({ state, verifier, returnTo: safeReturnTo(returnTo), reauth }), oidcCookieOptions());
  return res;
}
