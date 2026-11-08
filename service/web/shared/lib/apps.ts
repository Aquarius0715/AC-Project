// Base URLs of the four web apps (one per entry point, IR178), for links between apps (hard navigations).
// Server only: read from the runtime environment of each image.
import "server-only";
import { headers } from "next/headers";
import type { Role } from "@ac/web/lib/nav";
import { addressFor } from "@ac/web/lib/publicOrigin";

/** The other apps' base URLs, on the address this request came by when PUBLIC_HOSTS lists it (IR651). */
export async function appLinks(): Promise<Partial<Record<Role, string>>> {
  const e = process.env;
  const requestHeaders = await headers();
  const host = requestHeaders.get("host");
  const on = (url: string | undefined) => (url ? addressFor(url, host) : undefined);
  return { client: on(e.CUSTOMER_WEB_URL), contractor: on(e.PARTNER_WEB_URL), technician: on(e.TECHNICIAN_WEB_URL), admin: on(e.ADMIN_WEB_URL) };
}
