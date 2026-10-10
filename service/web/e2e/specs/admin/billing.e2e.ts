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

// Contractor payouts (FR-A23, DD-A23, Figma Admin 699:22119, IR322): the period, contractor and status filters live in
// the URL; Generate drafts takes only a month that has ended (the seed has no statements). Nothing is generated.
test("the payouts filters live in the URL and Generate drafts takes only an ended month", async ({ page }) => {
  await page.goto("/admin/billing?tab=payouts");
  const main = page.getByRole("main");
  await expect(main.getByRole("tab", { name: /^Contractor payouts/ })).toHaveAttribute("aria-selected", "true");
  await main.getByLabel("Contractor").selectOption({ label: "Demo Contractor A" });
  await page.waitForURL(/contractorOrgId=[0-9a-f-]{36}/);
  await main.getByLabel("Status").selectOption({ label: "Approved" });
  await page.waitForURL(/status=approved/);
  await expect(main.getByText("No statement matches these filters.")).toBeVisible();
  await main.getByRole("button", { name: "Clear the filters" }).click();
  await page.waitForURL((u) => !u.searchParams.has("contractorOrgId") && !u.searchParams.has("status"));

  // Generate drafts: the last month that has ended is the default; a month still running is refused before any call
  await main.getByRole("button", { name: "Generate drafts" }).click();
  const dialog = page.getByRole("dialog");
  const month = dialog.getByLabel("Month");
  await expect(month).toHaveValue(/^\d{4}-\d{2}$/);
  await month.fill("2099-01");
  await expect(dialog).toContainText("Only a month that has ended (Kuala Lumpur) can be generated");
  await expect(dialog.getByRole("button", { name: "Generate drafts" })).toBeDisabled();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);

  // a statement the filtered list does not hold
  await page.goto("/admin/billing?tab=payouts&statementId=00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "○ That statement is not in this list" })).toBeVisible();
});
