// HQ customers & units (FR-A02, Figma Admin 223:2, IR319): the Contract column marks each customer's standing. The seed's
// Demo Customer A has an overdue invoice and an applied restriction; hq-operator holds no restriction.read, so the
// restriction shows from the contracts' activeRestrictionIds as "Restriction active", with no link to the restrictions
// screen it cannot open.
import { test, expect } from "../../fixtures/test";

test("a customer's row marks the overdue invoice and the active restriction", async ({ page }) => {
  await page.goto("/admin/units");
  const row = page.getByRole("main").getByRole("row", { name: /Demo Customer A/ });
  await expect(row).toContainText("‼ Overdue");
  await expect(row).toContainText("Restriction active");
  await row.click();
  await page.waitForURL(/customerId=/);
  const tile = page.getByRole("main").getByText("Restriction active").first();
  await expect(tile).toBeVisible(); // the header's billing tile says so too
  await expect(page.getByRole("main").getByRole("link", { name: "View restriction →" })).toHaveCount(0);
});
