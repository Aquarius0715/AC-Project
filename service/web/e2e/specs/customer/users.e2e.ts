// Customer users (FR-C19, DD-C19, IR268, IR417): the owner's own row shows when they last signed in — the setup project
// signed in through the identity provider, and the Core API records the token's sign-in time — and the invite dialog
// refuses a bad address and an existing user, then closes without inviting anyone.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("the owner's own row shows the last sign-in", async ({ page }) => {
  await page.goto("/customer/users");
  await expect(page.getByRole("heading", { name: /^Users of \S/, level: 1 })).toBeVisible(); // names the customer (Figma 11a, IR315)
  const me = page.locator("main tr", { hasText: "(you)" });
  await expect(me).toBeVisible();
  await expect(me.locator("td").nth(3)).toHaveText(/^\d{1,2} \w+ \d{4}, \d{1,2}:\d{2} (am|pm) MYT$/); // IR44, not "—"
});

test("the invite dialog refuses a bad or existing address and invites nobody when cancelled", async ({ page }) => {
  await page.goto("/customer/users");
  const me = page.locator("main tr", { hasText: "(you)" });
  const email = ((await me.locator("td").first().innerText()).match(/\S+@\S+/) ?? [""])[0];
  expect(email).not.toBe("");
  await page.getByRole("button", { name: "Invite user" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: /^Invite a user to \S/ })).toBeVisible();
  await expectAccessible(page, '[role="dialog"]'); // the open dialog (IR360)
  await dialog.getByLabel("Email").fill("not-an-address");
  await dialog.getByRole("button", { name: "Send invite" }).click();
  await expect(dialog).toContainText("A valid email address (up to 254 characters)");
  await dialog.getByLabel("Email").fill(email.toUpperCase()); // the check ignores case
  await expect(dialog).toContainText("Already a user of this customer");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.locator("main tr", { hasText: "Invite pending" })).toHaveCount(0);
});

// IR417: the owner invites an owner, changes the invite to a member with a reason, and withdraws it; the own row has
// no actions.
test("the owner invites, edits and removes a user", async ({ page }) => {
  await page.goto("/customer/users");
  await expect(page.locator("main tr", { hasText: "(you)" })).toContainText("Your account");
  const email = `e2e-${Date.now()}@example.com`;
  await page.getByRole("button", { name: "Invite user" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("owner");
  await dialog.getByRole("button", { name: "Send invite" }).click();
  const row = page.locator("main tr", { hasText: email });
  await expect(row).toContainText("Invite pending");
  await expect(row).toContainText("Owner");
  await row.getByRole("button", { name: "Edit" }).click();
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("member");
  await dialog.getByRole("button", { name: "Save invite" }).click();
  await expect(dialog).toContainText("A reason of 1–1000 characters");
  await dialog.getByLabel("Reason").fill("Not an owner after all");
  await dialog.getByRole("button", { name: "Save invite" }).click();
  await expect(row).toContainText("Member");
  await row.getByRole("button", { name: "Remove" }).click();
  await dialog.getByLabel("Reason").fill("Sent to the wrong address");
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(row).toHaveCount(0);
});
