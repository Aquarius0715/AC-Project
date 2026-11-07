// Certifications (FR-P09, IR593): a certificate submitted with nothing filled in takes focus to the first field to
// fix, and the file picker — a hidden input inside its drop box — shows a focus outline when reached by keyboard.
// Nothing is submitted.
import { test, expect } from "../../fixtures/test";

test("an empty certificate takes focus to the first field to fix and the file picker shows its focus", async ({ page }) => {
  await page.goto("/partner/team?tab=certifications");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "Add certificate" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Submit for verification" }).click();
  await expect(dialog).toContainText("Choose the issue date");
  await expect(dialog.getByLabel(/^Issued on/)).toBeFocused();
  await dialog.getByLabel(/^Certificate number/).focus();
  await page.keyboard.press("Tab");
  const picker = await page.evaluate(() => {
    const a = document.activeElement;
    const label = a?.closest("label"); // this runs in the page: no test helpers here
    return { type: a instanceof HTMLInputElement ? a.type : undefined, outline: label ? getComputedStyle(label).outlineStyle : "no label" };
  });
  expect(picker).toEqual({ type: "file", outline: "solid" });
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});
