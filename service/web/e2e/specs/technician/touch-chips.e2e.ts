// A technician's phone (IR615, IR616): on a 390×844 touch screen the overview's job tabs and the devices' state
// filters are at least 44 px tall, as the shared controls are, and still filter; a filter that hides the open device
// opens its first device instead. Nothing is written.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });

test("the job tabs and the device filters are touch-sized and filter", async ({ page }) => {
  const tall = async (loc: import("@playwright/test").Locator) => {
    for (const box of await loc.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height))) expect(box).toBeGreaterThanOrEqual(43.5);
  };
  await page.goto("/technician");
  const tabs = page.getByRole("main").getByRole("tab");
  await expect(tabs.first()).toBeVisible();
  await tall(tabs);
  await tabs.filter({ hasText: /^All assigned/ }).tap();
  await page.waitForURL(/tab=all/);
  await expect(tabs.filter({ hasText: /^All assigned/ })).toHaveAttribute("aria-selected", "true");

  await page.goto("/technician/devices");
  const main = page.getByRole("main");
  const filters = main.locator("button[aria-pressed]");
  await expect(filters.first()).toBeVisible();
  await tall(filters);
  await filters.filter({ hasText: /^Offline/ }).tap();
  await expect(filters.filter({ hasText: /^Offline/ })).toHaveAttribute("aria-pressed", "true");
  const listed = main.getByRole("button").filter({ hasText: /^AC-/ }).first(); // the filter's first device
  if (await listed.count()) { // the detail follows the filter: its heading is that device's serial
    const listedText = await listed.innerText();
    const serial = (listedText.split("\n", 1)[0] ?? "").trim();
    await expect(main.getByRole("heading", { name: serial, exact: true })).toBeVisible();
  }
  await expectAccessible(page);
});
