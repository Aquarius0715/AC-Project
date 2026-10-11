import { NextResponse, type NextRequest } from "next/server";
import { endIdpSession, oidcConfig, SESSION_COOKIE, unseal, type Session } from "@ac/web/lib/session";

// POST /bff/auth/logout: ends the identity provider's session of this sign-in (back-channel, with the session's
// refresh token, IR357), then drops the BFF session. Sign-out always succeeds for the user: when the provider cannot
// be reached, the cookie is still dropped and the provider's session ends with its own lifetime.
export async function POST(req: NextRequest) {
  const session = unseal<Session>(req.cookies.get(SESSION_COOKIE)?.value);
  if (session?.refreshToken) await endIdpSession(session.refreshToken);
  const res = NextResponse.redirect(new URL("/login", oidcConfig().appUrl), 303);
  res.cookies.delete(SESSION_COOKIE);
  return res;
}
