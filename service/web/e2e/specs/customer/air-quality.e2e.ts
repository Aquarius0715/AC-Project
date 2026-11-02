// Air quality (FR-C07, IR592, IR607): the 7-day PM2.5 table of Bedroom AC reads the whole week — hourly averages of
// the valid readings (telemetry.series bucketMinutes) — so no hour reads "Not loaded" and no notice says that earlier
// readings are not loaded; an hour without a valid reading is a gap. The table's scroll region takes the keyboard
// focus. Nothing is written.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("the air chart reads its whole window: nothing is left unloaded", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/customer/air-quality?metric=pm25&period=7d");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^PM2\.5 — last 7 days/ })).toBeVisible();
  await expect(main.getByText(/^Showing the latest \d+ of \d+ readings/)).toHaveCount(0);
  await main.getByRole("tab", { name: "Table" }).click();
  await expect(main.getByRole("table")).toBeVisible();
  await expect(main.getByRole("table")).not.toContainText("Not loaded");
  // the week's table scrolls on its own: a keyboard reaches and scrolls it (axe scrollable-region-focusable)
  const region = main.getByRole("region", { name: /^PM2\.5 — last 7 days/ });
  await region.focus();
  await expect(region).toBeFocused();
  await expectAccessible(page);
});
