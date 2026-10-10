// HQ offsets (FR-A15, DD-A15, SCR-A15, Figma Admin 106:9, IR324): the record filters — customer, status and created,
// the last 30 days by default — live in the URL, and a recordId the list does not hold says so. The seed has no offset
// records. Nothing is quoted or requested.
import { test, expect } from "../../fixtures/test";

test("the offset record filters live in the URL", async ({ page }) => {
  await page.goto("/admin/offsets");
  const main = page.getByRole("main");
  await expect(main.getByLabel("Created")).toHaveValue("30d");
  await main.getByLabel("Status").selectOption({ label: "Demo retired" });
  await page.waitForURL(/status=demo_retired/);
  await main.getByLabel("Customer").selectOption({ label: "Demo Customer A" });
  await page.waitForURL(/customerId=[0-9a-f-]{36}/);
  await main.getByLabel("Created").selectOption("all");
  await page.waitForURL(/created=all/);
  await expect(main).toContainText("No record matches these filters");
  await main.getByLabel("Created").selectOption("30d"); // the default leaves the URL
  await page.waitForURL((u) => !u.searchParams.has("created"));

  await page.goto("/admin/offsets?recordId=00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "○ That record is not in this list" })).toBeVisible();
});
