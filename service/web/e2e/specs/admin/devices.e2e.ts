// HQ IoT devices (FR-A04, DD-A04, Figma Admin 246:2, IR319): the list is sorted by serial with the state chips — All,
// Online, Offline, Tamper, No sensors — and a search; the detail shows the tiles, the bound unit, the actions, sensors
// with Calibrate, the operation and calibration history and the events. In the seed AC-DEMO-0003 (Meeting room AC)
// is offline and AC-DEMO-0005 (Bedroom AC B) has no sensors; the state checks look at the seed's AC-DEMO devices, since
// the IoT demo's mock fleet (AC-MOCK-…, SEED_MOCK_FLEET) adds a device for every state. Nothing is sent to a device.
import { test, expect } from "../../fixtures/test";

test("the device list filters by state and the detail shows the device as Figma draws it", async ({ page }) => {
  await page.goto("/admin/devices?tab=devices");
  const main = page.getByRole("main");
  const list = main.getByRole("heading", { name: "Devices", exact: true }).locator('xpath=ancestor::*[self::section or @data-slot="card"][1]');
  const serials = async () => {
    const texts = await list.getByRole("button").allInnerTexts();
    return texts.map((x) => (x.split("\n", 1)[0] ?? "").trim());
  };
  await expect(list.getByRole("button").first()).toBeVisible();
  const seed = async () => {
    const all = await serials();
    return all.filter((x) => x.startsWith("AC-DEMO-"));
  };
  const all = await serials();
  expect(all).toEqual([...all].toSorted((a, b) => a.localeCompare(b))); // sorted by serial
  await main.getByRole("button", { name: /^Offline\s*\d+$/ }).click();
  expect(await seed()).toEqual(["AC-DEMO-0003"]);
  await main.getByRole("button", { name: /^No sensors\s*\d+$/ }).click();
  expect(await seed()).toEqual(["AC-DEMO-0005"]);
  await main.getByRole("button", { name: /^All\s*\d+$/ }).click();
  await main.getByPlaceholder("Search serial, device or unit…").fill("DEMO-0004");
  expect(await serials()).toEqual(["AC-DEMO-0004"]);
  await main.getByPlaceholder("Search serial, device or unit…").fill("");

  await list.getByRole("button", { name: /AC-DEMO-0001/ }).click();
  await page.waitForURL(/deviceId=/);
  for (const label of ["Connection", "Power signal", "Tamper", "Firmware", "Bound to unit"]) await expect(main).toContainText(label);
  for (const name of [/^Sensors \(\d+\) · from the IoT model$/, "Operation history", "Calibration history", "Device events"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main.getByRole("button", { name: "Calibrate…", exact: true }).first()).toBeVisible(); // a small button like Rebind… (IR576)
  for (const name of [/^Check connection/, /^Calibrate sensor/, /^Update firmware/]) await expect(main.getByRole("button", { name })).toBeVisible();

  // a calibration without values asks for a number under both fields (IR585); nothing is recorded
  await main.getByRole("button", { name: /^Calibrate sensor/ }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Record", exact: true }).click();
  await expect(dialog.getByText("Enter a number", { exact: true })).toHaveCount(2);
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});

// The aircon model editor (DD-A04, DEC-105): a saved model shows its capability version and only the controls of the
// air conditioner; the sensors and firmware are in the IoT models. Nothing is saved.
test("the aircon model editor has the controls and no sensors or firmware", async ({ page }) => {
  await page.goto("/admin/devices?tab=models");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /Capability v\d+/ })).toBeVisible();
  await expect(main).toContainText("No unsaved changes");
  await expect(main.getByRole("button", { name: /^Save as capability v\d+$/ })).toBeDisabled();
  await expect(main.getByRole("heading", { name: "Controls" })).toBeVisible();
  await expect(main.getByRole("region", { name: "Firmware candidates" })).toHaveCount(0);
  await expect(main.getByRole("heading", { name: "Sensors", exact: true })).toHaveCount(0);
});

// The IoT model editor (DD-A04, DEC-105, Figma Admin 106:3, IR350): a saved model shows its version; an edit marks the
// draft "Editing → v<next>" and names what changed; the firmware candidates are chips with "Add version" (a listed
// version is refused); Discard puts the saved model back. Nothing is saved.
test("the IoT model editor marks a draft, adds a firmware version and discards it", async ({ page }) => {
  await page.goto("/admin/devices?tab=iot");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: /IoT model v\d+/ })).toBeVisible();
  await expect(main).toContainText("No unsaved changes");
  await expect(main.getByRole("button", { name: /^Save as IoT model v\d+$/ })).toBeDisabled();
  const firmware = main.getByRole("region", { name: "Firmware candidates" });
  await expect(firmware.getByRole("button", { name: "Remove v1" })).toBeVisible();

  await firmware.getByRole("button", { name: "Add version" }).click();
  const version = firmware.getByRole("textbox", { name: "Firmware version" });
  await version.fill("v1");
  await expect(firmware).toContainText("v1 is already a candidate");
  await expect(firmware.getByRole("button", { name: "Add", exact: true })).toBeDisabled();
  await version.fill("v9-e2e");
  await firmware.getByRole("button", { name: "Add", exact: true }).click();
  await expect(firmware.getByRole("button", { name: "Remove v9-e2e" })).toBeVisible();
  await expect(main.getByRole("heading", { name: /Editing → v\d+/ })).toBeVisible();
  await expect(main).toContainText("1 change · firmware candidates");
  await expect(main.getByRole("button", { name: /^Save as IoT model v\d+$/ })).toBeEnabled();

  await main.getByRole("button", { name: "Discard" }).click();
  await expect(main).toContainText("No unsaved changes");
  await expect(firmware.getByRole("button", { name: "Remove v9-e2e" })).toHaveCount(0);
  await expect(main.getByRole("heading", { name: /Editing → v\d+/ })).toHaveCount(0);
});

// Registering reserves a device for its planned unit; it is bound only by Bind (D05, business operation review
// 2026-10-15, IR467): the detail says "Not bound yet", and Bind opens with the planned unit chosen. Nothing is bound
// here (Cancel); the registered device stays, under a serial of its own per run.
test("a registered device is not bound yet, and Bind starts from its planned unit", async ({ page }) => {
  await page.goto("/admin/devices?tab=devices");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "Register device" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: /^Serial/ }).fill(`AC-E2E-${String(Date.now())}`);
  const planned = dialog.getByRole("combobox", { name: /^Planned unit/ });
  await planned.selectOption({ index: 1 });
  const unitText = await planned.locator("option:checked").innerText();
  const unit = unitText.trim();
  await dialog.getByRole("button", { name: "Register", exact: true }).click();
  await expect(page.getByText("Device registered — bind it to the unit once it is installed").first()).toBeVisible();
  await page.waitForURL(/deviceId=/);
  await expect(main).toContainText("Not bound yet");
  await main.getByRole("button", { name: "Bind…" }).click();
  await expect(dialog.getByRole("heading", { name: "Bind device" })).toBeVisible();
  await expect(dialog.getByRole("combobox", { name: /^Unit/ }).locator("option:checked")).toHaveText(unit);
  await dialog.getByRole("button", { name: "Cancel" }).click();
});

test("a device is registered with its MQTT node ID, which Edit… changes and clears (IR635)", async ({ page }) => {
  await page.goto("/admin/devices?tab=devices");
  const main = page.getByRole("main");
  const stamp = String(Date.now());
  const node = `e2e_${stamp}`;
  await main.getByRole("button", { name: "Register device" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: /^Serial/ }).fill(`AC-E2E-${stamp}`);
  await dialog.getByRole("combobox", { name: /^Planned unit/ }).selectOption({ index: 1 });
  const nodeBox = dialog.getByRole("textbox", { name: /^MQTT node ID/ });
  await nodeBox.fill("bad node");
  await dialog.getByRole("button", { name: "Register", exact: true }).click();
  await expect(dialog).toContainText("Use 1–64 letters, digits, hyphens or underscores");
  await nodeBox.fill(node);
  await dialog.getByRole("button", { name: "Register", exact: true }).click();
  await page.waitForURL(/deviceId=/);
  await expect(main.getByText(node, { exact: true })).toBeVisible();
  const renamed = `${node}_b`;
  await main.getByRole("button", { name: "Edit…" }).click();
  await dialog.getByRole("textbox", { name: "Node ID" }).fill(renamed);
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByText("MQTT node ID saved").first()).toBeVisible();
  await expect(main.getByText(renamed, { exact: true })).toBeVisible();
  await main.getByRole("button", { name: "Edit…" }).click();
  await dialog.getByRole("textbox", { name: "Node ID" }).fill("");
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByText("MQTT node ID cleared").first()).toBeVisible();
  await expect(main.getByText("Not set", { exact: true })).toBeVisible();
});

// A registered device that is in no use is deleted from its detail, with a reason (DEC-103, IR652); a bound one is not
// offered for deletion until it is unbound.
test("a device registered by mistake is deleted, and a bound device must be unbound first", async ({ page }) => {
  await page.goto("/admin/devices?tab=devices");
  const main = page.getByRole("main");
  const dialog = page.getByRole("dialog");
  const serial = `AC-E2E-${String(Date.now())}`;
  await main.getByRole("button", { name: "Register device" }).click();
  await dialog.getByRole("textbox", { name: /^Serial/ }).fill(serial);
  await dialog.getByRole("combobox", { name: /^Planned unit/ }).selectOption({ index: 1 });
  await dialog.getByRole("button", { name: "Register", exact: true }).click();
  await page.waitForURL(/deviceId=/);
  await expect(main.getByRole("heading", { name: serial })).toBeVisible();

  await main.getByRole("button", { name: "Delete…", exact: true }).click();
  await dialog.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(dialog.getByText("A reason is required")).toBeVisible(); // nothing is deleted without one
  await dialog.getByRole("textbox", { name: "Reason" }).fill("E2E: registered by mistake");
  await dialog.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page.getByText("Device deleted").first()).toBeVisible();
  await expect(main.getByRole("button", { name: new RegExp(serial) })).toHaveCount(0);

  // a bound device: Unbind… is offered, Delete… is not
  await main.getByRole("button", { name: /AC-DEMO-0001/ }).click();
  await page.waitForURL(/deviceId=/);
  await expect(main.getByRole("button", { name: "Unbind…" })).toBeVisible();
  await expect(main.getByRole("button", { name: "Delete…", exact: true })).toBeDisabled();
  await expect(main).toContainText("A bound device cannot be deleted: unbind it first.");
});
