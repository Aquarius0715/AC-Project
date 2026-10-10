// HQ overview → Jobs tab with the stage, the scope and the requested-time period (IR244, IR245): the status rows link
// with from / to, the tab names the period on a chip and its stage totals follow it, and the chip clears it.
import { test, expect } from "../../fixtures/test";

test("a job-status row opens the Jobs tab for that period", async ({ page }) => {
  await page.goto("/admin?period=30d");
  const rows = page.locator("main a[href^='/admin/jobs?'][href*='stage=']");
  await expect(rows.first()).toBeVisible();
  const texts = await rows.allInnerTexts();
  const i = Math.max(0, texts.findIndex((t) => /\s[1-9]\d*$/.test(t.replace(/\s+/g, " ").trim())));
  const count = Number(texts[i].replace(/\s+/g, " ").trim().match(/(\d+)$/)?.[1] ?? 0);
  const href = (await rows.nth(i).getAttribute("href"))!;
  expect(href).toMatch(/[?&]from=.+&to=.+/);
  await rows.nth(i).click();
  await page.waitForURL(/\/admin\/jobs\?.*from=/);
  const chip = page.getByRole("button", { name: "Clear the requested-time period" });
  await expect(chip).toContainText(/^Requested time \d{1,2} \S+.* – .+ MYT/); // one span in the display time zone (IR290)
  await expect(page.getByRole("main")).toContainText(/jobs? in scope · in the period/);
  await expect(page.locator("main button[aria-pressed='true']").first()).toContainText(String(count)); // the stage total follows the period
  await chip.click();
  await page.waitForURL((u) => !u.searchParams.has("from"));
  await expect(chip).toHaveCount(0);
});
