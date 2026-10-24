// HQ client users (FR-A17, IR461): HQ adds a client user to a customer — the account is created at once and the sign-in
// service e-mails the set-up link, which reaches Mailpit — sees them listed as not signed in yet, and removes them
// again. Demo Customer B, so the customer app's own users spec (customer A) sees nothing of it.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("HQ adds a client user, listed as not signed in yet, and removes them", async ({ page }) => {
  await page.goto("/admin/units");
  const main = page.getByRole("main");
  await main.getByRole("row", { name: /Demo Customer B/ }).click();
  await page.waitForURL(/customerId=/);
  await main.getByRole("tab", { name: /^Users/ }).click();
  await main.getByRole("button", { name: "Add user" }).click();
  const dialog = page.getByRole("dialog");
  await expectAccessible(page, '[role="dialog"]'); // the add dialog (IR360)
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(dialog).toContainText("A name of 1–120 characters"); // nothing typed yet
  const email = `e2e-client-${Date.now()}@example.com`;
  await dialog.getByRole("textbox", { name: /^Name/ }).fill("E2E Client");
  await dialog.getByRole("textbox", { name: /^Email/ }).fill(email);
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(page.getByText(`E2E Client added — a set-up e-mail is on its way to ${email}`).first()).toBeVisible();
  const row = main.getByRole("row").filter({ hasText: email });
  await expect(row).toContainText("Not signed in yet");
  const search = `http://localhost:8025/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  if ((await fetch(search).catch(() => null))?.ok) {
    await expect.poll(async () => ((await (await fetch(search)).json()) as { messages_count: number }).messages_count, { timeout: 30_000 }).toBeGreaterThanOrEqual(1);
  }
  await row.getByRole("button", { name: `Actions for ${email}` }).click();
  await page.getByRole("menuitem", { name: "Remove from customer" }).click();
  await dialog.getByRole("textbox", { name: /^Reason/ }).fill("E2E: added by mistake");
  await dialog.getByRole("button", { name: "Remove user" }).click();
  await expect(row).toHaveCount(0);
});
