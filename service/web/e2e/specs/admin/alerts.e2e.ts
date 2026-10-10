// HQ alerts (FR-A05, DD-A05 items 5 and 11, Figma Admin 256:2, IR318): the scope, severity, status and selected alert
// live in the URL; the detail explains why the alert was raised and its activity, and offers Request maintenance (New
// job for the unit, its symptom from the alert) and Open unit →. The seed has two open alerts on Demo Customer A: the
// warning "Possible open window" and the Info filter cleaning reminder. Nothing is acknowledged, resolved or created.
import { test, expect } from "../../fixtures/test";

test("the alerts tab filters in the URL and opens a job or the unit from an alert", async ({ page }) => {
  await page.goto("/admin/alerts");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "Open alerts · all customers", exact: true })).toBeVisible();
  await expect(main.getByRole("button", { name: /Possible open window/ })).toBeVisible();

  // the customer scope and the severity filter are URL state
  await main.getByRole("combobox", { name: "Customer" }).selectOption({ label: "Demo Customer A" });
  await page.waitForURL(/customerId=/);
  await expect(main.getByRole("heading", { name: "Open alerts · Demo Customer A", exact: true })).toBeVisible();
  await main.getByRole("combobox", { name: "Severity" }).selectOption({ label: "Severity: Warning" });
  await page.waitForURL(/severity=warning/);
  await expect(main.getByRole("button", { name: /Possible open window/ })).toBeVisible();
  await expect(main.getByRole("button", { name: /Filter cleaning reminder/ })).toHaveCount(0); // an Info reminder
  // the status tiles: nothing is resolved in the seed
  await main.getByRole("button", { name: /^Resolved\s*\d+$/ }).click();
  await page.waitForURL(/status=resolved/);
  await expect(main.getByText("No alerts in this scope")).toBeVisible();
  await main.getByRole("button", { name: /^Open\s*\d+$/ }).click();
  await page.waitForURL((u) => !u.searchParams.has("status"));

  // the detail: why, activity, and the two ways on
  await main.getByRole("button", { name: /Possible open window/ }).click();
  await page.waitForURL(/alertId=/);
  for (const name of ["Why we think this", "Activity"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
  await expect(main).toContainText("No policy escalation — this alert was not raised by a policy");
  await expect(main.getByRole("link", { name: "Open unit →" })).toHaveAttribute("href", /^\/admin\/units\?customerId=[0-9a-f-]{36}&unitId=[0-9a-f-]{36}$/);
  await main.getByRole("link", { name: "Request maintenance" }).click();
  await page.waitForURL(/\/admin\/jobs\?new=[0-9a-f-]{36}&alertId=/);
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: /New maintenance job/ })).toBeVisible();
  await expect(dialog.locator("textarea")).toHaveValue(/^Alert [0-9a-f]{8}: Possible open window — /); // the symptom from the alert
  expect(await dialog.locator("select").first().evaluate((s: HTMLSelectElement) => s.options[s.selectedIndex].text)).toContain("Bedroom AC");
  await dialog.getByRole("button", { name: "Cancel" }).click(); // nothing is created
  await expect(dialog).toHaveCount(0);
  await page.waitForURL((u) => !u.searchParams.has("new"));
});
