// Choosing in a select that writes the URL (the scope bar: customer, property, unit). A select in a part of the page
// that hydrates after the shell can drop a change made just before (a busy CI runner: alert-policies, Dependabot #7 and
// #23), and the URL never follows; choosing again until it does keeps the test about the screen, not the timing.
import { expect, type Locator, type Page } from "@playwright/test";

export async function selectForURL(page: Page, select: Locator, option: { label: string } | string, url: RegExp): Promise<void> {
  await expect(async () => {
    await select.selectOption(option);
    await page.waitForURL(url, { timeout: 5_000 });
  }).toPass({ timeout: 60_000 });
}
