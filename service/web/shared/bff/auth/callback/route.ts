import { NextResponse, type NextRequest } from "next/server";
import { APP_ROLE, cookieOptions, createSessionCookie, endIdpSession, LOGIN_HINT_COOKIE, loginHintCookieOptions, loginHintFor, OIDC_COOKIE, oidcConfigFor, oidcCookieOptions, roleHome, SESSION_COOKIE, sessionFromTokens, tokenResponse, unsealAs, type Session, type TokenResponse } from "@ac/web/lib/session";
import { z } from "zod";
import { orNull } from "@ac/web/lib/promises";

const refusal = z.object({ messageKey: z.string().optional() });
const pendingSignIn = z.object({ state: z.string(), verifier: z.string(), returnTo: z.string().nullable(), reauth: z.boolean().optional() });

/** True when the Core API refuses this sign-in because HQ signed the user out everywhere after it (IR384): the identity
provider's single sign-on answered with the old sign-in. Any other answer, or none, lets the sign-in go on. */
async function revokedSignIn(s: Session): Promise<boolean> {
  try {
    const r = await fetch(`${process.env.CORE_API_URL ?? "http://localhost:8080"}/v1/session`, {
      cache: "no-store", signal: AbortSignal.timeout(3000),
      headers: { Authorization: `Bearer ${s.accessToken}`, "X-Tenant-Id": s.tenantId, "X-Membership-Id": s.membershipId },
    });
    if (r.status !== 401) return false;
    const body = refusal.safeParse(await orNull(r.json()));
    return body.success && body.data.messageKey === "errors.session_revoked";
  } catch {
    return false;
  }
}

/** The token response for the code, or null when the identity provider refused it, answered something that is not a
token response (a proxy's error page) or gave no answer within 10 s: the sign-in page's token error, not the error page
(IR719). */
async function exchange(config: ReturnType<typeof oidcConfigFor>, verifier: string, code: string): Promise<TokenResponse | null> {
  try {
    const token = await fetch(`${config.internalIssuer}/protocol/openid-connect/token`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "authorization_code",
        code_verifier: verifier,
        redirect_uri: `${config.appUrl}/bff/auth/callback`,
        client_id: config.clientId,
        client_secret: config.clientSecret,
        code,
      }),
      signal: AbortSignal.timeout(10_000),
    });
    if (!token.ok) return null;
    const tokens = tokenResponse.safeParse(await token.json());
    return tokens.success ? tokens.data : null;
  } catch {
    return null;
  }
}

/** Starts the sign-in again with the identity provider's password form (prompt=login), back to `returnTo`. */
function askPassword(appUrl: string, returnTo: string | null): NextResponse {
  const again = new URL("/bff/auth/login", appUrl);
  again.searchParams.set("reauth", "1");
  if (returnTo) again.searchParams.set("returnTo", returnTo);
  const res = NextResponse.redirect(again);
  res.cookies.set(OIDC_COOKIE, "", oidcCookieOptions(0));
  return res;
}

// GET /bff/auth/callback: exchanges the code, reads tenant / membership / role claims and starts the session: a new
// stored record and its sealed ID in the cookie (IR382), or the sealed session itself without the store.
export async function GET(req: NextRequest) {
  const config = oidcConfigFor(req.headers.get("host")); // the address the sign-in started on (IR651)
  const pending = unsealAs(req.cookies.get(OIDC_COOKIE)?.value, pendingSignIn);
  const code = req.nextUrl.searchParams.get("code");
  if (!pending || !code || req.nextUrl.searchParams.get("state") !== pending.state) {
    return NextResponse.redirect(new URL("/login?error=state", config.appUrl));
  }
  const tokens = await exchange(config, pending.verifier, code);
  if (!tokens) return NextResponse.redirect(new URL("/login?error=token", config.appUrl));
  const session = sessionFromTokens(tokens);
  if (!session) return NextResponse.redirect(new URL("/login?error=membership", config.appUrl));
  // another role's account: the four apps share one single sign-on session, so it is the account last signed in to
  // another app (IR876). That session is ended (as the provider's own "change account" does), so the password form
  // asks for an e-mail address again, filled in with this app's last account; another role's account typed there is
  // the sign-in page's role error.
  if (session.role !== APP_ROLE) {
    if (pending.reauth) return NextResponse.redirect(new URL("/login?error=role", config.appUrl));
    if (session.refreshToken) await endIdpSession(session.refreshToken); // not ended: the form keeps that account, with its "change account"
    return askPassword(config.appUrl, pending.returnTo);
  }
  // signed out everywhere after this sign-in (IR384): ask for the password once (prompt=login), then give up (IR385)
  if (await revokedSignIn(session)) {
    return pending.reauth ? NextResponse.redirect(new URL("/login?error=revoked", config.appUrl)) : askPassword(config.appUrl, pending.returnTo);
  }
  let cookie: string;
  try {
    cookie = await createSessionCookie(session, req.cookies.get(SESSION_COOKIE)?.value); // a new ID: no session fixation
  } catch {
    return NextResponse.redirect(new URL("/login?error=session", config.appUrl)); // the store did not answer
  }
  const res = NextResponse.redirect(new URL(pending.returnTo ?? roleHome[session.role], config.appUrl));
  res.cookies.set(SESSION_COOKIE, cookie, cookieOptions());
  res.cookies.set(OIDC_COOKIE, "", oidcCookieOptions(0));
  const hint = loginHintFor(session.accessToken); // this app's account, for the next password form (IR876)
  if (hint !== null) res.cookies.set(LOGIN_HINT_COOKIE, hint, loginHintCookieOptions());
  return res;
}
