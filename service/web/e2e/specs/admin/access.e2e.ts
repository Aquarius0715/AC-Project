// HQ access & roles (FR-A03, DD-A03, SCR-A03, Figma Admin 539:16956, IR321): the organization filter is a
// search-select in the URL (organizationId) beside the role and status filters, and a membershipId the list does not
// hold says so instead of opening another membership. Nothing is saved.
import { test, expect } from "../../fixtures/test";

test("the organization filter narrows the memberships; an unknown membership is not opened", async ({ page }) => {
  await page.goto("/admin/settings/access");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "Memberships", exact: true })).toBeVisible();

  // the search-select: typing narrows the options, a click chooses one
  const org = main.getByRole("combobox", { name: "Filter by organization" });
  await expect(org).toHaveValue("Organization: All");
  await org.click();
  await org.fill("contractor a");
  const list = page.getByRole("listbox", { name: "Filter by organization" });
  await expect(list.getByRole("option")).toHaveCount(1);
  await list.getByRole("option", { name: "Demo Contractor A" }).click();
  await page.waitForURL(/organizationId=[0-9a-f-]{36}/);
  await expect(org).toHaveValue("Demo Contractor A");
  const rows = main.locator("button[aria-pressed]");
  await expect(rows.first()).toBeVisible();
  for (const label of await rows.locator("div.text-\\[11px\\]").allInnerTexts()) expect(label).toContain("Demo Contractor A");
  // Escape closes the list and keeps the choice
  await org.click();
  await org.press("Escape");
  await expect(list).toHaveCount(0);
  await expect(org).toHaveValue("Demo Contractor A");

  // the keyboard: typing narrows, Enter takes the active (first) match
  const before = new URL(page.url()).searchParams.get("organizationId");
  await org.click();
  await org.fill("demo hq");
  await org.press("ArrowDown"); // stays on the only match
  await org.press("Enter");
  await page.waitForURL((u) => !!u.searchParams.get("organizationId") && u.searchParams.get("organizationId") !== before);
  await expect(org).toHaveValue("Demo HQ");
  for (const label of await rows.locator("div.text-\\[11px\\]").allInnerTexts()) expect(label).toContain("Demo HQ");

  // a membership the list does not hold: no other membership opens in its place
  await page.goto("/admin/settings/access?role=admin&membershipId=00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "○ That membership is not in this list" })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Identity & role" })).toHaveCount(0);
  await main.getByRole("button", { name: "Clear the filters" }).click();
  await page.waitForURL((u) => !u.searchParams.has("role") && u.searchParams.get("membershipId") === "00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "○ That membership is not in this list" })).toBeVisible();
});
