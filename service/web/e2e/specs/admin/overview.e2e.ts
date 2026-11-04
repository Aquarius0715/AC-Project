// HQ overview in business language (FR-A01, IR577): the power and connection rows link with "View", named by their
// state, and no requirement ID, API field name or list filter shows on the screen.
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

test("the overview speaks the business's words", async ({ page }) => {
  await page.goto("/admin");
  const main = page.getByRole("main");
  await expect(main.getByRole("link", { name: "View the units: Running" })).toHaveAttribute("href", /powerState=on/);
  await expect(main.getByRole("link", { name: "View the units: Online" })).toHaveAttribute("href", /connections=online/);
  await expect(main).not.toContainText(/\((?:IR|SR|A|D)\d+|powerState=|connections=|amountsByCurrency|unitMinutes|forecastSav|baseline_unavailable|_coverage|billing\.read/);
});

// A role without a page's read permission sees what it needs and whom to ask, not a failure with Try again (IR581,
// IR582). hq-override-only holds restriction.override alone, so every HQ page but Restrictions says so.
test("a page the role may not read names the permission it needs", async ({ browser, app }) => {
  const context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } });
  try {
    const page = await context.newPage();
    await signIn(page, app, "hq-override-only");
    const pages = [
      ["/admin", "Overview", "dashboard.read"], ["/admin/units", "Customers & units", "asset.read"], ["/admin/settings/access", "Access & roles", "identity.read"],
      ["/admin/devices", "Devices & models", "device.read"], ["/admin/alerts", "Alert policies", "alert.read"], ["/admin/jobs", "Maintenance jobs", "job.read"],
      ["/admin/billing/contracts", "Contracts", "contract.read"], ["/admin/billing", "Billing", "billing.read"], ["/admin/settings/automation", "Automation policies", "automation.policy.read"],
      ["/admin/energy", "Energy analysis", "energy.read"], ["/admin/mrv", "MRV", "mrv.read"], ["/admin/offsets", "Offsets", "offset.read"], ["/admin/audit", "Audit", "audit.read"],
    ] as const;
    for (const [path, what, permission] of pages) {
      await page.goto(path);
      const main = page.getByRole("main");
      await expect(main, path).toContainText(`To open ${what}, your role needs ${permission}.`);
      await expect(main.getByRole("button", { name: "Try again" })).toHaveCount(0);
    }
  } finally {
    await context.close();
  }
});

// "Waiting for HQ" counts the requests to book whatever their requested time and opens them with no period (DEC-90,
// IR611): the count matches the Jobs list's Requested tile.
test("Waiting for HQ opens the requests to book, with no period", async ({ page }) => {
  await page.goto("/admin?period=today");
  const main = page.getByRole("main");
  const link = main.getByRole("link", { name: "Open requests" });
  await expect(link).toHaveAttribute("href", "/admin/jobs?stage=requested");
  const tile = main.locator('[data-slot="card"]').filter({ hasText: "Waiting for HQ" });
  const countText = await tile.locator("div").filter({ hasText: /^\d+$/ }).first().innerText();
  const count = countText.trim();
  await link.click();
  await page.waitForURL(/\/admin\/jobs\?stage=requested$/);
  await expect(main.getByRole("button", { name: new RegExp(`^Requested ${count}`) }).or(main.getByText(new RegExp(String.raw`^Requested\s*${count}$`))).first()).toBeVisible();
});
