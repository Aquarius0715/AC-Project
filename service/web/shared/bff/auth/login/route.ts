import { createHash, randomBytes } from "node:crypto";
import { NextResponse, type NextRequest } from "next/server";
import { loginHint, uiLocale } from "@ac/web/lib/login";
import { OIDC_COOKIE, oidcConfig, oidcCookieOptions, safeReturnTo, seal } from "@ac/web/lib/session";

// GET /bff/auth/login?login_hint=<e-mail>&returnTo=<path>[&ui_locales=en|ms|ja][&reauth=1]: starts the OIDC authorization-code flow with
// PKCE. reauth=1 asks the identity provider for the password again (prompt=login): after HQ signed the user out
// everywhere, its single sign-on would answer with the old sign-in, which the Core API refuses (IR385).
export function GET(req: NextRequest) {
  const config = oidcConfig();
  const state = randomBytes(16).toString("base64url");
  const verifier = randomBytes(32).toString("base64url");
  const challenge = createHash("sha256").update(verifier).digest("base64url");
  const returnTo = req.nextUrl.searchParams.get("returnTo");
  const isReauth = req.nextUrl.searchParams.get("reauth") === "1";
  const hint = loginHint(req.nextUrl.searchParams.get("login_hint")); // an address the person typed, or nothing (IR646)
  const locale = uiLocale(req.nextUrl.searchParams.get("ui_locales")); // the identity provider's pages speak the language chosen on sign-in
  const url = new URL(`${config.issuer}/protocol/openid-connect/auth`);
  url.search = new URLSearchParams({
    client_id: config.clientId,
    response_type: "code",
    scope: "openid",
    redirect_uri: `${config.appUrl}/bff/auth/callback`,
    state,
    code_challenge: challenge,
    code_challenge_method: "S256",
    ...(hint && { login_hint: hint }),
    ...(locale && { ui_locales: locale }),
    ...(isReauth && { prompt: "login" }),
  }).toString();
  const res = NextResponse.redirect(url);
  res.cookies.set(OIDC_COOKIE, seal({ state, verifier, returnTo: safeReturnTo(returnTo), reauth: isReauth }), oidcCookieOptions());
  return Promise.resolve(res);
}
