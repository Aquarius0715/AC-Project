// Force delete (data.forceDelete, IR690): the seed's HQ operator holds the permission, so the customer's delete dialog
// offers "Force delete"; it needs the customer's name typed before Delete can be pressed. A customer made here is
// deleted with it, so nothing stays behind.
import { test, expect } from "../../fixtures/test";

const NAME = "E2E Force Delete Ltd";

test("the delete dialog offers a forced delete that needs the name typed, and deletes a customer made here", async ({ page }) => {
  const main = page.getByRole("main");
  await page.goto("/admin/units");
  await main.getByRole("button", { name: "New customer" }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("Billing name · required").fill(NAME);
  await dialog.getByLabel("Customer display name · required").fill(NAME);
  await dialog.getByRole("button", { name: "Create customer" }).click();
  await expect(dialog).toHaveCount(0);

  await main.getByRole("row", { name: new RegExp(NAME) }).click();
  await page.waitForURL(/customerId=/);
  await main.getByRole("button", { name: "Delete customer…" }).click();
  dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: "Reason" }).fill("Made by the force-delete spec");
  await dialog.getByRole("checkbox", { name: /Force delete/ }).click();
  const confirm = dialog.getByRole("button", { name: "Delete", exact: true });
  await expect(confirm).toBeDisabled();
  await dialog.getByRole("textbox", { name: `Type ${NAME} to confirm` }).fill(NAME);
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await page.waitForURL((url) => !url.searchParams.has("customerId"));
  await expect(main.getByRole("row", { name: new RegExp(NAME) })).toHaveCount(0);
});
