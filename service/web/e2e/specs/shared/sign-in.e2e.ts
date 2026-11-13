// The sign-in of each app (FR-X01, IR790): /login leads straight to the identity provider's one sign-in page; a forgotten
// password is reset there, and its e-mail reaches the inbox.
import { test, expect } from "../../fixtures/test";
import { emailOf, startSignIn } from "../../fixtures/auth";
import { inboxAvailable, mailsTo } from "../../fixtures/mailpit";

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("/login leads straight to the identity provider's one page for the address and the password", async ({ page, app }) => {
    await page.goto("/login");
    await expect(page.locator("#password")).toBeVisible();
    await startSignIn(page, emailOf(app.user));
    await expect(page.locator("#username")).toHaveValue(emailOf(app.user));
    await expect(page.locator('form[action="/bff/auth/login"]')).toHaveCount(0); // no sign-in page of the app in between
  });

  test("a forgotten password is reset on the identity provider's pages: the e-mail reaches the inbox", async ({ page, app }) => {
    test.skip(app.id !== "customer", "one app is enough: every app leads to the same identity provider");
    test.skip(!(await inboxAvailable()), "no inbox on this stack");
    const email = emailOf("customer-b");
    const before = await mailsTo(email);
    await page.goto("/login");
    await page.getByRole("link", { name: "Forgot password?" }).click();
    await page.locator("#username").fill(email);
    await page.getByRole("button", { name: "Submit" }).click();
    await expect(page.getByText(/receive an email|instructions/i).first()).toBeVisible();
    await expect.poll(() => mailsTo(email), { timeout: 30_000 }).toBeGreaterThan(before);
  });
});
