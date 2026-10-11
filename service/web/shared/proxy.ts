// Shared proxy logic (each app's proxy.ts calls authProxy with its role): optimistic auth checks, as the Next.js authentication
// guide recommends — it reads the signed session cookie and never calls the Core API; it calls the identity
// provider only to refresh a token that is about to expire. The Core API authorizes every operation (D01); this only keeps signed-out users and the wrong role
// out of the role areas in API mode. The Phase 1A demo (DATA_SOURCE=mock) is unaffected.
import { NextResponse, type NextRequest } from "next/server";
import { cookieOptions, needsRefresh, oidcConfig, refresh, roleHome, SESSION_COOKIE, seal, unseal, type Session } from "@ac/web/lib/session";
import { contentSecurityPolicy, HSTS, newNonce, originOf } from "@ac/web/lib/securityHeaders";

const sharedAreas = ["/notifications", "/settings"]; // shared pages that need a session but no particular role

/** The optimistic check of one role's app: signed-out users go to sign-in, sessions of another role are ignored. Every
 * page response also carries the Content-Security-Policy with this request's nonce, which Next.js reads from the request
 * headers to put on its scripts and styles (Next.js CSP guide, network architecture §6, IR354). */
export async function authProxy(req: NextRequest, appRole: Session["role"]) {
  const https = req.nextUrl.protocol === "https:" || req.headers.get("x-forwarded-proto") === "https";
  const nonce = newNonce();
  const idp = originOf(oidcConfig().issuer);
  const csp = contentSecurityPolicy(nonce, { dev: process.env.NODE_ENV === "development", https, formActions: idp ? [idp] : [] });
  const headers = new Headers(req.headers);
  headers.set("x-nonce", nonce);
  headers.set("Content-Security-Policy", csp);
  const secure = (res: NextResponse) => {
    res.headers.set("Content-Security-Policy", csp);
    if (https) res.headers.set("Strict-Transport-Security", HSTS);
    return res;
  };
  if (process.env.DATA_SOURCE !== "api") return secure(NextResponse.next({ request: { headers } }));
  let session = unseal<Session>(req.cookies.get(SESSION_COOKIE)?.value);
  if (session && session.role !== appRole) session = null;
  // Server Components cannot write cookies, so an access token about to expire is refreshed here (the one network
  // call proxy makes, only near expiry) and the new cookie is forwarded to the page render and the browser.
  let renewed: string | null | undefined;
  if (session && needsRefresh(session)) {
    const next = await refresh(session);
    renewed = next ? seal(next) : null;
    session = next;
  }
  const pass = () => {
    if (renewed !== undefined) {
      req.cookies.set(SESSION_COOKIE, renewed ?? "");
      headers.set("cookie", req.cookies.toString());
    }
    const res = NextResponse.next({ request: { headers } });
    if (renewed) res.cookies.set(SESSION_COOKIE, renewed, cookieOptions());
    else if (renewed === null) res.cookies.delete(SESSION_COOKIE);
    return secure(res);
  };
  const path = req.nextUrl.pathname;
  const home = roleHome[appRole];
  const area = path === home || path.startsWith(home + "/");
  const shared = sharedAreas.some((p) => path === p || path.startsWith(p + "/"));
  if (!area && !shared) return pass();
  if (!session) {
    const login = new URL("/login", req.nextUrl);
    login.searchParams.set("returnTo", path + req.nextUrl.search);
    return NextResponse.redirect(login);
  }
  return pass();
}

// Run on pages only: not on the BFF routes, Next.js assets or static files.
export const config = {
  matcher: ["/((?!bff|_next/static|_next/image|favicon.ico|.*\\.(?:png|svg|jpg|ico|css|js)$).*)"],
};
