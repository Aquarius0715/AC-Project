import { describe, expect, it } from "vitest";
import { demoHref, loginError, needsOtherAccount, safeReturnTo, signInStart } from "@ac/web/lib/login";

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

  // UI/UX loop R3: an account of another service came back with error=role; the plain start would let single sign-on
  // answer with that account again, a dead end. The start then asks for the sign-in form (prompt=login, reauth=1).
  it("lets another account sign in after the account itself was refused", () => {
    expect(["role", "membership", "expired", "token", undefined].map((c) => needsOtherAccount(c))).toStrictEqual([true, true, false, false, false]);
    expect([signInStart("/partner", true), signInStart("/partner/jobs", false)]).toStrictEqual(["/bff/auth/login?returnTo=%2Fpartner&reauth=1", "/bff/auth/login?returnTo=%2Fpartner%2Fjobs"]);
  });
});
