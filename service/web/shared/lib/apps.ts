// Base URLs of the four web apps (one per entry point, IR178), for links between apps (hard navigations).
// Server only: read from the runtime environment of each image.
import "server-only";
import type { Role } from "@ac/web/lib/nav";

export function appLinks(): Partial<Record<Role, string>> {
  const e = process.env;
  return { client: e.CUSTOMER_WEB_URL, contractor: e.PARTNER_WEB_URL, technician: e.TECHNICIAN_WEB_URL, admin: e.ADMIN_WEB_URL };
}
