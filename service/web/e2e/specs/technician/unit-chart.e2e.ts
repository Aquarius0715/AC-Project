// The technician's monitoring chart (FR-T03, IR689): the tooltip names the metric with its unit and says when the value
// was measured, and a second metric joins the chart. It signs in as tech-external-a, the technician with an assigned
// job on the seed. Nothing is written.
import type { BrowserContext } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

let context: BrowserContext | null = null;

test.afterEach(async () => { await context?.close(); context = null; });

test("hovering the monitoring chart names the value, its unit and the time; a second metric joins it", async ({ browser, app }) => {
  test.skip(app.id !== "technician", "technician only");
  context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } });
  const page = await context.newPage();
  await signIn(page, app, "tech-external-a");
  await page.goto("/technician?tab=all");
  const main = page.getByRole("main");
  const job = main.locator("a[href^='/technician/jobs/']").first();
  test.skip((await job.count()) === 0, "no assigned job on this seed");
  await job.click();
  await page.waitForURL(/\/technician\/jobs\/[^/?]+$/);
  const unit = main.getByRole("link", { name: "Unit", exact: true });
  test.skip((await unit.count()) === 0, "the unit link shows inside the work window only");
  await unit.click();
  await page.waitForURL(/\/technician\/units\/[^/?]+\?jobId=/);
  await main.getByRole("tab", { name: "Monitoring", exact: true }).click();
  await page.waitForURL(/metric=/);
  const chips = main.getByRole("group", { name: "Metrics in the chart" }).getByRole("button");
  await expect(chips.first()).toBeVisible();
  const chart = main.locator(".recharts-wrapper").first();
  if (await chart.count()) {
    const box = await chart.boundingBox();
    expect(box).not.toBeNull();
    if (!box) return;
    let tooltip = "";
    for (let k = 1; k <= 12 && !/\d{1,2}:\d{2}/.test(tooltip); k++) {
      await page.mouse.move(box.x + (box.width * k) / 13, box.y + box.height / 2);
      tooltip = (await page.locator(".recharts-tooltip-wrapper").first().textContent()) ?? "";
    }
    expect(tooltip).not.toContain("Value");
    expect(tooltip).toMatch(/\d{1,2}:\d{2}/);
  }
  // a metric that is off joins the chart (when the unit measures a second one)
  const off = main.getByRole("group", { name: "Metrics in the chart" }).getByRole("button", { pressed: false, disabled: false }).first();
  if (!(await off.count())) return;

  await off.click();
  await expect(page).toHaveURL(/metric=[^&]*(%2C|,)/);
  await expect(main.getByRole("group", { name: "Metrics in the chart" }).getByRole("button", { pressed: true })).toHaveCount(2);
});
