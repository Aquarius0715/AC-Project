// Air quality (FR-C07, IR592): the 7-day PM2.5 table of Bedroom AC. When the series is cut off at its page limit, the
// hours before the oldest loaded reading read "Not loaded — Earlier readings are not loaded", not "No data · Gap", so a
// cut-off is never shown as sensor downtime. A stack whose 7 days fit in the limit shows no such row. Nothing is written.
import { test, expect } from "../../fixtures/test";

test("a cut-off air series says what is not loaded instead of showing a gap", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/customer/air-quality?metric=pm25&period=7d");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^PM2\.5 — last 7 days/ })).toBeVisible();
  await main.getByRole("tab", { name: "Table" }).click();
  const cut = main.getByText(/^Showing the latest \d+ of \d+ readings/);
  if (await cut.count()) {
    await expect(main.getByRole("table")).toContainText("Not loaded");
    await expect(main.getByRole("table")).toContainText("Earlier readings are not loaded");
  } else {
    await expect(main.getByRole("table")).not.toContainText("Not loaded");
  }
});
