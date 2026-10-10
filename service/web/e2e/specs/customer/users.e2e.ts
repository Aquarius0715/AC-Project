// Customer users (FR-C19, DD-C19, IR268): the owner's own row shows when they last signed in — the setup project
// signed in through the identity provider, and the Core API records the token's sign-in time — and the invite dialog
// refuses a bad address and an existing user, then closes without inviting anyone.
import { test, expect } from "../../fixtures/test";

test("the owner's own row shows the last sign-in", async ({ page }) => {
  await page.goto("/customer/users");
  const me = page.locator("main tr", { hasText: "(you)" });
  await expect(me).toBeVisible();
  await expect(me.locator("td").nth(3)).toHaveText(/^\d{1,2} \w+ \d{4}, \d{1,2}:\d{2} (am|pm) MYT$/); // IR44, not "—"
});

test("the invite dialog refuses a bad or existing address and invites nobody when cancelled", async ({ page }) => {
  await page.goto("/customer/users");
  const me = page.locator("main tr", { hasText: "(you)" });
  const email = ((await me.locator("td").first().innerText()).match(/\S+@\S+/) ?? [""])[0];
  expect(email).not.toBe("");
  await page.getByRole("button", { name: "+ Invite member" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Email").fill("not-an-address");
  await dialog.getByRole("button", { name: "Send invite" }).click();
  await expect(dialog).toContainText("A valid email address (up to 254 characters)");
  await dialog.getByLabel("Email").fill(email.toUpperCase()); // the check ignores case
  await expect(dialog).toContainText("Already a user of this customer");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.locator("main tr", { hasText: "Invite pending" })).toHaveCount(0);
});
