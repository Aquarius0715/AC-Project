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

// A role without a page's read permission sees what it needs and whom to ask, not a failure with Try again (IR581).
// hq-override-only holds restriction.override alone.
test("a page the role may not read names the permission it needs", async ({ browser, app }) => {
  const context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } });
  try {
    const page = await context.newPage();
    await signIn(page, app, "hq-override-only");
    for (const [path, title] of [["/admin", "Overview needs dashboard.read"], ["/admin/alerts", "Alert policies needs alert.read"]] as const) {
      await page.goto(path);
      const main = page.getByRole("main");
      await expect(main).toContainText(title);
      await expect(main).toContainText("Ask an identity administrator");
      await expect(main.getByRole("button", { name: "Try again" })).toHaveCount(0);
    }
  } finally {
    await context.close();
  }
});
