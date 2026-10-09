// Demo controls on the Core API (FR-X05, IR249): always labelled demo, the scenario clock, the browser-only parts
// disabled with their reason, a device fault and its recovery when the role has a device it can manage, and Expire
// session. The clock is moved only with E2E_ADVANCE_CLOCK=1, because the demo clock never goes back.
import { test, expect } from "../../fixtures/test";

test("the panel is labelled demo and shows the Core API clock", async ({ page }) => {
  await page.goto("/demo");
  await expect(page.getByRole("banner")).toContainText("Always labelled demo");
  await expect(page.getByRole("main")).toContainText(/\d{4}-\d\d-\d\d \d\d:\d\d UTC/);
  await expect(page.getByRole("button", { name: "Reset demo data" })).toBeDisabled();
  await expect(page.getByRole("button", { name: /Delay transport/ })).toBeDisabled();
});

test("the clock moves forward by a minute", async ({ page }) => {
  test.skip(!process.env.E2E_ADVANCE_CLOCK, "moves the shared demo clock for good; set E2E_ADVANCE_CLOCK=1");
  await page.goto("/demo");
  const read = async () => new Date(((await page.getByRole("main").innerText()).match(/(\d{4}-\d\d-\d\d) (\d\d:\d\d) UTC/) ?? []).slice(1).join("T") + ":00Z").getTime();
  const before = await read();
  await page.getByRole("button", { name: "+1 min" }).click();
  await expect(page.getByText("Clock advanced 1 minute")).toBeVisible();
  expect(await read()).toBeGreaterThanOrEqual(before + 60_000);
});

test("a device loses its connection and gets it back", async ({ page }) => {
  await page.goto("/demo");
  const select = page.getByLabel("Device");
  test.skip((await select.count()) === 0, "no device this role can manage now");
  const options = await select.locator("option").allInnerTexts();
  const online = options.findIndex((o) => / · online$/.test(o));
  test.skip(online < 0, "no online device to take offline");
  await select.selectOption({ index: online });
  const unit = options[online].split(" · ")[0];
  await page.getByRole("button", { name: /Device offline/ }).click();
  await expect(page.getByText(`${unit}: connection lost`)).toBeVisible();
  await expect(select.locator("option:checked")).toHaveText(/ · offline · fault open$/);
  await page.getByRole("button", { name: /Restore connection/ }).click();
  await expect(page.getByText(`${unit}: connection restored`)).toBeVisible();
  await expect(select.locator("option:checked")).toHaveText(/ · online$/);
});

test("Expire session asks to sign in again", async ({ page, app }) => {
  await page.goto("/demo");
  await page.getByRole("button", { name: /Expire session/ }).click();
  await expect(page).toHaveURL(/\/login\?error=expired$/);
  await expect(page.getByRole("status")).toContainText("Your session ended");
  await page.goto(app.home);
  await expect(page).toHaveURL(/\/login\?returnTo=/);
});
