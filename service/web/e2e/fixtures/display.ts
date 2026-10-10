// The signed-in user's display language and time zone (Preferences, FR-X01, IR258), set on the page in either language.
// Specs that change them put English and the earlier zone back in afterEach: it also runs after a test timeout, while a
// finally block in a timed-out test only runs once the page is gone (R256 left a user in Malay that way).
import { expect, type Page } from "@playwright/test";

export const ENGLISH = /English|Inggeris/;
export const MALAY = /^Bahasa Melayu$/;
const SAVE = /Save preferences|Simpan keutamaan/;
const SAVED = /Preferences saved|Keutamaan disimpan/;

/** Saves Preferences and waits for the Server Action's answer and its toast (Save is also disabled while pending). */
export async function savePreferences(page: Page) {
  const answered = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === "/settings/preferences");
  await page.getByRole("button", { name: SAVE }).click();
  await answered;
  await expect(page.getByText(SAVED).last()).toBeVisible();
}

/** The saved display time zone. */
export async function displayZone(page: Page): Promise<string> {
  await page.goto("/settings/preferences");
  return page.locator("main select").first().inputValue();
}

/** Saves a display language and time zone, whichever language the page shows; nothing when they are already set. */
export async function setDisplay(page: Page, language: RegExp, zone: string) {
  await page.goto("/settings/preferences");
  await page.locator("main select").first().selectOption(zone);
  const choice = page.getByRole("button", { name: language });
  if ((await choice.getAttribute("aria-pressed")) !== "true") await choice.click();
  if (await page.getByRole("button", { name: SAVE }).isEnabled()) await savePreferences(page);
}
