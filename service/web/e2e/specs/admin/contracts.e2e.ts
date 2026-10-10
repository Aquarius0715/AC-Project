// HQ contracts (FR-A07, DD-A07, SCR-A07, Figma Admin 106:4, IR322): the scope with its contract count, the plan tabs
// with their counts, the eligibility of each row, and the editor of the seed's RTO contract — its customer & units,
// plan and eligibility sections, and Save disabled while its restriction is active (SR19) with a link to that
// restriction. The seed has two contracts of Demo Customer A: RTO (eligible, demo-v1, restricted) and General. Nothing
// is saved.
import { test, expect } from "../../fixtures/test";

test("the contract scope, plan counts, eligibility and the restricted contract's editor", async ({ page }) => {
  await page.goto("/admin/billing/contracts");
  const main = page.getByRole("main");
  await expect(main).toContainText("2 contracts in scope");
  for (const [name, n] of [["All", 2], ["RTO", 1], ["General", 1], ["Energy", 0], ["Environment", 0]] as const) await expect(main.getByRole("tab", { name: new RegExp(`^${name}\\s*${n}$`) })).toBeVisible();
  const list = main.locator("section").filter({ has: page.getByRole("heading", { name: "Contracts", exact: true }) });
  const rto = list.locator("button[aria-pressed]").filter({ hasText: "RTO" });
  await expect(rto).toContainText("Eligible · demo-v1 · restriction active");
  await expect(list.locator("button[aria-pressed]").filter({ hasText: "General" })).toContainText("Not restriction eligible");

  // the RTO contract: its sections, and Save disabled while the restriction is active
  await rto.click();
  await page.waitForURL(/contractId=[0-9a-f-]{36}/);
  for (const name of ["Customer & units", "Plan", "Restriction eligibility"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
  await expect(main).toContainText("1 active restriction");
  await expect(main).toContainText("Save is disabled (SR19)");
  await expect(main.getByRole("link", { name: "Open restriction →" })).toHaveAttribute("href", /^\/admin\/restrictions\?restrictionId=[0-9a-f-]{36}$/);
  await expect(main.getByRole("button", { name: "Save as version 2" })).toBeDisabled();

  // a scope without contracts
  await main.getByLabel("Customer").first().selectOption({ label: "Demo Customer B" });
  await page.waitForURL(/customerId=[0-9a-f-]{36}/);
  await expect(main).toContainText("0 contracts in scope");
  await expect(main.getByRole("heading", { name: "○ No contracts" })).toBeVisible();
});
