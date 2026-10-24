// HQ contractor register (FR-A21, DD-A21, IR465): Add contractor creates a new contractor company and its profile in one
// dialog — no organization has to exist first — and opens it; a second company with the same name is refused and
// nothing else is created. The company stays (organizations are not deleted), under a name of its own per run.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("HQ adds a new contractor company with its profile; the same name again is refused", async ({ page }) => {
  await page.goto("/admin/jobs?tab=contractors");
  const main = page.getByRole("main");
  const company = `E2E Aircon ${Date.now()}`;
  const add = async () => {
    await main.getByRole("button", { name: "Add contractor" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("combobox", { name: /^Contractor organization/ }).selectOption({ label: "New company…" });
    await dialog.getByRole("textbox", { name: /^Company name/ }).fill(company);
    await dialog.getByRole("textbox", { name: /^Registration number/ }).fill("SSM 202601234567");
    await dialog.getByRole("textbox", { name: /^Service areas/ }).fill("Kuala Lumpur, Selangor");
    await dialog.getByRole("textbox", { name: /^Contact e-mail/ }).fill("ops@aircon.example");
    return dialog;
  };
  const dialog = await add();
  await expectAccessible(page, '[role="dialog"]'); // the dialog with the new company's name (IR360)
  await dialog.getByRole("button", { name: "Add contractor" }).click();
  await expect(page.getByText("Contractor profile created").first()).toBeVisible();
  await page.waitForURL(/contractorId=[0-9a-f-]{36}/);
  await expect(main.getByRole("heading", { name: company })).toBeVisible();

  // the same name again: refused, and no second company
  const again = await add();
  await again.getByRole("button", { name: "Add contractor" }).click();
  await expect(main.getByText("a contractor company with this name exists already")).toBeVisible();
  await expect(main.getByRole("link", { name: new RegExp(company) })).toHaveCount(1);
});
