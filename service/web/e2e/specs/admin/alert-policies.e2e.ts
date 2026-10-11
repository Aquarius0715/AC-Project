// HQ alert policies (FR-A05, DD-A05 items 7–8, Figma Admin 38:5 / 356:3714, IR345): “+ New policy” asks for the owner
// customer first; the draft starts off, at priority 50; Save policy checks the name and the recipients before anything
// is sent; recipients come from notifications.recipients; the created policy opens by its policyId; an edit shows in the
// footer; Delete policy removes it. A policy left behind by a failed run is deleted in afterEach through the app's
// operation relay (policies.delete with its version). The unit page's “+ New policy for …” opens the same draft.
import { test, expect } from "../../fixtures/test";

let created: string | null = null;

test.afterEach(async ({ page }) => {
  if (!created) return;
  const status = await page.evaluate(async (id) => {
    const got = await fetch("/bff/ops/policies.get", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ policyId: id }) });
    if (got.status === 404) return 200; // the test deleted it
    const { data } = await got.json();
    return (await fetch("/bff/ops/policies.delete", {
      method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID(), "X-Expected-Version": String(data.version) }, body: JSON.stringify({ policyId: id }),
    })).status;
  }, created);
  expect(status).toBe(200);
  created = null;
});

test("HQ creates an alert policy for a customer, edits it and deletes it", async ({ page }) => {
  const name = `E2E too hot ${Date.now()}`;
  await page.goto("/admin/alerts?tab=policies");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "+ New policy" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "New alert policy" })).toBeVisible();
  const owner = dialog.getByRole("combobox", { name: "Owner (customer)" });
  await owner.click();
  await owner.fill("Demo Customer A");
  await dialog.getByRole("option", { name: "Demo Customer A" }).click();
  await dialog.getByRole("button", { name: "Start the policy" }).click();
  await page.waitForURL(/tab=policies&new=[0-9a-f-]{36}/);

  // the draft: off, priority 50, in-app always on, nobody notified yet
  await expect(main.getByRole("switch", { name: "Enabled" })).toHaveAttribute("aria-checked", "false");
  await expect(main.getByRole("spinbutton", { name: /^Priority/ })).toHaveValue("50"); // a field's name includes its hint
  await expect(main.getByRole("checkbox", { name: "In-app" })).toBeDisabled();
  await expect(main).toContainText("Owner (customer) · fixed after creation");
  await expect(main).toContainText("New policy · not saved yet");
  await main.getByRole("button", { name: "Save policy" }).click(); // checked before anything is sent
  await expect(main).toContainText("VALIDATION · 2 problems · nothing saved");
  await expect(main).toContainText("Add at least one recipient");

  await main.getByRole("textbox", { name: /^Name/ }).fill(name);
  await main.getByRole("button", { name: "+ Add recipient" }).click();
  const picker = page.getByRole("dialog");
  await expect(picker.getByRole("heading", { name: "Add recipients" })).toBeVisible();
  await picker.getByRole("checkbox", { name: /^hq-operator/ }).check(); // HQ with alert.read, next to the customer's users
  await picker.getByRole("button", { name: "Add 1 recipient" }).click();
  await expect(main.getByRole("button", { name: "Remove hq-operator" })).toBeVisible();
  await main.getByRole("button", { name: "Save policy" }).click();
  await page.waitForURL(/policyId=[0-9a-f-]{36}/);
  created = new URL(page.url()).searchParams.get("policyId");
  await expect(main.getByRole("heading", { name: new RegExp(name) })).toBeVisible();
  await expect(main).toContainText("Policy v1");
  await expect(main).toContainText("made by hq-operator");
  await expect(main).toContainText("Not attached to any unit yet");
  await expect(main).toContainText("No unsaved changes");

  // an edit shows in the footer; Discard puts it back; Save makes version 2
  await main.getByRole("switch", { name: "Enabled" }).click();
  await expect(main).toContainText("1 unsaved change · Enabled");
  await main.getByRole("button", { name: "Discard" }).click();
  await expect(main).toContainText("No unsaved changes");
  await main.getByRole("spinbutton", { name: /^For \(s\)/ }).fill("120");
  await main.getByRole("button", { name: "Save policy" }).click();
  await expect(main).toContainText("Policy v2");

  // Delete policy: not attached to any unit, so nothing is detached
  await main.getByRole("button", { name: "Delete policy" }).click();
  const confirm = page.getByRole("dialog");
  await expect(confirm).toContainText("is deleted. It is not attached to any unit.");
  await confirm.getByRole("button", { name: "Delete policy" }).click();
  await page.waitForURL((u) => !u.searchParams.has("policyId"));
  await expect(main.getByRole("button", { name: new RegExp(name) })).toHaveCount(0);
});

test("the unit page's new-policy link opens a draft for that customer", async ({ page }) => {
  await page.goto("/admin/units");
  const main = page.getByRole("main");
  await main.getByRole("row", { name: /Demo Customer A/ }).click();
  await page.waitForURL(/customerId=/);
  await main.getByRole("tab", { name: /^Alert policies/ }).click();
  await page.waitForURL(/tab=policies/);
  await main.getByRole("link", { name: "+ New policy for Demo Customer A" }).click();
  await page.waitForURL(/\/admin\/alerts\?tab=policies&new=[0-9a-f-]{36}$/);
  await expect(main.getByRole("heading", { name: /New alert policy/ })).toBeVisible();
  await expect(main).toContainText("Demo Customer A");
  await main.getByRole("button", { name: "Discard" }).click(); // nothing is created
  await page.waitForURL((u) => !u.searchParams.has("new"));
});
