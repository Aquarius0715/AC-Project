// Setup project of each app: one UI sign-in, whose browser state (the app's session cookie) the app's specs reuse.
import { test, expect } from "../fixtures/test";
import { signIn } from "../fixtures/auth";

test("sign in through the identity provider", async ({ page, app }) => {
  await signIn(page, app);
  await expect(page.locator("aside")).toContainText(app.user === "hq-operator" ? "HQ Admin" : "AC Project");
  await page.context().storageState({ path: app.state });
});
