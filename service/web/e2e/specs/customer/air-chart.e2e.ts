// Air quality chart (FR-C07, IR689): the tooltip says what each value is and when it was measured, and the chart takes
// several metrics at once — each on the axis of its unit. Nothing is written.
import { test, expect } from "../../fixtures/test";

test("hovering the chart names the value, its unit and the time; two metrics share one chart", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/customer/air-quality?metric=pm25&period=7d");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^PM2\.5 — last 7 days/ })).toBeVisible();
  const chart = main.locator(".recharts-wrapper").first();
  await expect(chart).toBeVisible();
  const box = await chart.boundingBox();
  expect(box).not.toBeNull();
  if (!box) return;
  // the readings of the week are hourly averages: some point under the pointer carries a value (a gap shows nothing)
  let tooltip = "";
  for (let k = 1; k <= 12 && !tooltip.includes("µg/m³"); k++) {
    await page.mouse.move(box.x + (box.width * k) / 13, box.y + box.height / 2);
    tooltip = (await page.locator(".recharts-tooltip-wrapper").first().textContent()) ?? "";
  }
  expect(tooltip).toContain("PM2.5");
  expect(tooltip).toContain("µg/m³");
  expect(tooltip).not.toContain("Value");
  expect(tooltip).toMatch(/\d{1,2}:\d{2}/); // the hour the point averages
  // a second metric joins the chart (CO2 ppm: another unit, so the right axis)
  await main.getByRole("button", { name: "CO2", exact: true }).click();
  await expect(page).toHaveURL(/metric=pm25%2Cco2|metric=pm25,co2/);
  await expect(main.getByRole("heading", { name: /^PM2\.5 · CO2 — last 7 days/ })).toBeVisible();
  await expect(main.locator(".recharts-line-curve")).toHaveCount(2);
  await expect(main.locator(".recharts-yAxis")).toHaveCount(2);
  // a third unit does not fit: its chip is off until one is taken out
  await expect(main.getByRole("button", { name: "Temp", exact: true })).toBeDisabled();
  await main.getByRole("button", { name: "CO2", exact: true }).click();
  await expect(main.locator(".recharts-line-curve")).toHaveCount(1);
});
