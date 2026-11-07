// Team & capacity (FR-P06, FR-P09, DD-P06, DD-P09, Figma Contractor 04-1…04-8, IR276, IR277): the company's
// technicians with the chosen day, the team's week and the qualification grants; a Saturday has no available hours;
// another company's orgId reads as not found (AT-P06-E①); the unavailable days form checks its days before saving;
// unavailable days are listed and removed again (IR612); the Certifications tab shows the KPIs, one row per technician
// and qualification and the certificate form's checks. Unavailable days added here are removed by the same test;
// certificates are never sent (they have no undo).
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("the team shows the technicians, the week and the qualifications", async ({ page }) => {
  await page.goto("/partner/team?date=2026-09-16"); // a weekday: on a weekend of the demo clock the week reads "hours (Sat … selected)"
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /^Technicians — .+ · \d+ active$/ })).toBeVisible();
  await expect(main.getByRole("heading", { name: /^Week of \d{1,2} \S+ — assigned \/ available hours$/ })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Qualifications", exact: true })).toBeVisible();
  await expect(main.getByRole("combobox", { name: "Qualification" })).toContainText("All qualifications");
  await expect(main.getByRole("button", { name: "Active only" })).toHaveAttribute("aria-pressed", "true");
  // accounts are the contractor's own, in Users (FR-P11); qualifications come from HQ; no API names (IR579)
  await expect(main).toContainText("Accounts are managed in Users.");
  await expect(main).not.toContainText(/requested from HQ|registered by HQ|members\.\w+/);
  await main.getByRole("button", { name: "Active only" }).click();
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
  await page.getByRole("button", { name: "Unavailable days", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add unavailable days" });
  await expectAccessible(page, '[role="dialog"]'); // the open dialog (IR360)
  const to = dialog.getByLabel(/^To/); // the label also holds the field's error once shown
  await expect(dialog.getByLabel(/^From/)).toHaveValue("2026-09-16");
  await expect(dialog).toContainText("the lists of who can take a visit skip them"); // no API name (IR579)
  await to.fill("2026-09-15");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog).toContainText("The last day must be on or after the first day");
  await to.fill("2026-10-20");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog).toContainText("At most 31 days at a time");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
});

// IR612: three days from today are added, listed, shortened to the first one and removed; the dialog says what a removal
// does, refuses a day outside the row and hands focus back to the list once the row is gone.
test("unavailable days are listed, shortened and removed", async ({ page }) => {
  await page.goto("/partner/team");
  const main = page.getByRole("main");
  await page.getByRole("button", { name: "Unavailable days", exact: true }).click();
  const add = page.getByRole("dialog", { name: "Add unavailable days" });
  await add.getByRole("combobox").first().selectOption({ label: "tech-external-a" });
  const from = await add.getByLabel(/^From/).inputValue(); // today in Kuala Lumpur
  const day = (n: number) => new Date(Date.parse(`${from}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);
  await add.getByLabel(/^To/).fill(day(2));
  await add.getByLabel(/^Note/).fill("E2E remove-days");
  await add.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText(/^Unavailable days saved/).first()).toBeVisible();
  const row = main.getByRole("listitem").filter({ hasText: "E2E remove-days" });
  await expect(row).toContainText("tech-external-a");
  await expectAccessible(page);

  await row.getByRole("button", { name: /^Remove unavailable days of tech-external-a, / }).click();
  const dialog = page.getByRole("dialog", { name: "Remove unavailable days" });
  const endDateInput = dialog.getByLabel(/^Remove from/);
  await expect(endDateInput).toHaveValue(from);
  await expect(dialog).toContainText("All of these days are removed.");
  await expectAccessible(page, '[role="dialog"]');
  await endDateInput.fill(day(5));
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(dialog).toContainText(/^.*Choose a day from .+ to .+/);
  await expect(dialog).not.toContainText("Still unavailable");
  await endDateInput.fill(day(1));
  await expect(dialog).toContainText(/Still unavailable: .+; the days from .+ on are removed\./);
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(page.getByText(/^Unavailable days shortened — tech-external-a can take visits again from /).first()).toBeVisible();
  await expect(dialog).toBeHidden();

  await row.getByRole("button", { name: /^Remove unavailable days of tech-external-a, / }).click();
  await expect(endDateInput).toHaveValue(from);
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(page.getByText("Unavailable days removed — tech-external-a can take visits on those days again.").first()).toBeVisible();
  await expect(row).toHaveCount(0);
  await expect(page.locator(":focus")).toHaveAttribute("id", "unavailable-days");
});

test("the Certifications tab counts the certificates and checks an upload before sending it", async ({ page }) => {
  await page.goto("/partner/team?tab=certifications");
  const main = page.getByRole("main");
  await expect(page.getByRole("tab", { name: "Certifications" })).toHaveAttribute("aria-selected", "true");
  for (const name of ["Valid", /^Expiring ≤ 60 days$/, "Expired", "Pending HQ verification"]) await expect(main.getByRole("region", { name })).toBeVisible(); // the KPIs
  await expect(main.getByRole("heading", { name: /^Certificates — / })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Assignment impact" })).toBeVisible();
  await main.getByRole("combobox", { name: "Expiring within" }).selectOption("30");
  await page.waitForURL(/expiringWithinDays=30/);
  await expect(main.getByRole("region", { name: /^Expiring ≤ 30 days$/ })).toBeVisible();
  await main.getByRole("button", { name: "Add certificate" }).click();
  const dialog = page.getByRole("dialog", { name: "Add certificate" });
  await expectAccessible(page, '[role="dialog"]'); // the open dialog (IR360)
  await dialog.getByRole("button", { name: "Submit for verification" }).click();
  await expect(dialog).toContainText("Attach the certificate file");
  await dialog.getByLabel(/^Issued on/).fill("2026-09-28");
  await dialog.getByLabel(/^Expires on/).fill("2026-09-01");
  await dialog.getByRole("button", { name: "Submit for verification" }).click();
  await expect(dialog).toContainText("The expiry date must be after the issue date");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
});
