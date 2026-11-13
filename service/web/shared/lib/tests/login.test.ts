import { describe, expect, it } from "vitest";
import { demoHref, loginError, safeReturnTo } from "@ac/web/lib/login";

describe("Sign-in page (FR-X01, SCR-X-login, IR251)", () => {
  it("keeps returnTo only inside the role's area", () => {
    expect([safeReturnTo("client", "/customer/units/u1"), safeReturnTo("client", "/customer"), safeReturnTo("client", "/admin"), safeReturnTo("client", "/customerx"), safeReturnTo("client", undefined), safeReturnTo("admin", "//evil.example/admin")])
      .toStrictEqual(["/customer/units/u1", "/customer", "/customer", "/customer", "/customer", "/admin"]);
  });

  it("opens the role home in the browser demo, which has no identity service", () => {
    expect([demoHref("contractor"), demoHref("admin")]).toStrictEqual(["/partner", "/admin"]);
  });

  it("says why a sign-in came back", () => {
    expect([loginError(undefined), loginError("role"), loginError("expired"), loginError("other")]).toStrictEqual([null,
      "This account belongs to another AC Project service — use that service's own sign-in.", "Your session ended — sign in again.", "The sign-in did not complete — try again."]);
  });
});
