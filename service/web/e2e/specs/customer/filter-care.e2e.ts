// Customer filter care (FR-C18, DD-C18, IR238): the tab is in the URL, every AC has a row, the reminder settings refuse
// a run time below the minimum and are saved and put back, and Request cleaning opens New request prefilled (cancelled,
// so no job is created). Mark cleaned is left out: it moves the cleaning date for good.
import { test, expect } from "../../fixtures/test";

test("the Filter care tab lists every AC and goes back to My requests", async ({ page }) => {
  await page.goto("/customer/maintenance");
  await page.getByRole("tab", { name: "Filter care" }).click();
  await page.waitForURL(/tab=filter-care/);
  await expect(page.getByText("Filters by AC")).toBeVisible();
  await expect(page.locator("main tr", { hasText: " AC" }).first()).toBeVisible();
  await page.getByRole("tab", { name: "My requests" }).click();
  await page.waitForURL((u) => !u.searchParams.has("tab"));
  await expect(page.getByRole("tab", { name: "My requests" })).toHaveAttribute("aria-selected", "true");
});

test("reminder settings refuse too few hours, then are saved and put back", async ({ page }) => {
  await page.goto("/customer/maintenance?tab=filter-care");
  const dialog = page.getByRole("dialog");
  // the reminder lines once the settings card has rendered (a read right after navigation can come before it)
  const reminders = async () => {
    await expect(page.getByRole("button", { name: "Edit reminders" })).toBeVisible();
    return (await page.getByRole("main").innerText()).match(/Reminders[\s\S]{0,200}/)?.[0] ?? "";
  };
  // a save is finished when its Server Action has answered: a toast of the previous save may still be on screen
  const save = async () => {
    const answered = page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === "/customer/maintenance");
    await dialog.getByRole("button", { name: "Save reminders" }).click();
    await answered;
    await expect(dialog).toBeHidden();
  };
  const before = await reminders();
  expect(before).not.toBe("");
  await page.getByRole("button", { name: "Edit reminders" }).click();
  await dialog.getByRole("button", { name: "Custom" }).click();
  await dialog.getByLabel("Hours of running").fill("20");
  await dialog.getByRole("button", { name: "Save reminders" }).click();
  await expect(dialog).toContainText("✕");
  await dialog.getByLabel("Hours of running").fill("120");
  await dialog.getByRole("button", { name: "All users of the location" }).click();
  await save();
  await expect(page.getByText("Reminder settings saved.").first()).toBeVisible();
  await expect(page.getByRole("main")).toContainText("120");
  // put the defaults back: model default hours, 30 days, owners only, no e-mail
  await page.getByRole("button", { name: "Edit reminders" }).click();
  await dialog.getByRole("button", { name: /Model default/ }).click();
  await dialog.getByRole("textbox").first().fill("30");
  await dialog.getByRole("button", { name: "Owners of the location" }).click();
  if (await dialog.getByLabel("E-mail").isChecked()) await dialog.getByLabel("E-mail").uncheck();
  await save(); // R293: waiting only for a "saved" toast let the reload read before this save had committed
  await page.reload();
  expect(await reminders()).toBe(before);
});

test("Request cleaning opens New request for that AC, prefilled", async ({ page }) => {
  await page.goto("/customer/maintenance?tab=filter-care");
  const request = page.locator("main tr").getByRole("button", { name: "Request cleaning" });
  test.skip((await request.count()) === 0, "no AC is overdue for cleaning");
  const row = page.locator("main tr", { has: request.first() });
  const unit = ((await row.locator("td").first().innerText()).split("\n")[0] ?? "").trim();
  await request.first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  expect(await dialog.locator("select").first().evaluate((s: HTMLSelectElement) => s.options[s.selectedIndex].text)).toContain(unit);
  await expect(dialog.getByRole("button", { name: "Preventive" })).toHaveAttribute("aria-pressed", "true");
  await expect(dialog.locator("textarea")).not.toHaveValue("");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});
