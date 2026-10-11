import { NextResponse, type NextRequest } from "next/server";
import { clearedCookieOptions, endIdpSession, oidcConfig, SESSION_COOKIE, unseal, type Session } from "@ac/web/lib/session";
import { sameOriginRequest } from "@ac/web/lib/securityHeaders";

// POST /bff/auth/logout: ends the identity provider's session of this sign-in (back-channel, with the session's
// refresh token, IR357), then drops the BFF session. Sign-out always succeeds for the user: when the provider cannot
// be reached, the cookie is still dropped and the provider's session ends with its own lifetime. A sign-out posted by
// another site is refused (CSRF, IR358).
export async function POST(req: NextRequest) {
  if (!sameOriginRequest(req.headers)) return new NextResponse(null, { status: 403 });
  const session = unseal<Session>(req.cookies.get(SESSION_COOKIE)?.value);
  if (session?.refreshToken) await endIdpSession(session.refreshToken);
  const res = NextResponse.redirect(new URL("/login", oidcConfig().appUrl), 303);
  res.cookies.set(SESSION_COOKIE, "", clearedCookieOptions());
  return res;
}
