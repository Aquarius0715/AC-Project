// Proxy (Next.js 16 file convention, formerly middleware): optimistic auth checks only, as the Next.js
// authentication guide recommends — it reads the signed session cookie and never calls the identity provider or
// the Core API. The Core API authorizes every operation (D01); this only keeps signed-out users and the wrong role
// out of the role areas in API mode. The Phase 1A demo (DATA_SOURCE=mock) is unaffected.
import { NextResponse, type NextRequest } from "next/server";
import { roleHome, SESSION_COOKIE, verify, type Session } from "@/lib/session";

const areas = Object.entries(roleHome) as [Session["role"], string][];

export function proxy(req: NextRequest) {
  if (process.env.DATA_SOURCE !== "api") return NextResponse.next();
  const path = req.nextUrl.pathname;
  const area = areas.find(([, home]) => path === home || path.startsWith(home + "/"));
  if (!area) return NextResponse.next();
  const session = verify<Session>(req.cookies.get(SESSION_COOKIE)?.value);
  if (!session) {
    const login = new URL("/login", req.nextUrl);
    login.searchParams.set("returnTo", path + req.nextUrl.search);
    return NextResponse.redirect(login);
  }
  if (session.role !== area[0]) return NextResponse.redirect(new URL(roleHome[session.role], req.nextUrl));
  return NextResponse.next();
}

// Run on pages only: not on the BFF routes, Next.js assets or static files.
export const config = {
  matcher: ["/((?!bff|_next/static|_next/image|favicon.ico|.*\\.(?:png|svg|jpg|ico|css|js)$).*)"],
};
