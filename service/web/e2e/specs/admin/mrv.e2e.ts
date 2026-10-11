// HQ MRV (FR-A14, DD-A14 item 6, Figma Admin 324:2, IR349): a new report's "Evidence to attach" offers the photos of
// accepted work reports on its units uploaded in its period (attachments.list), once the units and the period are
// chosen. The seed has no work-report photos, so the list says so. Nothing is saved: the dialog is cancelled.
import { test, expect } from "../../fixtures/test";

test("a new report's evidence comes from the photos of accepted work reports", async ({ page }) => {
  await page.goto("/admin/mrv");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "+ New report" }).click();
  const dialog = page.getByRole("dialog", { name: "New Scope 2 report" });
  const evidence = dialog.getByRole("group", { name: "Evidence to attach" });
  await expect(evidence).toContainText("None yet.");
  await expect(evidence.getByRole("button", { name: "+ Add evidence" })).toBeDisabled();
  await expect(evidence).toContainText("Choose the units and the period first");

  await dialog.getByLabel("Customer organization").selectOption({ label: "Demo Customer A" });
  await dialog.getByRole("checkbox").first().check();
  await dialog.getByLabel("Start (Kuala Lumpur)").fill("2026-09-01T00:00");
  await dialog.getByLabel("End", { exact: true }).fill("2026-09-14T00:00");
  await evidence.getByRole("button", { name: "+ Add evidence" }).click();
  await expect(evidence).toContainText("No photos of accepted work reports on these units in this period.");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toBeHidden();
});

// The factor editor (Figma Admin 324:321, IR350): an edit marks the factor "Editing → v<next>" and enables Save as
// v<next>; Discard puts the saved values back. Nothing is saved.
test("the factor editor marks an edit and discards it", async ({ page }) => {
  await page.goto("/admin/mrv?tab=factors");
  const main = page.getByRole("main");
  const save = main.getByRole("button", { name: /^Save as v\d+$/ });
  await expect(save).toBeDisabled();
  await expect(main).toContainText("No unsaved changes");
  const value = main.getByRole("spinbutton", { name: /^kgCO₂e\/kWh/ });
  const before = await value.inputValue();
  await value.fill("0.999");
  await expect(main.getByRole("heading", { name: /Editing → v\d+/ })).toBeVisible();
  await expect(main).toContainText("Unsaved changes");
  await expect(save).toBeEnabled();
  await main.getByRole("button", { name: "Discard" }).click();
  await expect(value).toHaveValue(before);
  await expect(save).toBeDisabled();
});
