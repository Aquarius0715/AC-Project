// Technician unit QR scan (FR-T13, DD-T13, IR230, IR254): an unknown code is refused without unit data, and the one-tap
// labels are the units of the technician's open jobs — each matches and opens the unit. The seed's open job is
// tech-external-a's (job-contractor-a, its work window 14–20 Sept), so the spec signs in as that technician: on a
// fresh seed tech-internal-a has none, and the label part would be skipped (IR316).
import type { BrowserContext } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

let context: BrowserContext | null = null;
test.afterEach(async () => {
  await context?.close();
  context = null;
});

test("an unknown label is refused and a unit's label matches", async ({ browser, app }) => {
  context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } }); // not the project's tech-internal-a session
  const page = await context.newPage();
  await signIn(page, app, "tech-external-a");
  await page.goto("/technician");
  await page.getByRole("button", { name: /Scan QR/ }).click();
  const dialog = page.getByRole("dialog", { name: "Scan unit QR" });
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("Label code, device serial or unit ID").fill("ac-unit:00000000-0000-4000-8000-000000000000");
  await dialog.getByRole("button", { name: "Scan" }).click();
  await expect(dialog.getByRole("status")).toBeVisible();
  await expect(dialog.getByRole("link", { name: "Open unit" })).toHaveCount(0);
  const labels = dialog.getByText("Labels on the units of your open jobs:").getByRole("button"); // the one-tap labels only
  test.skip((await labels.count()) === 0, "no open job for this technician now (the seed's work window has passed)");
  await labels.first().click();
  await expect(dialog.getByText("✓ Matched")).toBeVisible();
  await dialog.getByRole("link", { name: "Open unit" }).click();
  await page.waitForURL(/\/technician\/units\/[0-9a-f-]{36}/);
  await expect(page.getByRole("main")).not.toContainText("This page isn’t available");
});
