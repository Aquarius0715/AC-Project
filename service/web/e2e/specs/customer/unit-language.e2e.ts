// AT-X01-N ③ (FR-X01, IR44, IR259): with Malay and another display time zone saved in Preferences, the customer's unit
// screen speaks Malay and shows its times in that zone with the zone's abbreviation, while the unit's ID and units (°C)
// stay as stored. English and the time zone are put back in finally.
import { test, expect } from "../../fixtures/test";
import type { Page } from "@playwright/test";

const SAVE = /Save preferences|Simpan keutamaan/;
const SAVED = /Preferences saved|Keutamaan disimpan/;

/** Saves the display language and time zone on Preferences, whichever language the page shows. */
async function setDisplay(page: Page, language: RegExp, zone: string) {
  await page.goto("/settings/preferences");
  await page.locator("main select").first().selectOption(zone);
  const choice = page.getByRole("button", { name: language });
  if ((await choice.getAttribute("aria-pressed")) !== "true") await choice.click();
  const save = page.getByRole("button", { name: SAVE });
  if (!(await save.isEnabled())) return;
  const answered = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === "/settings/preferences");
  await save.click();
  await answered;
  await expect(page.getByText(SAVED).last()).toBeVisible();
}

test("the unit screen in Malay keeps the ID and units and shows times in the display time zone", async ({ page }) => {
  await page.goto("/settings/preferences");
  const zone = await page.locator("main select").first().inputValue();
  await page.goto("/customer"); // the overview lists every AC with a link to its screen
  const href = await page.locator('main a[href^="/customer/units/"]').first().getAttribute("href");
  expect(href).toBeTruthy();
  try {
    await setDisplay(page, /^Bahasa Melayu$/, "Asia/Tokyo");
    await page.goto(href!);
    const main = page.getByRole("main");
    for (const name of ["KAWALAN JAUH", "Sejarah arahan", "Telemetri langsung"]) await expect(main.getByRole("heading", { name })).toBeVisible();
    expect(new URL(page.url()).pathname).toBe(href); // the ID is the stored one
    await expect(main).toContainText("°C");
    await expect(main).toContainText("GMT+9"); // Asia/Tokyo, not the Malaysian MYT
    await expect(main).not.toContainText("MYT");
  } finally {
    await setDisplay(page, /English|Inggeris/, zone);
  }
  await page.goto(href!);
  await expect(page.getByRole("main").getByRole("heading", { name: "REMOTE CONTROL" })).toBeVisible();
});
