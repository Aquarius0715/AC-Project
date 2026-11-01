// Payouts (FR-P10, IR584): the seed has no approved or paid statement, so the page says when the first one appears
// and the empty detail does not ask to select one. A filter that matches nothing says so and offers "Clear the
// filters", as HQ's payouts tab does. Nothing is written.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("an empty payouts page explains itself and clears its filters", async ({ page }) => {
  await page.goto("/partner/payouts");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "No statements yet" })).toBeVisible();
  await expect(main).toContainText("A statement appears here once HQ approves your first one.");
  await expect(main.getByRole("heading", { name: "No statement to show" })).toBeVisible();
  await expect(main.getByRole("button", { name: "Clear the filters" })).toHaveCount(0);
  await expectAccessible(page);

  // a period without statements
  await page.goto("/partner/payouts?period=2099-01");
  await expect(main.getByRole("heading", { name: "No statements", exact: true })).toBeVisible();
  await expect(main).toContainText("No statement matches these filters.");
  await main.getByRole("button", { name: "Clear the filters" }).click();
  await page.waitForURL((u) => !u.searchParams.has("period"));
  await expect(main.getByRole("heading", { name: "No statements yet" })).toBeVisible();
  await expect(main.getByRole("button", { name: "Clear the filters" })).toHaveCount(0);
});
