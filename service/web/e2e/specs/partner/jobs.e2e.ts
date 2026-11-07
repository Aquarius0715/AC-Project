// Partner jobs list (FR-P02, DD-P01, IR225): each status tab is in the URL, is selected, and lists jobs that open their
// detail — or, for a job whose delegation ended in the period, its history (IR614) — or shows the empty state when its
// count is 0.
import { test, expect } from "../../fixtures/test";
import { must } from "../../fixtures/must";

test("the status tabs keep the URL and open the jobs they list", async ({ page }) => {
  await page.goto("/partner/jobs");
  await expect(page.getByRole("tablist").first()).toBeVisible(); // after the streamed loading state
  const tabs = page.getByRole("tablist").first().getByRole("tab");
  const names = await tabs.allInnerTexts();
  expect(names.length).toBeGreaterThan(1);
  for (let i = names.length - 1; i >= 0; i--) {
    await tabs.nth(i).click();
    await expect(tabs.nth(i)).toHaveAttribute("aria-selected", "true");
    await (i > 0 ? page.waitForURL(/[?&]tab=/) : page.waitForURL((u) => !u.searchParams.has("tab")));
    const count = Number((/(\d+)\s*$/.exec(must(names.at(i))))?.[1] ?? NaN);
    const jobs = page.locator("main a[href^='/partner/jobs/'], main a[href^='/partner/history?jobId=']");
    if (count === 0) await expect(jobs).toHaveCount(0);
    else if (count > 0) await expect(jobs.first()).toBeVisible();
  }
  const first = page.locator("main a[href^='/partner/jobs/']").first();
  test.skip((await first.count()) === 0, "no jobs for this contractor");
  const href = must((await first.getAttribute("href")));
  await first.click();
  await page.waitForURL((u) => u.pathname === href.split("?", 1)[0]);
  await expect(page.getByRole("main")).not.toContainText("This page isn’t available");
});
