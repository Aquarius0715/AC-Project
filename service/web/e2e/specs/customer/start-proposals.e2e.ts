import { test, expect } from "../../fixtures/test";
import { pickTime } from "../../fixtures/time";

test("reviews and approves a weekday suggestion from database history", async ({ page }, testInfo) => {
  test.skip(process.env.E2E_START_PROPOSALS !== "1", "Requires the start_proposals.sql fixture on a fresh seed");
  await page.goto("/customer/automations");
  const suggestions = page.getByRole("region", { name: "Schedule suggestions" });
  const wednesday = suggestions.getByRole("heading", { name: /Proposal fixture Wednesday/ }).locator("../..");
  const friday = suggestions.getByRole("heading", { name: /Proposal fixture Friday/ }).locator("../..");
  await expect(wednesday).toContainText("Wednesday 18:30 → 20:00");
  await expect(wednesday).toContainText("3 of the last 4 occurrences of Wednesday");
  await expect(wednesday).toContainText("2026-08-26, 2026-09-02, 2026-09-09");
  await expect(friday).toContainText("Friday 18:30 → 21:15");
  await expect(friday).toContainText("3 of the last 4 occurrences of Friday");
  await expect(friday).toContainText("2026-08-28, 2026-09-04, 2026-09-11");
  await expect(suggestions.getByRole("heading", { name: /Proposal fixture unconfirmed/ })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("schedule-suggestions.png"), fullPage: true });
  await wednesday.getByRole("button", { name: "Approve schedule change" }).click();
  await expect(page.getByText("Schedule change approved")).toBeVisible();
  await expect(suggestions.getByRole("heading", { name: /Proposal fixture Wednesday/ })).toHaveCount(0);

  await page.reload();
  const rules = page.getByRole("button", { name: "More for Proposal fixture Wednesday" });
  await expect(rules).toHaveCount(2);
  const cards = page.locator('[data-slot="card"]', { has: rules });
  const approved = cards.filter({ hasText: "20:00" });
  await expect(approved).toContainText("Wed at 20:00");
  await expect(approved).toContainText("23:30");
  await expect(approved).toContainText("24°C");
  const remaining = cards.filter({ hasText: "18:30" });
  await expect(remaining).toContainText("Mon, Thu");
  await expect(remaining).toContainText("23:30");
  await expect(remaining).toContainText("24°C");

  await using editPage = await page.context().newPage();
  await editPage.goto("/customer/automations");
  const fridayRule = editPage.locator('[data-slot="card"]', {
    has: editPage.getByRole("button", { name: "More for Proposal fixture Friday" }),
  });
  await fridayRule.getByRole("button", { name: "Edit", exact: true }).click();
  await pickTime(editPage.getByRole("group", { name: "Start", exact: true }), "19:00");
  await editPage.getByRole("button", { name: "Save automation" }).click();
  await expect(editPage.getByText("Automation saved").first()).toBeVisible();

  await friday.getByRole("button", { name: "Approve schedule change" }).click();
  await expect(page.getByText(/the screen now shows the latest version/)).toBeVisible();
  await expect(friday).toContainText("Friday 19:00 → 21:15");
});
