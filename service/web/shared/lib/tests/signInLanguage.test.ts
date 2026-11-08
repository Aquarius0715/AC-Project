// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { chooseSignInLanguage } from "@ac/web/screens/auth/login/actions";
import { SIGN_IN_LOCALE_COOKIE } from "@ac/web/lib/login";

const jar = vi.hoisted(() => ({ set: vi.fn() }));
vi.mock("next/headers", () => ({ cookies: () => Promise.resolve(jar) }));

const form = (locale: string) => { const f = new FormData(); f.set("locale", locale); return f; };

// IR646: Server Functions are public endpoints, so only a display language is ever kept
describe("choosing the sign-in language", () => {
  it("keeps a display language in the sign-in cookie", async () => {
    await chooseSignInLanguage(form("ja"));
    expect(jar.set).toHaveBeenCalledWith(SIGN_IN_LOCALE_COOKIE, "ja", expect.objectContaining({ path: "/", sameSite: "lax", httpOnly: true }));
  });

  it("keeps nothing for another value or none", async () => {
    jar.set.mockClear();
    await chooseSignInLanguage(form("fr"));
    await chooseSignInLanguage(new FormData());
    expect(jar.set).not.toHaveBeenCalled();
  });
});
