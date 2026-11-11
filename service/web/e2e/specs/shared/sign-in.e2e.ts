// The sign-in page of each app (FR-X01, IR646): the language is chosen before signing in and the identity provider's own
// pages speak it, with the address already filled in; a forgotten password is reset there, and its e-mail reaches the inbox.
import { test, expect } from "../../fixtures/test";
import { emailOf, startSignIn } from "../../fixtures/auth";
import { inboxAvailable, mailsTo } from "../../fixtures/mailpit";

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("the language chosen on the sign-in page is the identity provider's language, with the address filled in", async ({ page, app }) => {
    await page.goto("/login");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await page.getByRole("button", { name: "日本語" }).click();
    await expect(page.getByRole("heading", { name: "サインイン" })).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", "ja");
    await startSignIn(page, emailOf(app.user));
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.locator("#username")).toHaveValue(emailOf(app.user));
    await expect(page.locator("html")).toHaveAttribute("lang", "ja");
    await expect(page.getByRole("button", { name: "サインイン" })).toBeVisible();
  });

  test("a forgotten password is reset on the identity provider's pages: the e-mail reaches the inbox", async ({ page, app }) => {
    test.skip(app.id !== "customer", "one app is enough: every app leads to the same identity provider");
    test.skip(!(await inboxAvailable()), "no inbox on this stack");
    const email = emailOf("customer-b");
    const before = await mailsTo(email);
    await page.goto("/login");
    await expect(page.getByRole("link", { name: "Forgot password?" })).toHaveCount(0); // in API mode the page leads to the provider's own reset
    await startSignIn(page, email);
    await page.getByRole("link", { name: "Forgot password?" }).click();
    await page.locator("#username").fill(email);
    await page.getByRole("button", { name: "Submit" }).click();
    await expect(page.getByText(/receive an email|instructions/i).first()).toBeVisible();
    await expect.poll(() => mailsTo(email), { timeout: 30_000 }).toBeGreaterThan(before);
  });
});
