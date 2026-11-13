// The services the portal lists (IR791): each address comes from the environment that already names it for the apps, and
// is shown on the address this request came by when PUBLIC_HOSTS lists it (IR651). A service whose address is not set is
// left out, so the page never links to something that is not there.
import { z } from "zod";
import { addressFor } from "@ac/web/lib/publicOrigin";

const address = z.url({ protocol: /^https?$/ });
const optional = address.optional().catch(undefined); // an unset or malformed address leaves the service out
const settings = z.object({
  CUSTOMER_WEB_URL: optional,
  PARTNER_WEB_URL: optional,
  TECHNICIAN_WEB_URL: optional,
  ADMIN_WEB_URL: optional,
  KEYCLOAK_URL: optional,
  MAILPIT_URL: optional,
  JAEGER_URL: optional,
  CORE_API_URL: optional,
});

interface ServiceLink { id: string; name: string; note: string; href: string }
export interface ServiceGroup { id: string; title: string; links: ServiceLink[] }
type Env = Readonly<Record<string, string | undefined>>;

/** One link, or none when its address is not known. */
const link = (id: string, name: string, note: string, href: string | undefined): ServiceLink[] => (href ? [{ id, name, note, href }] : []);

/** The groups of links for a request that came by `host`; empty groups are dropped. */
export function serviceGroups(env: Env, host: string | null): ServiceGroup[] {
  const s = settings.parse({
    ...env,
    ...Object.fromEntries(Object.entries(env).filter(([, v]) => v === "").map(([k]) => [k, undefined])), // "" is unset
  });
  const on = (base: string | undefined, path = "") => (base ? `${addressFor(base, host, env)}${path}` : undefined);
  const groups: ServiceGroup[] = [
    { id: "apps", title: "Applications", links: [
      ...link("customer", "Customer", "Units, alerts, maintenance, billing", on(s.CUSTOMER_WEB_URL, "/login")),
      ...link("partner", "Partner", "Contractor jobs, technicians, payouts", on(s.PARTNER_WEB_URL, "/login")),
      ...link("technician", "Technician", "Assigned jobs and work reports", on(s.TECHNICIAN_WEB_URL, "/login")),
      ...link("admin", "HQ Admin", "Operations, billing, restrictions, audit", on(s.ADMIN_WEB_URL, "/login")),
    ] },
    { id: "identity", title: "Sign-in service (Keycloak)", links: [
      ...link("keycloak-account", "Account", "Your sign-in account: password and one-time code device", on(s.KEYCLOAK_URL, "/realms/ac/account")),
      ...link("keycloak-admin", "Administration console", "Realm, clients, users (Keycloak administrator)", on(s.KEYCLOAK_URL, "/admin/master/console/")),
    ] },
    { id: "api", title: "API", links: s.CORE_API_URL ? [
      { id: "openapi", name: "OpenAPI (Swagger UI)", note: "The Core API's operations, readable and callable", href: "/docs" },
      { id: "openapi-json", name: "OpenAPI document", note: "swagger.json", href: "/swagger.json" },
    ] : [] },
    { id: "tools", title: "Development tools", links: [
      ...link("mailpit", "Mailpit", "The e-mails the system sent", on(s.MAILPIT_URL)),
      ...link("jaeger", "Jaeger", "Request traces", on(s.JAEGER_URL)),
    ] },
  ];
  return groups.filter((g) => g.links.length > 0);
}
