// A new aircon installation, HQ's side (FR-A02, FR-A06, FR-T16; DEC-106, IR789): HQ registers a unit as "Installation
// planned" (no installation date, no warranty end), and the New job dialog then offers that unit an installation job
// and nothing else; an installed unit is never offered one. The job is cancelled and the unit deleted (archived) at the
// end, so a run leaves one archived unit. The technician's part needs the visit window, which only a jump of the
// never-returning demo clock would reach, so it is covered by the API's integration test and the Vitest cases.
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

const CUSTOMER_A = "d67d153b-fa85-57be-aa40-c8aa09e3d02c"; // the customer record of Demo Customer A
const MEETING_ROOM = "971554c4-e36b-5d0b-a8fe-c61445c649c5";

const unitSelect = (page: Page) => page.getByRole("dialog").getByRole("combobox", { name: /Unit/ }).first();

test("HQ plans a unit and creates its installation job", async ({ page }) => {
  test.setTimeout(180_000);
  const main = page.getByRole("main");
  const name = `E2E planned AC ${String(Date.now())}`;
  let jobId = "";

  try {
    await test.step("HQ registers the unit as installation planned", async () => {
      await page.goto(`/admin/units?customerId=${CUSTOMER_A}&locationId=${MEETING_ROOM}`);
      await main.getByRole("button", { name: /^New unit/ }).first().click();
      const dialog = page.getByRole("dialog");
      await dialog.getByLabel(/Display name/).fill(name);
      const model = dialog.getByLabel(/Aircon model/);
      await model.selectOption({ index: 0 });
      await dialog.getByRole("button", { name: "Installation planned" }).click();
      await expect(dialog.getByLabel(/Installed at/)).toBeDisabled();
      await expect(dialog.getByLabel(/Warranty end/)).toBeDisabled();
      await expectAccessible(page, '[role="dialog"]');
      await dialog.getByRole("button", { name: "Register unit" }).click();
      await expect(page.getByText(`Unit “${name}” registered`).first()).toBeVisible();
      await expect(main).toContainText("Installation planned: the installation date is set when HQ accepts");
      await expect(main.getByLabel("Installed at")).toBeDisabled();
    });

    await test.step("the New job dialog offers the planned unit an installation job and nothing else", async () => {
      await page.goto("/admin/jobs");
      await main.getByRole("button", { name: /^New job/ }).first().click();
      const dialog = page.getByRole("dialog");
      const units = unitSelect(page);
      const option = await units.locator("option", { hasText: name }).first().innerText();
      await units.selectOption({ label: option.trim() });
      await expect(dialog).toContainText("Installation");
      await expect(dialog.getByRole("button", { name: "Repair" })).toHaveCount(0);
      await dialog.getByLabel(/Symptom/).fill("E2E: install a new split unit in the meeting room.");
      await dialog.getByRole("button", { name: "Save as requested" }).click();
      await page.waitForURL(/jobId=/);
      jobId = new URL(page.url()).searchParams.get("jobId") ?? "";
      expect(jobId).toMatch(/^[0-9a-f-]{36}$/);
      await expect(main).toContainText("Installation");
    });

    await test.step("an installed unit is not offered one", async () => {
      await page.goto("/admin/jobs");
      await main.getByRole("button", { name: /^New job/ }).first().click();
      const dialog = page.getByRole("dialog");
      await unitSelect(page).selectOption({ index: 0 });
      await expect(dialog.getByRole("button", { name: "Repair" })).toBeVisible();
      await expect(dialog.getByRole("button", { name: "Installation", exact: true })).toHaveCount(0);
      await dialog.getByRole("button", { name: "Cancel" }).click();
    });
  } finally {
    if (jobId) {
      await page.goto(`/admin/jobs?jobId=${jobId}`);
      await main.getByRole("button", { name: "Cancel…" }).click();
      await page.getByLabel(/Reason · required/).fill("E2E installation finished.");
      await page.getByRole("button", { name: "Confirm" }).click();
      await expect(page.getByText("Job cancelled.").first()).toBeVisible();
    }
    await page.goto(`/admin/units?customerId=${CUSTOMER_A}&locationId=${MEETING_ROOM}`);
    const row = main.getByText(name).first();
    if (await row.isVisible()) {
      await row.click();
      await main.getByRole("button", { name: "Delete unit…" }).click();
      await page.getByRole("dialog").getByLabel(/Reason · required/).fill("E2E installation finished.");
      await page.getByRole("dialog").getByRole("button", { name: "Delete unit" }).click();
    }
  }
});
