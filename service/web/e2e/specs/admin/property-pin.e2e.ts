// The property pin (DEC-109, IR824): HQ sets Home A's position and away/home radius in Edit details, the property's
// page shows them, and a coordinate off the globe keeps the dialog open with the field's message. The coordinates are
// typed, the way that works with or without a map in the environment. afterEach removes the pin again, also after a
// timeout.
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

const CUSTOMER_A = "d67d153b-fa85-57be-aa40-c8aa09e3d02c"; // the customer record of Demo Customer A
const HOME_A = "cd425e38-655c-5a52-b51d-cf12ef43d0bf";
const PAGE = `/admin/units?customerId=${CUSTOMER_A}&locationId=${HOME_A}`;

/** Opens Edit details of Home A, lets `fill` change the dialog, saves and waits for the Server Action's answer. */
async function editHomeA(page: Page, fill: (dialog: Locator) => Promise<void>) {
  await page.goto(PAGE);
  await page.getByRole("main").getByRole("button", { name: "Edit details" }).click();
  const dialog = page.getByRole("dialog");
  await fill(dialog);
  const saved = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === "/admin/units");
  await dialog.getByRole("button", { name: "Save property" }).click();
  await saved;
  await expect(dialog).toHaveCount(0);
}

test.afterEach(async ({ page }) => {
  await editHomeA(page, async (dialog) => {
    const remove = dialog.getByRole("button", { name: "Remove pin" });
    if (await remove.count()) await remove.click();
    await dialog.getByLabel("Away/home radius (m)").fill("");
  });
  await expect(page.getByRole("main")).toContainText("No pin yet");
});

test("HQ pins Home A on the map with a radius, and the property's page shows them", async ({ page }) => {
  await editHomeA(page, async (dialog) => {
    await expect(dialog.getByText("Location on the map")).toBeVisible();
    await expectAccessible(page, '[role="dialog"]');
    await dialog.getByLabel("Latitude", { exact: true }).fill("3.1579");
    await dialog.getByLabel("Longitude", { exact: true }).fill("101.7116");
    await dialog.getByLabel("Away/home radius (m)").fill("200");
  });
  const main = page.getByRole("main");
  await expect(main).toContainText("3.157900, 101.711600");
  await expect(main).toContainText("200 m");
});

test("a coordinate off the globe or a radius out of range keeps the dialog open with the field's message", async ({ page }) => {
  await page.goto(PAGE);
  await page.getByRole("main").getByRole("button", { name: "Edit details" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Latitude", { exact: true }).fill("95");
  await dialog.getByLabel("Longitude", { exact: true }).fill("101.7");
  await dialog.getByLabel("Away/home radius (m)").fill("10");
  await dialog.getByRole("button", { name: "Save property" }).click();
  await expect(dialog).toContainText("Latitude −90 to 90 and longitude −180 to 180, in degrees");
  await expect(dialog).toContainText("A whole number of metres, 50–2000");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});
