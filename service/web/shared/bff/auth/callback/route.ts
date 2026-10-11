import { NextResponse, type NextRequest } from "next/server";
import { APP_ROLE, cookieOptions, createSessionCookie, OIDC_COOKIE, oidcConfig, oidcCookieOptions, roleHome, SESSION_COOKIE, sessionFromTokens, unseal, type Session } from "@ac/web/lib/session";

type Pending = { state: string; verifier: string; returnTo: string | null; reauth?: boolean };

/** True when the Core API refuses this sign-in because HQ signed the user out everywhere after it (IR384): the identity
 * provider's single sign-on answered with the old sign-in. Any other answer, or none, lets the sign-in go on. */
async function revokedSignIn(s: Session): Promise<boolean> {
  try {
    const r = await fetch(`${process.env.CORE_API_URL ?? "http://localhost:8080"}/v1/session`, {
      method: "GET", cache: "no-store", signal: AbortSignal.timeout(3000),
      headers: { Authorization: `Bearer ${s.accessToken}`, "X-Tenant-Id": s.tenantId, "X-Membership-Id": s.membershipId },
    });
    return r.status === 401 && ((await r.json().catch(() => null)) as { messageKey?: string } | null)?.messageKey === "errors.session_revoked";
  } catch {
    return false;
  }
}

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
  // signed out everywhere after this sign-in (IR384): ask for the password once (prompt=login), then give up (IR385)
  if (await revokedSignIn(session)) {
    if (pending.reauth) return NextResponse.redirect(new URL("/login?error=revoked", cfg.appUrl));
    const again = new URL("/bff/auth/login", cfg.appUrl);
    again.searchParams.set("reauth", "1");
    if (pending.returnTo) again.searchParams.set("returnTo", pending.returnTo);
    const res = NextResponse.redirect(again);
    res.cookies.set(OIDC_COOKIE, "", oidcCookieOptions(0));
    return res;
  }
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
