// Job history (FR-P07, FR-P08, IR589, IR614): the list opens on the last 90 days, where a cancelled job or one whose
// delegation ended unfinished counts by when it ended. When the 90 days hold no job but the company has older ones, the
// empty list says how many and "Show all time" lists them. The CI seed's 90 days hold its job, so there the list itself
// is checked. Nothing is written.
import { test, expect } from "../../fixtures/test";

test("an empty 90-day history leads to the jobs under all time", async ({ page }) => {
  await page.goto("/partner/history");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^Your jobs — / })).toBeVisible();
  const empty = main.getByRole("heading", { name: "No jobs in the last 90 days" });
  if (await empty.count()) {
    await expect(main).toContainText(/\d+ jobs? of your company (is|are) older than 90 days/);
    await main.getByRole("button", { name: "Show all time" }).click();
    await page.waitForURL(/period=all/);
  }
  await expect(main.getByRole("table")).toBeVisible();
  await expect(main.getByRole("table").getByRole("row").nth(1)).toBeVisible();
});
