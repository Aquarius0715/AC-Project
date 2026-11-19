// Air quality chart (FR-C07, IR689): the tooltip says what each value is and when it was measured, and the chart takes
// several metrics at once — one or two share a chart (axis per unit), three or more get a chart each. Nothing is written.
import { test, expect } from "../../fixtures/test";

test("hovering the chart names the value, its unit and the time; metrics share one chart, three or more get one each", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/customer/air-quality?metric=pm25&period=7d");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^PM2\.5 — last 7 days/ })).toBeVisible();
  const chart = main.locator(".recharts-wrapper").first();
  await expect(chart).toBeVisible();
  const box = await chart.boundingBox();
  expect(box).not.toBeNull();
  if (!box) return;
  // the readings of the week are hourly averages and a gap shows nothing. The E2E stack's room may hold only the
  // current hour's reading, at the right end, so the pointer starts there and moves left (IR730)
  let tooltip = "";
  for (let k = 0; k <= 12 && !tooltip.includes("µg/m³"); k++) {
    await page.mouse.move(box.x + box.width * (1 - k / 13) - 4, box.y + box.height / 2);
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
  // a third and a fourth unit join too: three or more get a chart each on the same time axis (IR793)
  await main.getByRole("button", { name: "Temp", exact: true }).click();
  await main.getByRole("button", { name: "Humidity", exact: true }).click();
  await expect(page).toHaveURL(/metric=pm25%2Cco2%2Ctemperature%2Chumidity|metric=pm25,co2,temperature,humidity/);
  await expect(main.locator(".recharts-wrapper")).toHaveCount(4);
  // "All" goes back to the first metric and shows the four again
  await main.getByRole("button", { name: "All", exact: true }).click();
  await expect(main.locator(".recharts-wrapper")).toHaveCount(1);
  await main.getByRole("button", { name: "All", exact: true }).click();
  await expect(main.locator(".recharts-wrapper")).toHaveCount(4);
});
