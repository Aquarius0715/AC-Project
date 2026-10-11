// The shell and the session (FR-X01, IR241, IR250, IR251): the frame names the signed-in organization and user, Sign out
// ends the session — the identity provider's too (IR357) — a protected page asks to sign in again with returnTo, and a
// sign-in that came back says why.
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

// the page's own alerts, not Next.js's route announcer (role alert too)
const notice = (page: Page) => page.locator('[role="alert"]:not(#__next-route-announcer__)');

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

test("a copy of the session cookie stops working at sign-out", async ({ browser, app }) => {
  // IR382: the session lives in the store and the cookie only names it, so sign-out revokes every copy at once (with
  // the session sealed into the cookie, a copy kept working until its access token expired)
  const fresh = { baseURL: app.url, storageState: { cookies: [], origins: [] } };
  const context = await browser.newContext(fresh);
  const other = await browser.newContext(fresh);
  try {
    const page = await context.newPage();
    await signIn(page, app);
    const copy = (await context.cookies()).filter((c) => c.name.includes("ac_session_"));
    expect(copy).toHaveLength(1);
    await other.addCookies(copy);
    const elsewhere = await other.newPage();
    await elsewhere.goto(app.home);
    await expect(elsewhere.getByRole("banner")).toContainText(app.user); // the copy works while the session lasts
    await page.getByRole("button", { name: "Sign out" }).click();
    await page.waitForURL(/\/login$/);
    await elsewhere.goto(app.home);
    await expect(elsewhere).toHaveURL(new RegExp(`/login\\?returnTo=${encodeURIComponent(app.home)}`));
  } finally {
    await context.close();
    await other.close();
  }
});

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("a returned sign-in says why", async ({ page }) => {
    await page.goto("/login?error=role");
    await expect(notice(page)).toContainText("belongs to another AC Project service"); // an error banner is an alert (IR362)
    await page.goto("/login?error=expired");
    await expect(notice(page)).toContainText("Your session ended");
    await page.goto("/login");
    await expect(notice(page)).toHaveCount(0);
  });

  test("a protected page keeps where it was going", async ({ page, app }) => {
    await page.goto(`${app.home}/x-not-a-page`);
    await expect(page).toHaveURL(new RegExp(`/login\\?returnTo=${encodeURIComponent(`${app.home}/x-not-a-page`)}`));
    const href = await page.getByRole("link", { name: /^Continue as / }).getAttribute("href");
    expect(decodeURIComponent(href ?? "")).toContain(`returnTo=${app.home}/x-not-a-page`);
  });
});
