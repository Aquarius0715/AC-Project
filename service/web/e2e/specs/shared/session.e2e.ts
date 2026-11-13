// The shell and the session (FR-X01, IR241, IR250, IR251): the frame names the signed-in organization and user, Sign out
// ends the session — the identity provider's too (IR357) — a protected page asks to sign in again with returnTo, and a
// sign-in that came back says why.
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { enterPassword, signIn } from "../../fixtures/auth";

// the page's own alerts, not Next.js's route announcer (role alert too)
const notice = (page: Page) => page.locator('[role="alert"]:not(#__next-route-announcer__)');

test("the shell names the signed-in user, not the demo persona", async ({ page, app }) => {
  await page.goto(app.home);
  await expect(page.getByRole("banner")).toContainText(app.user);
  await expect(page.locator("aside")).not.toContainText(/CUSTOMER-A|HQ TENANT/); // the Phase 1A scope labels
});

test("Page unavailable leads a signed-in user to their Overview and anyone else to sign in", async ({ browser, page, app }) => {
  // one view for 403 and 404 that never moves on by itself (IR57); its one link fits who is looking (IR571)
  for (const path of [`${app.home}/no-such-page`, "/forbidden"]) {
    await page.goto(path);
    await expect(page.getByText("This page isn’t available")).toBeVisible();
    await expect(page.getByRole("link", { name: "Go to Overview" })).toHaveAttribute("href", app.home);
    await expect(page.getByText(/NOT_FOUND|IR57/)).toHaveCount(0); // no design notes on the screen
  }
  await using context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } });
  const out = await context.newPage();
  await out.goto("/no-such-page");
  await expect(out.getByRole("link", { name: "Go to sign in" })).toHaveAttribute("href", "/login");
});

test("Sign out ends the session, at the identity provider too", async ({ browser, app }) => {
  // a sign-in of its own: ending the provider's session must not end the one the project's other tests share
  await using context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } });
  const page = await context.newPage();
  await signIn(page, app);
  await page.getByRole("button", { name: "Sign out" }).click();
  await page.waitForURL(/\/login$/);
  await page.goto(app.home);
  // the provider's session ended with the sign-out: the app's sign-in leads to the provider, which asks for the password
  // instead of letting the person straight back in
  await expect(page.locator("#password")).toBeVisible();
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
    const cookies = await context.cookies();
    const copy = cookies.filter((c) => c.name.includes("ac_session_"));
    expect(copy).toHaveLength(1);
    await other.addCookies(copy);
    const elsewhere = await other.newPage();
    await elsewhere.goto(app.home);
    await expect(elsewhere.getByRole("banner")).toContainText(app.user); // the copy works while the session lasts
    await page.getByRole("button", { name: "Sign out" }).click();
    await page.waitForURL(/\/login$/);
    await elsewhere.goto(app.home);
    await expect(elsewhere).toHaveURL(new RegExp(String.raw`/login\?returnTo=${encodeURIComponent(app.home)}`));
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
    await page.goto("/login"); // a plain visit goes straight on to the identity provider (IR790)
    await expect(page.locator("#password")).toBeVisible();
  });

  test("a protected page keeps where it was going", async ({ page, app }) => {
    await page.goto(`${app.home}/x-not-a-page`);
    await enterPassword(page, app.user); // the identity provider's page; the sign-in returns to where it was going
    await expect(page).toHaveURL(new RegExp(`${app.home}/x-not-a-page$`));
  });
});
