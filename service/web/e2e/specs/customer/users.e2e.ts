// Customer users (FR-C19, DD-C19, IR268, IR417, IR461): the owner's own row shows when they last signed in — the setup
// project signed in through the identity provider, and the Core API records the token's sign-in time — and the add
// dialog refuses a missing name, a bad address and an existing user, then closes without adding anyone.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("the owner's own row shows the last sign-in", async ({ page }) => {
  await page.goto("/customer/users");
  await expect(page.getByRole("heading", { name: /^Users of \S/, level: 1 })).toBeVisible(); // names the customer (Figma 11a, IR315)
  const me = page.locator("main tr", { hasText: "(you)" });
  await expect(me).toBeVisible();
  await expect(me.locator("td").nth(3)).toHaveText(/^\d{1,2} \w+ \d{4}, \d{1,2}:\d{2} (am|pm) MYT$/); // IR44, not "—"
});

test("the add dialog refuses a missing name, a bad or existing address and adds nobody when cancelled", async ({ page }) => {
  await page.goto("/customer/users");
  const me = page.locator("main tr", { hasText: "(you)" });
  const email = ((await me.locator("td").first().innerText()).match(/\S+@\S+/) ?? [""])[0];
  expect(email).not.toBe("");
  await page.getByRole("button", { name: "Add user" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: /^Add a user to \S/ })).toBeVisible();
  await expectAccessible(page, '[role="dialog"]'); // the open dialog (IR360)
  await dialog.getByLabel("Email").fill("not-an-address");
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(dialog).toContainText("A valid email address (up to 254 characters)");
  await expect(dialog).toContainText("A name of 1–120 characters");
  await dialog.getByLabel("Email").fill(email.toUpperCase()); // the check ignores case
  await expect(dialog).toContainText("Already a user of this customer");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.locator("main tr", { hasText: "Not signed in yet" })).toHaveCount(0);
});

// IR417, IR461: the owner adds an owner (the account is created at once and the set-up e-mail reaches Mailpit), makes
// them a member with a reason, and removes the account; the own row has no actions.
test("the owner adds a user, changes the role and removes them", async ({ page }) => {
  await page.goto("/customer/users");
  await expect(page.locator("main tr", { hasText: "(you)" })).toContainText("Your account");
  const email = `e2e-${Date.now()}@example.com`;
  await page.getByRole("button", { name: "Add user" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name").fill("E2E Owner");
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("owner");
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(page.getByText(`E2E Owner added — a set-up e-mail is on its way to ${email}`).first()).toBeVisible();
  const row = page.locator("main tr", { hasText: email });
  await expect(row).toContainText("Not signed in yet");
  await expect(row).toContainText("Owner");
  const search = `http://localhost:8025/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  if ((await fetch(search).catch(() => null))?.ok) {
    await expect.poll(async () => ((await (await fetch(search)).json()) as { messages_count: number }).messages_count, { timeout: 30_000 }).toBeGreaterThanOrEqual(1);
  }
  await row.getByRole("button", { name: "Make member" }).click();
  await dialog.getByRole("button", { name: "Change role" }).click();
  await expect(dialog).toContainText("A reason of 1–1000 characters");
  await dialog.getByLabel("Reason").fill("Not an owner after all");
  await dialog.getByRole("button", { name: "Change role" }).click();
  await expect(row).toContainText("Member");
  await row.getByRole("button", { name: "Remove" }).click();
  await dialog.getByLabel("Reason").fill("Not joining after all");
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(row).toHaveCount(0);
});
