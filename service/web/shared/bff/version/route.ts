import { NextResponse } from "next/server";
import { APP_VERSION } from "@ac/web/lib/appVersion";

// GET /bff/version: the build this server runs (IR856). A page compares it with its own build to see that the app was
// deployed since it loaded; the deployment's health check reads it too (IR855). No session and no data.
export function GET() {
  return NextResponse.json({ version: APP_VERSION }, { headers: { "Cache-Control": "no-store" } });
}
