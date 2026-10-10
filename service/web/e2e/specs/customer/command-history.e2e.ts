// FR-C03, IR258, IR314: the unit's command history words every command the contract allows — the remote control's
// settings, ventilation and a restriction's own commands. The seed's Lobby AC carries the restriction command that limits
// cooling to 24 °C (source restriction, acknowledged), which read “undefined” before IR314. Nothing is changed.
import { test, expect } from "../../fixtures/test";

test("the command history names a restriction's command and who sent it", async ({ page }) => {
  await page.goto("/customer");
  const main = page.getByRole("main");
  const link = main.locator('a[href^="/customer/units/"]').filter({ hasText: "Lobby AC" }).first();
  await page.goto((await link.getAttribute("href"))!);
  const history = main.getByRole("heading", { name: "Command history" }).locator("xpath=ancestor::section[1]");
  await expect(history).toContainText("Apply restriction: Temperature limit — cooling setpoint ≥ 24 °C — acknowledged by device · by a restriction");
  await expect(history).not.toContainText("undefined");
});
