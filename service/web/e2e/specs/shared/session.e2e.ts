// The shell and the session (FR-X01, IR241, IR250, IR251): the frame names the signed-in organization and user, Sign out
// ends the session — the identity provider's too (IR357) — a protected page asks to sign in again with returnTo, and a
// sign-in that came back says why.
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

test("the shell names the signed-in user, not the demo persona", async ({ page, app }) => {
  await page.goto(app.home);
  await expect(page.getByRole("banner")).toContainText(app.user);
  await expect(page.locator("aside")).not.toContainText(/CUSTOMER-A|HQ TENANT/); // the Phase 1A scope labels
});

test("Sign out ends the session, at the identity provider too", async ({ browser, app }) => {
  // a sign-in of its own: ending the provider's session must not end the one the project's other tests share
  const context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } });
  try {
    const page = await context.newPage();
    await signIn(page, app);
    await page.getByRole("button", { name: "Sign out" }).click();
    await page.waitForURL(/\/login$/);
    await page.goto(app.home);
    await expect(page).toHaveURL(new RegExp(`/login\\?returnTo=${encodeURIComponent(app.home)}`));
    const again = page.getByRole("link", { name: `Continue as ${app.user} →` });
    await expect(again).toHaveAttribute("href", new RegExp(`^/bff/auth/login\\?login_hint=${app.user}&returnTo=`));
    // the provider's session ended with the sign-out: signing in again asks for the password instead of coming back
    await again.click();
    await expect(page.locator("#password")).toBeVisible();
  } finally {
    await context.close();
  }
});

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("a returned sign-in says why", async ({ page }) => {
    await page.goto("/login?error=role");
    await expect(page.getByRole("status")).toContainText("belongs to another AC Project service");
    await page.goto("/login?error=expired");
    await expect(page.getByRole("status")).toContainText("Your session ended");
    await page.goto("/login");
    await expect(page.getByRole("status")).toHaveCount(0);
  });

  test("a protected page keeps where it was going", async ({ page, app }) => {
    await page.goto(`${app.home}/x-not-a-page`);
    await expect(page).toHaveURL(new RegExp(`/login\\?returnTo=${encodeURIComponent(`${app.home}/x-not-a-page`)}`));
    const href = await page.getByRole("link", { name: /^Continue as / }).getAttribute("href");
    expect(decodeURIComponent(href ?? "")).toContain(`returnTo=${app.home}/x-not-a-page`);
  });
});
