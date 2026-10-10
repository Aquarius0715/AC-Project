// FR-C03, DD-C03, IR315: ✎ Rename on the unit screen renames the AC (locations.rename, kind=unit) — Figma Client 02e.
// The new name shows on the screen at once; the spec then gives the AC its seed name back, also in afterEach when the
// test fails half-way.
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";

const SEED = "Bedroom AC", TEMP = "Bedroom AC E2E";
let href: string | null = null;

async function rename(page: Page, from: string, to: string) {
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "✎ Rename" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: `Rename ${from}` })).toBeVisible();
  await dialog.getByRole("textbox").fill(to); // the dialog's one field (its name includes the hint)
  const saved = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === new URL(page.url()).pathname);
  await dialog.getByRole("button", { name: "Save name" }).click();
  await saved;
  await expect(dialog).toBeHidden();
  await expect(main.getByRole("heading", { name: to, exact: true })).toBeVisible(); // the screen renders again
}

test.afterEach(async ({ page }) => {
  if (!href) return;
  await page.goto(href);
  if (await page.getByRole("main").getByRole("heading", { name: TEMP, exact: true }).isVisible()) await rename(page, TEMP, SEED);
});

test("the unit screen renames its AC and gives it back its name", async ({ page }) => {
  await page.goto("/customer");
  const links = page.getByRole("main").locator('a[href^="/customer/units/"]');
  await links.filter({ hasText: SEED }).first().waitFor(); // the overview's unit table streams in
  href = await links.evaluateAll((as) => as.find((a) => (a.textContent ?? "").includes("Bedroom AC"))?.getAttribute("href") ?? null);
  expect(href).toBeTruthy();
  await page.goto(href!);
  await rename(page, SEED, TEMP);
  await expect(page.getByRole("main")).toContainText(`Home A › 1F › Bedroom › ${TEMP}`); // the breadcrumb too
  await rename(page, TEMP, SEED);
});
