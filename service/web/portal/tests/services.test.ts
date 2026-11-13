import { describe, expect, it } from "vitest";
import { serviceGroups } from "@/lib/services";

const ENV = {
  CUSTOMER_WEB_URL: "http://localhost:3200",
  PARTNER_WEB_URL: "http://localhost:3201",
  TECHNICIAN_WEB_URL: "http://localhost:3202",
  ADMIN_WEB_URL: "http://localhost:3203",
  KEYCLOAK_URL: "http://localhost:8281",
  CORE_API_URL: "http://localhost:8080",
};
const hrefs = (env: Record<string, string>, host: string | null = "localhost:3204") => serviceGroups(env, host).flatMap((g) => g.links.map((l) => l.href));

// IR791: the portal lists every service the environment names, and only those
describe("portal services", () => {
  it("lists the four apps' sign-ins, Keycloak, and the API documentation", () => {
    expect(hrefs(ENV)).toStrictEqual([
      "http://localhost:3200/login", "http://localhost:3201/login", "http://localhost:3202/login", "http://localhost:3203/login",
      "http://localhost:8281/realms/ac/account", "http://localhost:8281/admin/master/console/", "/docs", "/swagger.json",
    ]);
  });

  it("leaves out a service whose address is unset, empty or malformed, and the group when nothing is left", () => {
    const groups = serviceGroups({ ...ENV, KEYCLOAK_URL: "", MAILPIT_URL: "not a url", CORE_API_URL: "" }, null);
    expect(groups.map((g) => g.id)).toStrictEqual(["apps"]);
  });

  it("shows the address the request came by when PUBLIC_HOSTS lists it, and the canonical one otherwise", () => {
    const env = { ...ENV, PUBLIC_HOSTS: "localhost,127.0.0.1" };
    expect(hrefs(env, "127.0.0.1:3204").slice(0, 1)).toStrictEqual(["http://127.0.0.1:3200/login"]);
    expect(hrefs(env, "evil.example").slice(0, 1)).toStrictEqual(["http://localhost:3200/login"]);
  });

  it("lists the development tools only when they are published", () => {
    const ids = serviceGroups({ ...ENV, MAILPIT_URL: "http://localhost:8025", JAEGER_URL: "http://localhost:16686" }, null).find((g) => g.id === "tools")?.links.map((l) => l.id);
    expect(ids).toStrictEqual(["mailpit", "jaeger"]);
  });
});
