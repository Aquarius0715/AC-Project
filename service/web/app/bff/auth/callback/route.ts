import { NextResponse, type NextRequest } from "next/server";
import { jwtClaims, OIDC_COOKIE, oidcConfig, roleHome, SESSION_COOKIE, sign, verify, type Session } from "@/lib/session";

type Pending = { state: string; verifier: string; returnTo: string | null };

// GET /bff/auth/callback: exchanges the code, reads tenant / membership / role claims and stores the session cookie.
export async function GET(req: NextRequest) {
  const cfg = oidcConfig();
  const pending = verify<Pending>(req.cookies.get(OIDC_COOKIE)?.value);
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
  const claims = jwtClaims(t.access_token);
  const role = claims.role as Session["role"] | undefined;
  if (!claims.tenant_id || !claims.membership_id || !role || !(role in roleHome)) {
    return NextResponse.redirect(new URL("/login?error=membership", cfg.appUrl));
  }
  const session: Session = {
    accessToken: t.access_token, refreshToken: t.refresh_token ?? null, tenantId: String(claims.tenant_id), membershipId: String(claims.membership_id),
    role, expiresAt: Math.floor(Date.now() / 1000) + t.expires_in,
  };
  const res = NextResponse.redirect(new URL(pending.returnTo ?? roleHome[role], cfg.appUrl));
  res.cookies.set(SESSION_COOKIE, sign(session), { httpOnly: true, sameSite: "lax", secure: cfg.appUrl.startsWith("https"), path: "/", maxAge: 30 * 60 });
  res.cookies.delete({ name: OIDC_COOKIE, path: "/bff/auth" });
  return res;
}
