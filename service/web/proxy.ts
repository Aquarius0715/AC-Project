// Proxy (Next.js 16 file convention, formerly middleware): optimistic auth checks, as the Next.js authentication
// guide recommends — it reads the signed session cookie and never calls the Core API; it calls the identity
// provider only to refresh a token that is about to expire. The Core API authorizes every operation (D01); this only keeps signed-out users and the wrong role
// out of the role areas in API mode. The Phase 1A demo (DATA_SOURCE=mock) is unaffected.
import { NextResponse, type NextRequest } from "next/server";
import { cookieOptions, needsRefresh, refresh, roleHome, SESSION_COOKIE, sign, verify, type Session } from "@/lib/session";

const areas = Object.entries(roleHome) as [Session["role"], string][];
const sharedAreas = ["/notifications", "/settings"]; // shared pages that need a session but no particular role

export async function proxy(req: NextRequest) {
  if (process.env.DATA_SOURCE !== "api") return NextResponse.next();
  let session = verify<Session>(req.cookies.get(SESSION_COOKIE)?.value);
  // Server Components cannot write cookies, so an access token about to expire is refreshed here (the one network
  // call proxy makes, only near expiry) and the new cookie is forwarded to the page render and the browser.
  let renewed: string | null | undefined;
  if (session && needsRefresh(session)) {
    const next = await refresh(session);
    renewed = next ? sign(next) : null;
    session = next;
  }
  const pass = () => {
    if (renewed === undefined) return NextResponse.next();
    const headers = new Headers(req.headers);
    req.cookies.set(SESSION_COOKIE, renewed ?? "");
    headers.set("cookie", req.cookies.toString());
    const res = NextResponse.next({ request: { headers } });
    if (renewed) res.cookies.set(SESSION_COOKIE, renewed, cookieOptions());
    else res.cookies.delete(SESSION_COOKIE);
    return res;
  };
  const path = req.nextUrl.pathname;
  const area = areas.find(([, home]) => path === home || path.startsWith(home + "/"));
  const shared = sharedAreas.some((p) => path === p || path.startsWith(p + "/"));
  if (!area && !shared) return pass();
  if (!session) {
    const login = new URL("/login", req.nextUrl);
    login.searchParams.set("returnTo", path + req.nextUrl.search);
    return NextResponse.redirect(login);
  }
  if (area && session.role !== area[0]) return NextResponse.redirect(new URL(roleHome[session.role], req.nextUrl));
  return pass();
}

// Run on pages only: not on the BFF routes, Next.js assets or static files.
export const config = {
  matcher: ["/((?!bff|_next/static|_next/image|favicon.ico|.*\\.(?:png|svg|jpg|ico|css|js)$).*)"],
};
