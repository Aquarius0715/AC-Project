// Customer automations (FR-C04, FR-C05, IR214, IR264): a new schedule previews its next runs — start and end in the
// rule's time zone (the end once read as "0 MYT", R257) — is saved switched off (saving sends no command). Switched on,
// the overview names its next run (automations.nextRuns, IR312); then it is deleted. A schedule left behind by a failed
// run is deleted in afterEach.
import { test, expect } from "../../fixtures/test";
import type { Page } from "@playwright/test";

let leftover: string | null = null;

async function remove(page: Page, name: string) {
  await page.goto("/customer/automations");
  const card = page.locator("section", { hasText: name });
  if (!(await card.count())) return;
  await card.getByRole("button", { name: `More for ${name}` }).click();
  await card.getByRole("button", { name: "🗑 Delete" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Delete automation" }).click();
  await expect(page.getByText("Automation deleted").first()).toBeVisible();
}

test.afterEach(async ({ page }) => {
  if (leftover) await remove(page, leftover);
  leftover = null;
});

test("a schedule previews its next runs, is saved off and deleted", async ({ page, app }) => {
  test.skip(app.id !== "customer", "client only");
  await page.goto("/customer/automations");
  await page.getByRole("button", { name: "+ Create automation" }).first().click();
  await page.waitForURL(/automationId=new/);
  const name = `E2E schedule ${Date.now()}`;
  await page.getByLabel("Name").fill(name);
  // the default schedule (weekdays 18:00 → 22:00) in the Preferences zone: each run with its end time
  const runs = page.getByText("Next runs", { exact: true }).locator("..");
  await expect(runs).toContainText(/\d{2}:\d{2} MYT · 22:00 MYT/);
  await expect(page.getByText("Off on save")).toBeVisible();
  await page.getByRole("button", { name: "Save automation" }).click();
  leftover = name;
  await expect(page.getByText("Automation saved").first()).toBeVisible();
  const card = page.locator("section", { hasText: name });
  await expect(card).toContainText("Weekdays at 18:00 (Asia/Kuala_Lumpur)");
  await expect(card.getByRole("switch")).toHaveAttribute("aria-checked", "false");
  // switched on (the next run is 18:00, so nothing is sent now), the overview shows when it runs next
  await card.getByRole("switch").click();
  await expect(card.getByRole("switch")).toHaveAttribute("aria-checked", "true");
  await page.goto("/customer");
  await expect(page.locator("section", { hasText: name })).toContainText(/Next run: .* · /);
  await remove(page, name);
  leftover = null;
  await expect(page.locator("section", { hasText: name })).toHaveCount(0);
});
