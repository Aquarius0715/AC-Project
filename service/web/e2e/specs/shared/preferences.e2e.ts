// Preferences on the Core API (FR-X01, FR-X08, IR246, IR247, IR258): the time zone and language are saved and read back
// (Malay puts the shell and the screen in Malay), a client's location consent and monthly report e-mail too, and
// two-step verification turns on (setup QR, six code boxes, recovery codes once) and off. Every change is put back.
import { test, expect } from "../../fixtures/test";
import { displayZone, ENGLISH, savePreferences as save, setDisplay } from "../../fixtures/display";
import { expectAccessible } from "../../fixtures/a11y";

// English and the earlier zone come back at the end of the language test, or here when it fails (also on a timeout)
let zoneBefore: string | null = null;
test.afterEach(async ({ page }) => {
  if (zoneBefore) await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
});

test("language and time zone are saved and read back", async ({ page }) => {
  const before = await displayZone(page);
  const target = before === "Asia/Tokyo" ? "Asia/Singapore" : "Asia/Tokyo";
  zoneBefore = before;
  await page.locator("main select").first().selectOption(target);
  await page.getByRole("button", { name: "Bahasa Melayu" }).click();
  await save(page);
  await page.reload();
  await expect(page.locator("main select").first()).toHaveValue(target);
  await expect(page.getByRole("button", { name: "Bahasa Melayu" })).toHaveAttribute("aria-pressed", "true");
  // the shell and the screen read the saved language (a text without a Malay entry stays English)
  await expect(page.getByRole("button", { name: "Log keluar" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Keutamaan" })).toBeVisible();
  await expect(page.getByText("Bahasa paparan", { exact: true })).toBeVisible();
  await page.goto("/notifications"); // the inbox too (FR-X01: key screens and notifications)
  await expect(page.getByRole("tab", { name: /^Semua/ })).toBeVisible();
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Pemberitahuan");
  // put it back, whichever language the page shows now
  await setDisplay(page, ENGLISH, before);
  zoneBefore = null;
  await page.reload();
  await expect(page.locator("main select").first()).toHaveValue(before);
  await expect(page.getByRole("button", { name: ENGLISH })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
});

test("a client's consent and monthly report e-mail are saved", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/settings/preferences");
  for (const name of ["Location consent", "Monthly energy report e-mail"]) {
    const sw = () => page.getByRole("switch", { name });
    const before = await sw().getAttribute("aria-checked");
    await sw().click();
    await save(page);
    await page.reload();
    await expect(sw()).toHaveAttribute("aria-checked", before === "true" ? "false" : "true");
    await sw().click();
    await save(page);
    await page.reload();
    await expect(sw()).toHaveAttribute("aria-checked", before ?? "false");
  }
});

test("HQ's two-step verification is required, with nothing to turn on or off (DEC-85, IR510)", async ({ page, app }) => {
  test.skip(app.id !== "admin", "HQ only");
  await page.goto("/settings/preferences");
  const main = page.getByRole("main");
  await expect(main).toContainText("Required for HQ: every sign-in to the HQ app asks for the 6-digit code from your authenticator app.");
  await expect(main).toContainText("Required");
  await expect(main.getByRole("button", { name: /^Turn (on|off)$/ })).toHaveCount(0);
});

test("two-step verification turns on with the setup QR and off again", async ({ page, app }) => {
  test.skip(app.id === "admin", "HQ's is required at sign-in (IR510)");
  await page.goto("/settings/preferences");
  await expect(page.getByRole("button", { name: "Turn on" })).toBeVisible(); // the demo users start without it
  await page.getByRole("button", { name: "Turn on" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("img", { name: /QR code with the setup key/ })).toBeVisible();
  await expectAccessible(page, '[role="dialog"]'); // the open dialog (IR360)
  await expect(dialog).toContainText(/or enter key\s+[A-Z2-7]{4}( [A-Z2-7]{4})+/);
  const code = dialog.getByLabel("6-digit code", { exact: true });
  await code.fill("12345");
  await expect(dialog.getByRole("button", { name: /Verify & turn on/ })).toBeDisabled();
  await code.fill("123456");
  await dialog.getByRole("button", { name: /Verify & turn on/ }).click();
  await expect(dialog).toContainText("Save these 8 recovery codes");
  await dialog.getByRole("button", { name: /I saved my codes/ }).click();
  await expect(page.getByRole("main")).toContainText("8 recovery codes left");
  // and off
  await page.getByRole("button", { name: "Turn off" }).click();
  await page.getByRole("dialog").getByLabel("6-digit code", { exact: true }).fill("654321");
  await page.getByRole("dialog").getByRole("button", { name: "Turn off" }).click();
  await expect(page.getByRole("button", { name: "Turn on" })).toBeVisible();
});
