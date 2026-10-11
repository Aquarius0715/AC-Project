import { NextResponse, type NextRequest } from "next/server";
import { APP_ROLE, cookieOptions, createSessionCookie, OIDC_COOKIE, oidcConfig, oidcCookieOptions, roleHome, SESSION_COOKIE, sessionFromTokens, unseal } from "@ac/web/lib/session";

type Pending = { state: string; verifier: string; returnTo: string | null };

// GET /bff/auth/callback: exchanges the code, reads tenant / membership / role claims and starts the session: a new
// stored record and its sealed ID in the cookie (IR382), or the sealed session itself without the store.
export async function GET(req: NextRequest) {
  const cfg = oidcConfig();
  const pending = unseal<Pending>(req.cookies.get(OIDC_COOKIE)?.value);
  const code = req.nextUrl.searchParams.get("code");
  if (!pending || !code || req.nextUrl.searchParams.get("state") !== pending.state) {
    return NextResponse.redirect(new URL("/login?error=state", cfg.appUrl));
  }
  const token = await fetch(`${cfg.internalIssuer}/protocol/openid-connect/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "authorization_code", code, code_verifier: pending.verifier, redirect_uri: `${cfg.appUrl}/bff/auth/callback`,
      client_id: cfg.clientId, client_secret: cfg.clientSecret,
    }),
  });
  if (!token.ok) return NextResponse.redirect(new URL("/login?error=token", cfg.appUrl));
  const t = (await token.json()) as { access_token: string; refresh_token?: string; expires_in: number };
  const session = sessionFromTokens(t);
  if (!session) return NextResponse.redirect(new URL("/login?error=membership", cfg.appUrl));
  if (session.role !== APP_ROLE) return NextResponse.redirect(new URL("/login?error=role", cfg.appUrl)); // another role's app
  let cookie: string;
  try {
    cookie = await createSessionCookie(session, req.cookies.get(SESSION_COOKIE)?.value); // a new ID: no session fixation
  } catch {
    return NextResponse.redirect(new URL("/login?error=session", cfg.appUrl)); // the store did not answer
  }
  const res = NextResponse.redirect(new URL(pending.returnTo ?? roleHome[session.role], cfg.appUrl));
  res.cookies.set(SESSION_COOKIE, cookie, cookieOptions());
  res.cookies.set(OIDC_COOKIE, "", oidcCookieOptions(0));
  return res;
}
