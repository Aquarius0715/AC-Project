// HQ IoT devices (FR-A04, DD-A04, Figma Admin 246:2, IR319): the list is sorted by serial with the state chips — All,
// Online, Offline, Tamper, No sensors — and a search; the detail shows the tiles, the bound unit, the actions, sensors
// with Calibrate →, the operation and calibration history and the events. In the seed AC-DEMO-0003 (Meeting room AC)
// is offline and AC-DEMO-0005 (Bedroom AC B) has no sensors. Nothing is sent to a device.
import { test, expect } from "../../fixtures/test";

test("the device list filters by state and the detail shows the device as Figma draws it", async ({ page }) => {
  await page.goto("/admin/devices?tab=devices");
  const main = page.getByRole("main");
  const list = main.getByRole("heading", { name: "Devices", exact: true }).locator("xpath=ancestor::section[1]");
  const serials = async () => (await list.getByRole("button").allInnerTexts()).map((x) => x.split("\n")[0].trim());
  await expect(list.getByRole("button").first()).toBeVisible();
  const all = await serials();
  expect(all).toEqual([...all].sort()); // sorted by serial
  await main.getByRole("button", { name: /^Offline\s*\d+$/ }).click();
  expect(await serials()).toEqual(["AC-DEMO-0003"]);
  await main.getByRole("button", { name: /^No sensors\s*\d+$/ }).click();
  expect(await serials()).toEqual(["AC-DEMO-0005"]);
  await main.getByRole("button", { name: /^All\s*\d+$/ }).click();
  await main.getByPlaceholder("Search serial, device or unit…").fill("0004");
  expect(await serials()).toEqual(["AC-DEMO-0004"]);
  await main.getByPlaceholder("Search serial, device or unit…").fill("");

  await list.getByRole("button", { name: /AC-DEMO-0001/ }).click();
  await page.waitForURL(/deviceId=/);
  for (const label of ["Connection", "Power signal", "Tamper", "Firmware", "Bound to unit"]) await expect(main).toContainText(label);
  for (const name of [/^Sensors \(\d+\) · from the unit’s capability$/, "Operation history", "Calibration history", "Device events"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main.getByRole("button", { name: "Calibrate →" }).first()).toBeVisible();
  for (const name of [/^Check connection/, /^Calibrate sensor/, /^Update firmware/]) await expect(main.getByRole("button", { name })).toBeVisible();
});
