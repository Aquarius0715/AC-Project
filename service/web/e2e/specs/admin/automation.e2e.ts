// HQ automation policies (FR-A11, DD-A11, Figma Admin 106:5 / 361:7456, IR323): the editor's Basics, the save footer
// (No unsaved changes / Discard), Show disabled, and Simulate with + Add fact — a tariff fact and an added solar fact
// that is missing — run without creating a command. The test creates one policy, disabled so that no evaluation runs
// it, and deletes it again in afterEach through the app's operation relay (policies.delete with its version).
import { test, expect } from "../../fixtures/test";

let created: { id: string; version: number } | null = null;

test.afterEach(async ({ page }) => {
  if (!created) return;
  const status = await page.evaluate(async ({ id, version }) => (await fetch("/bff/ops/policies.delete", {
    method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID(), "X-Expected-Version": String(version) }, body: JSON.stringify({ policyId: id }),
  })).status, created);
  expect(status).toBe(200);
  created = null;
});

test("the policy editor's footer, Show disabled and a simulation with an added fact", async ({ page }) => {
  await page.goto("/admin/settings/automation?policyId=new");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "Basics", exact: true })).toBeVisible();
  await main.getByLabel("Name").fill(`E2E tariff policy ${Date.now()}`);
  await main.getByRole("checkbox", { name: "Bedroom AC", exact: true }).check();
  await main.getByRole("button", { name: "Create policy" }).click();
  await page.waitForURL(/policyId=[0-9a-f-]{36}/);
  created = { id: new URL(page.url()).searchParams.get("policyId")!, version: 1 };

  // the footer: a loaded form is clean, an edit marks it, Discard puts it back
  await expect(main).toContainText("No unsaved changes · form loaded from policy v1");
  await main.getByLabel("Name").fill("changed name");
  await expect(main).toContainText("Unsaved changes — saving creates version 2");
  await main.getByRole("button", { name: "Discard" }).click();
  await expect(main).toContainText("No unsaved changes · form loaded from policy v1");

  // Show disabled: the new policy is disabled, yet it stays listed while it is open
  const list = main.locator("section").filter({ has: page.getByRole("heading", { name: "HQ automation policies", exact: true }) });
  await main.getByRole("checkbox", { name: "Show disabled" }).uncheck();
  await expect(list.getByRole("button", { name: /E2E tariff policy/ })).toBeVisible();

  // Simulate: the policy's tariff fact, + Add fact for a missing solar reading, Run simulation (no command)
  const sim = main.locator("section").filter({ has: page.getByRole("heading", { name: "Simulate", exact: true }) });
  await sim.getByLabel("Value", { exact: true }).first().fill("0.7");
  await sim.getByRole("button", { name: "+ Add fact" }).click();
  await expect(sim.getByLabel("Fact", { exact: true })).toHaveCount(2);
  await sim.getByLabel("Fact", { exact: true }).nth(1).selectOption("solar");
  await sim.getByLabel("Quality", { exact: true }).nth(1).selectOption("missing");
  await sim.getByRole("button", { name: "Run simulation" }).click();
  await expect(sim).toContainText("Result per unit");
  await expect(sim.locator("table").last().getByText("Bedroom AC", { exact: true })).toHaveCount(1); // one row for the unit, whatever its facts
  await sim.getByRole("button", { name: "Remove the fact" }).nth(1).click();
  await expect(sim.getByLabel("Fact", { exact: true })).toHaveCount(1);
});
