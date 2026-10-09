import { describe, expect, it } from "vitest";
import { continueHref, loginError, otherServices, safeReturnTo } from "@ac/web/lib/login";
import { ROLES } from "@ac/web/lib/nav";

describe("Sign-in page (FR-X01, SCR-X-login, IR251)", () => {
  it("keeps returnTo only inside the role's area", () => {
    expect([safeReturnTo("client", "/customer/units/u1"), safeReturnTo("client", "/customer"), safeReturnTo("client", "/admin"), safeReturnTo("client", "/customerx"), safeReturnTo("client", undefined), safeReturnTo("admin", "//evil.example/admin")])
      .toEqual(["/customer/units/u1", "/customer", "/customer", "/customer", "/customer", "/admin"]);
  });

  it("continues through the BFF sign-in in API mode and to the role home in the browser demo", () => {
    expect(continueHref(true, "technician", "/technician?tab=all")).toBe("/bff/auth/login?login_hint=tech-internal-a&returnTo=%2Ftechnician%3Ftab%3Dall");
    expect(continueHref(false, "contractor", "/partner/jobs")).toBe("/partner");
  });

  it("names the other services in Figma's order and says why a sign-in came back", () => {
    expect((["client", "admin", "contractor", "technician"] as const).map((r) => otherServices(r).map((o) => ROLES[o].loginTitle).join(", ")))
      .toEqual(["Admin, Partner, Technician", "Client, Partner, Technician", "Client, Admin, Technician", "Client, Admin, Partner"]);
    expect([loginError(undefined), loginError("role"), loginError("expired"), loginError("other")]).toEqual([null,
      "This account belongs to another AC Project service — use that service's own sign-in.", "Your session ended — sign in again.", "The sign-in did not complete — try again."]);
  });
});
