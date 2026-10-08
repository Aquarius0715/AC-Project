import { NextResponse } from "next/server";
import { oidcConfig, SESSION_COOKIE } from "@ac/web/lib/session";

// POST /bff/auth/logout: drops the BFF session (the Keycloak SSO session ends with its own lifetime in local runs).
export async function POST() {
  const res = NextResponse.redirect(new URL("/login", oidcConfig().appUrl), 303);
  res.cookies.delete(SESSION_COOKIE);
  return res;
}
