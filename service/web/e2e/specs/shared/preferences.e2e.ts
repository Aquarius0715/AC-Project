// Preferences on the Core API (FR-X01, FR-X08, IR246, IR247): the time zone and language are saved and read back, a
// client's location consent and monthly report e-mail too, and two-step verification turns on (setup QR, six code
// boxes, recovery codes once) and off. Every change is put back.
import { test, expect } from "../../fixtures/test";
import type { Page } from "@playwright/test";

/** Saves and waits for the Server Action's answer and its success toast (Save is also disabled while pending, so a
 * disabled button alone does not mean saved). */
async function save(page: Page) {
  const answered = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === "/settings/preferences");
  await page.getByRole("button", { name: /Save preferences/ }).click();
  await answered;
  await expect(page.getByText("Preferences saved").last()).toBeVisible();
}

test("language and time zone are saved and read back", async ({ page }) => {
  await page.goto("/settings/preferences");
  const zone = page.locator("main select").first();
  const before = await zone.inputValue();
  const target = before === "Asia/Tokyo" ? "Asia/Singapore" : "Asia/Tokyo";
  await zone.selectOption(target);
  await page.getByRole("button", { name: "Bahasa Melayu" }).click();
  await save(page);
  await page.reload();
  await expect(page.locator("main select").first()).toHaveValue(target);
  await expect(page.getByRole("button", { name: "Bahasa Melayu" })).toHaveAttribute("aria-pressed", "true");
  // put it back
  await page.locator("main select").first().selectOption(before);
  await page.getByRole("button", { name: /English/ }).click();
  await save(page);
  await page.reload();
  await expect(page.locator("main select").first()).toHaveValue(before);
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

test("two-step verification turns on with the setup QR and off again", async ({ page }) => {
  await page.goto("/settings/preferences");
  await expect(page.getByRole("button", { name: "Turn on" })).toBeVisible(); // the demo users start without it
  await page.getByRole("button", { name: "Turn on" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("img", { name: /QR code with the setup key/ })).toBeVisible();
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
