import { describe, expect, it } from "vitest";
import { demoHref, loginError, loginHint, otherServices, safeReturnTo, uiLocale } from "@ac/web/lib/login";
import { ROLES } from "@ac/web/lib/nav";
import { entryOf } from "@ac/web/lib/lookup";

describe("Sign-in page (FR-X01, SCR-X-login, IR251)", () => {
  it("keeps returnTo only inside the role's area", () => {
    expect([safeReturnTo("client", "/customer/units/u1"), safeReturnTo("client", "/customer"), safeReturnTo("client", "/admin"), safeReturnTo("client", "/customerx"), safeReturnTo("client", undefined), safeReturnTo("admin", "//evil.example/admin")])
      .toStrictEqual(["/customer/units/u1", "/customer", "/customer", "/customer", "/customer", "/admin"]);
  });

  it("opens the role home in the browser demo, which has no identity service", () => {
    expect([demoHref("contractor"), demoHref("admin")]).toStrictEqual(["/partner", "/admin"]);
  });

  // IR646: the person types an address, the BFF passes it to the identity provider; anything else is not passed on
  it("takes an e-mail address as the identity provider's hint, and nothing else", () => {
    expect([loginHint("  Mia@Example.com "), loginHint("customer-a"), loginHint("a@b"), loginHint("a b@example.com"), loginHint(""), loginHint(null)])
      .toStrictEqual(["Mia@Example.com", undefined, undefined, undefined, undefined, undefined]);
    expect(loginHint(`${"a".repeat(310)}@example.com`)).toBeUndefined(); // longer than any address (320)
    expect(loginHint(`${"a".repeat(60)}@example.com`)).toBeDefined();
  });

  it("asks the identity provider's pages only in a display language", () => {
    expect([uiLocale("ja"), uiLocale("ms"), uiLocale("en"), uiLocale("fr"), uiLocale(null), uiLocale(undefined)]).toStrictEqual(["ja", "ms", "en", undefined, undefined, undefined]);
  });

  it("names the other services in Figma's order and says why a sign-in came back", () => {
    expect((["client", "admin", "contractor", "technician"] as const).map((r) => otherServices(r).map((o) => entryOf(ROLES, o).loginTitle).join(", ")))
      .toStrictEqual(["Admin, Partner, Technician", "Client, Partner, Technician", "Client, Admin, Technician", "Client, Admin, Partner"]);
    expect([loginError(undefined), loginError("role"), loginError("expired"), loginError("other")]).toStrictEqual([null,
      "This account belongs to another AC Project service — use that service's own sign-in.", "Your session ended — sign in again.", "The sign-in did not complete — try again."]);
  });
});
