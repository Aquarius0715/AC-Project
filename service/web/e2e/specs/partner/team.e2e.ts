// Team & capacity (FR-P06, DD-P06, Figma Contractor 04-1…04-5, 04-8, IR276): the company's technicians with the
// chosen day, the team's week and the qualification grants; a Saturday has no available hours; another company's
// orgId reads as not found (AT-P06-E①); the unavailable days form checks its days before saving. Nothing is saved:
// members.setUnavailability has no undo.
import { test, expect } from "../../fixtures/test";

test("the team shows the technicians, the week and the qualifications", async ({ page }) => {
  await page.goto("/partner/team");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^Technicians — .+ · \d+ active$/ })).toBeVisible();
  await expect(main.getByRole("heading", { name: /^Week of \d{1,2} \S+ — assigned \/ available hours$/ })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Qualifications", exact: true })).toBeVisible();
  await expect(main.getByRole("combobox", { name: "Qualification" })).toContainText("All qualifications");
  await expect(main.getByRole("button", { name: "✓ Active only" })).toHaveAttribute("aria-pressed", "true");
  await main.getByRole("button", { name: "✓ Active only" }).click();
  await page.waitForURL(/activeOnly=false/);
  await expect(main.getByRole("button", { name: "Include expired" })).toHaveAttribute("aria-pressed", "false");
});

test("a Saturday has no available hours", async ({ page }) => {
  await page.goto("/partner/team?date=2026-09-19");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^Week of 14 Sept — hours \(Sat 19 selected\)$/ })).toBeVisible();
  await expect(main.getByText("No available hours set").first()).toBeVisible();
});

test("another company's roster reads as not found", async ({ page }) => {
  await page.goto("/partner/team?orgId=00000000-0000-0000-0000-000000000000");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "Roster not found" })).toBeVisible();
  await expect(main).not.toContainText("tech-external");
  await main.getByRole("link", { name: "Back to my team" }).click();
  await page.waitForURL((u) => u.pathname === "/partner/team" && !u.searchParams.has("orgId"));
  await expect(main.getByRole("heading", { name: /^Technicians — / })).toBeVisible();
});

test("the unavailable days form checks its days before saving", async ({ page }) => {
  await page.goto("/partner/team?date=2026-09-16");
  await page.getByRole("button", { name: "+ Unavailable days" }).click();
  const dialog = page.getByRole("dialog", { name: "Add unavailable days" });
  const to = dialog.getByLabel(/^To/); // the label also holds the field's error once shown
  await expect(dialog.getByLabel(/^From/)).toHaveValue("2026-09-16");
  await to.fill("2026-09-15");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog).toContainText("The last day must be on or after the first day");
  await to.fill("2026-10-20");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog).toContainText("At most 31 days at a time");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
});
