// HQ billing (FR-A08, DD-A08, SCR-A08, Figma Admin 38:7, IR321): the billing months are a URL filter (from / to,
// Kuala Lumpur months) and the paid tile then counts the period; the invoice's related restriction links the
// restriction on the Restrictions screen. The seed has one invoice, INV-202608-0001 (August 2026, overdue), the cause of
// Demo Customer A's applied restriction. Nothing is paid, reminded or created.
import { test, expect } from "../../fixtures/test";

test("billing months filter the invoices and the related restriction opens on the Restrictions screen", async ({ page }) => {
  await page.goto("/admin/billing?from=2026-08&to=2026-08");
  const main = page.getByRole("main");
  await expect(main).toContainText("Paid in period");
  await expect(main).toContainText("Aug 2026");
  await expect(main.getByRole("button", { name: /INV-202608-0001/ })).toBeVisible();
  await expect(main.getByLabel("Billing months from")).toHaveValue("2026-08");

  // the related restriction: its short ID and a link to it on the Restrictions screen
  const related = main.locator("section").filter({ has: page.getByRole("heading", { name: "Related restriction", exact: true }) });
  await expect(related.getByRole("link", { name: "Open restriction →" })).toHaveAttribute("href", /^\/admin\/restrictions\?restrictionId=[0-9a-f-]{36}$/);
  await expect(related).toContainText("This invoice is a cause of this restriction.");

  // a month without invoices, then all months again
  await page.goto("/admin/billing?from=2026-09&to=2026-09");
  await expect(main.getByRole("heading", { name: "○ No invoices" })).toBeVisible();
  await main.getByRole("button", { name: "All months ✕" }).click();
  await page.waitForURL((u) => !u.searchParams.has("from") && !u.searchParams.has("to"));
  await expect(main).toContainText("in this scope");
  await expect(main.getByRole("button", { name: /INV-202608-0001/ })).toBeVisible();

  // a reversed range is not sent: the fields say why and the URL keeps the last valid period
  await page.goto("/admin/billing?from=2026-08");
  await main.getByLabel("Billing months to").fill("2026-07");
  await expect(main).toContainText("The first month must not be after the last");
  expect(new URL(page.url()).searchParams.has("to")).toBe(false);
});
