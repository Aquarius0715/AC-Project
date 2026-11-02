// Energy & cost (FR-C06, FR-C16, IR587): without a baseline for the chosen units the page says actual energy stays
// viewable, in words; the monthly report export says that the demo file is neither downloaded nor e-mailed. The export
// reads only (energy.exportReport); the monthly e-mail choice is left as it is.
import { test, expect } from "../../fixtures/test";

test("the energy page and its demo report say what they do, in words", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/customer/energy");
  const main = page.getByRole("main");
  await expect(main.getByText("Actual energy", { exact: true })).toBeVisible();
  const note = main.getByText(/^No baseline covers exactly these units/);
  if (await note.count()) await expect(note).toHaveText("No baseline covers exactly these units — actual energy stays viewable.");

  await main.getByRole("button", { name: "Export" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("combobox", { name: /^Month/ })).toBeVisible();
  await dialog.getByRole("button", { name: "Download" }).click();
  await expect(dialog).toContainText(/\.pdf · \d+ KB · PDF — a demo file: nothing is downloaded or e-mailed\./);
  await dialog.getByRole("button", { name: "Close" }).last().click();
  await expect(dialog).toHaveCount(0);
});

// A demo quote (FR-C13, IR596) has no price — no offset provider is selected — and says so instead of a bare dash. The
// quote expires by itself after 15 minutes; no offset record is requested.
test("a demo offset quote says why it has no price", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/customer/energy");
  const main = page.getByRole("main");
  await main.getByRole("link", { name: /Offset these emissions/ }).click();
  await page.waitForURL(/\/customer\/energy\/offsets/);
  await main.getByRole("button", { name: "Get demo quote" }).click();
  await expect(main).toContainText("No price — no offset provider is selected in this demo");
});
