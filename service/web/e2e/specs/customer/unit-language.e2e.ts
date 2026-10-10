// AT-X01-N ③ (FR-X01, IR44, IR259–IR261): with Malay and another display time zone saved in Preferences, the customer's
// overview, alerts and unit screen speak Malay and show their times in that zone with the zone's abbreviation, while
// the unit's ID and units (°C) stay as stored. English and the time zone are put back in finally.
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

test("the overview, alerts and unit screen in Malay keep IDs and units and show times in the display time zone", async ({ page }) => {
  await page.goto("/settings/preferences");
  const zone = await page.locator("main select").first().inputValue();
  await page.goto("/customer"); // the overview lists every AC with a link to its screen
  const href = await page.locator('main a[href^="/customer/units/"]').first().getAttribute("href");
  expect(href).toBeTruthy();
  try {
    await setDisplay(page, /^Bahasa Melayu$/, "Asia/Tokyo");
    await page.goto("/customer");
    const main = page.getByRole("main");
    for (const name of ["Tenaga digunakan", "Anggaran pelepasan", "Perlu perhatian", "Automasi"]) await expect(main.getByRole("heading", { name })).toBeVisible();
    await expect(main).toContainText(/Dikemas kini \d{1,2}:\d{2} (PG|PTG) GMT\+9/); // the overview's read time in Asia/Tokyo
    await expect(main).toContainText("Asia/Kuala_Lumpur"); // the period note: the periods are Kuala Lumpur days
    await page.goto("/customer/alerts");
    for (const name of ["Perlu perhatian", "Peringatan & maklumat"]) await expect(main.getByRole("heading", { name })).toBeVisible();
    await expect(main).toContainText(/Dikemas kini \d{1,2}:\d{2} (PG|PTG) GMT\+9/);
    await page.getByRole("tab", { name: /^Polisi amaran/ }).click();
    await page.waitForURL(/tab=policies/);
    await expect(main.getByRole("heading", { name: "Polisi amaran" })).toBeVisible();
    await expect(main.getByRole("columnheader", { name: "Peraturan" })).toBeVisible();
    await page.goto(href!);
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
