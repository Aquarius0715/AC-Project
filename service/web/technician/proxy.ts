// Proxy (Next.js 16 file convention): the shared optimistic auth check for the Technician app (IR175/IR176).
import type { NextRequest } from "next/server";
import { authProxy } from "@ac/web/proxy";

export function proxy(req: NextRequest) {
  return authProxy(req, "technician");
}

// Run on pages only: not on the BFF routes, Next.js assets or static files.
export const config = {
  matcher: ["/((?!bff|_next/static|_next/image|favicon.ico|.*\\.(?:png|svg|jpg|ico|css|js)$).*)"],
};
