// Partner jobs list (FR-P02, DD-P01, IR225): each status tab is in the URL, is selected, and lists jobs that open their
// detail — or shows the empty state when its count is 0.
import { test, expect } from "../../fixtures/test";

test("the status tabs keep the URL and open the jobs they list", async ({ page }) => {
  await page.goto("/partner/jobs");
  await expect(page.getByRole("tablist").first()).toBeVisible(); // after the streamed loading state
  const tabs = page.getByRole("tablist").first().getByRole("tab");
  const names = await tabs.allInnerTexts();
  expect(names.length).toBeGreaterThan(1);
  for (let i = names.length - 1; i >= 0; i--) {
    await tabs.nth(i).click();
    await expect(tabs.nth(i)).toHaveAttribute("aria-selected", "true");
    if (i > 0) await page.waitForURL(/[?&]tab=/);
    else await page.waitForURL((u) => !u.searchParams.has("tab"));
    const count = Number(names[i].match(/(\d+)\s*$/)?.[1] ?? NaN);
    const jobs = page.locator("main a[href^='/partner/jobs/']");
    if (count === 0) await expect(jobs).toHaveCount(0);
    else if (count > 0) await expect(jobs.first()).toBeVisible();
  }
  const first = page.locator("main a[href^='/partner/jobs/']").first();
  test.skip((await first.count()) === 0, "no jobs for this contractor");
  const href = (await first.getAttribute("href"))!;
  await first.click();
  await page.waitForURL((u) => u.pathname === href.split("?")[0]);
  await expect(page.getByRole("main")).not.toContainText("This page isn’t available");
});
