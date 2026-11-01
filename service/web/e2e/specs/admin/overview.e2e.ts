// HQ overview in business language (FR-A01, IR577): the power and connection rows link with "View", named by their
// state, and no requirement ID, API field name or list filter shows on the screen.
import { test, expect } from "../../fixtures/test";

test("the overview speaks the business's words", async ({ page }) => {
  await page.goto("/admin");
  const main = page.getByRole("main");
  await expect(main.getByRole("link", { name: "View the units: Running" })).toHaveAttribute("href", /powerState=on/);
  await expect(main.getByRole("link", { name: "View the units: Online" })).toHaveAttribute("href", /connections=online/);
  await expect(main).not.toContainText(/\((?:IR|SR|A|D)\d+|powerState=|connections=|amountsByCurrency|unitMinutes|forecastSav|baseline_unavailable|_coverage|billing\.read/);
});
