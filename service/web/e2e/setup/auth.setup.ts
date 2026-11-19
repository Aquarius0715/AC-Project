// Setup project of each app: one UI sign-in, whose browser state (the app's session cookie) the app's specs reuse.
import { test, expect } from "../fixtures/test";
import { signIn } from "../fixtures/auth";

test("sign in through the identity provider", async ({ page, app }) => {
  await signIn(page, app);
  await expect(page.locator("aside")).toContainText(app.user === "hq-operator" ? "HQ Admin" : "AC Project");
  // The session cookie expires 30 minutes after its last renewal, and only the context that refreshes the token gets
  // the renewed one. Every test's context starts from this file, so a test started near 30 minutes after this sign-in
  // lost its session halfway (the admin crawl, the last project, on 2026-11-19). The browser keeps the cookie for the
  // whole run instead; the server's idle lifetime of the stored session still decides.
  const context = page.context();
  const cookies = await context.cookies();
  await context.addCookies(cookies.map((c) => ({ ...c, expires: -1 }))); // -1: a session cookie
  await context.storageState({ path: app.state });
});
