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
  await page.waitForURL(/\/admin\/alerts\?tab=policies&customerId=([0-9a-f-]{36})&new=\1$/); // scoped to that customer
  await expect(main.getByRole("heading", { name: /New alert policy/ })).toBeVisible();
  await expect(main).toContainText("Demo Customer A");
  await main.getByRole("button", { name: "Discard" }).click(); // nothing is created
  await page.waitForURL((u) => !u.searchParams.has("new"));
});

// DD-A05 item 8 (Figma 268:2, IR346): the Policies tab shares the alerts' customer → property → unit scope; scoped to a
// customer, “+ New policy for <customer>” opens its draft at once; “Only if …” limits the days and hours; Show disabled
// off hides a disabled policy unless it is open; a unit scope lists the policies attached to that unit (the seed has
// only the default policy); switching tabs keeps the scope.
test("the Policies tab's scope, Show disabled and a scoped new policy with active hours", async ({ page }) => {
  const name = `E2E office hours ${Date.now()}`;
  await page.goto("/admin/alerts?tab=policies");
  const main = page.getByRole("main");
  await main.getByRole("combobox", { name: "Customer" }).selectOption({ label: "Demo Customer A" });
  await page.waitForURL(/customerId=/);
  await main.getByRole("button", { name: "+ New policy for Demo Customer A" }).click();
  await page.waitForURL((u) => u.searchParams.get("tab") === "policies" && !!u.searchParams.get("new") && u.searchParams.get("new") === u.searchParams.get("customerId"));

  await expect(main).toContainText("Any day · any time");
  await main.getByRole("checkbox", { name: "Only on some days and times" }).check();
  await expect(main).toContainText("only 08:00–19:00 on weekdays");
  await main.getByRole("button", { name: "Mon", exact: true }).click();
  await expect(main).toContainText("only 08:00–19:00 on Tue, Wed, Thu, Fri");
  await main.getByRole("textbox", { name: /^Name/ }).fill(name);
  await main.getByRole("button", { name: "+ Add recipient" }).click();
  const picker = page.getByRole("dialog");
  await picker.getByRole("checkbox", { name: /^hq-operator/ }).check();
  await picker.getByRole("button", { name: "Add 1 recipient" }).click();
  await main.getByRole("button", { name: "Save policy" }).click();
  await page.waitForURL(/policyId=[0-9a-f-]{36}/);
  created = new URL(page.url()).searchParams.get("policyId");
  await expect(main).toContainText("only 08:00–19:00 on Tue, Wed, Thu, Fri"); // as saved
  expect(new URL(page.url()).searchParams.get("customerId")).toBeTruthy(); // still scoped

  // Show disabled off: the new policy is off, yet it stays listed while it is open
  const list = main.locator("section").filter({ has: page.getByRole("heading", { name: "Alert policies", exact: true }) });
  await main.getByRole("checkbox", { name: "Show disabled" }).uncheck();
  await expect(list.getByRole("button", { name: new RegExp(name) })).toBeVisible();
  await list.getByRole("button", { name: /Default policy/ }).click();
  await expect(list.getByRole("button", { name: new RegExp(name) })).toHaveCount(0);
  await main.getByRole("checkbox", { name: "Show disabled" }).check();
  await expect(list.getByRole("button", { name: new RegExp(name) })).toBeVisible();

  // a unit scope: the policies attached to it — the new policy is not attached, the default policy is on every unit
  await main.getByRole("combobox", { name: "Unit" }).selectOption({ label: "Bedroom AC" });
  await page.waitForURL(/unitId=/);
  await expect(main).toContainText("Showing the policies attached to Bedroom AC: Default policy.");
  await expect(list.getByRole("button", { name: new RegExp(name) })).toHaveCount(0);
  // switching tabs keeps the scope
  await main.getByRole("tab", { name: /^Alerts/ }).click();
  await page.waitForURL((u) => !u.searchParams.has("tab") && u.searchParams.has("unitId") && u.searchParams.has("customerId"));
  await expect(main.getByRole("heading", { name: "Open alerts · Demo Customer A", exact: true })).toBeVisible();
});

// DD-A05 item 9 (Figma 268:2, IR347): the default policy's six rules; a rule's limit saved for every unit and put back;
// a customer's switch of one rule with a reason, switched back on; a rule copied as a customer's policy (the owner is
// asked, the draft carries the rule's name and condition, nothing is saved); a rule whose metric no policy may watch
// cannot be copied. afterEach puts the limits and the switch back through the operation relay if a run stopped halfway.
type DefaultPolicy = { id: string; version: number; rules: { ruleKey: string; threshold: number }[]; ruleSettings: { ruleKey: string; customerId: string; enabled: boolean; version: number }[] };
const relay = async (page: import("@playwright/test").Page, op: string, input: unknown, version?: number) => page.evaluate(async ({ op, input, version }) => {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (version !== undefined) Object.assign(headers, { "Idempotency-Key": crypto.randomUUID(), "X-Expected-Version": String(version) });
  const r = await fetch(`/bff/ops/${op}`, { method: "POST", headers, body: JSON.stringify(input) });
  return { status: r.status, body: await r.json() };
}, { op, input, version });
let before: DefaultPolicy | null = null;

test.afterEach(async ({ page }) => {
  if (!before) return;
  const now = (await relay(page, "policies.list", { limit: 1, filters: { kind: "default_alert" } })).body.data.items[0] as DefaultPolicy;
  if (JSON.stringify(now.rules) !== JSON.stringify(before.rules)) expect((await relay(page, "policies.save", { id: now.id, kind: "default_alert", rules: before.rules }, now.version)).status).toBe(200);
  for (const s of now.ruleSettings.filter((x) => !x.enabled && !before!.ruleSettings.some((b) => b.ruleKey === x.ruleKey && b.customerId === x.customerId && !b.enabled))) {
    expect((await relay(page, "policies.setDefaultRule", { policyId: now.id, ruleKey: s.ruleKey, customerId: s.customerId, enabled: true }, s.version)).status).toBe(200);
  }
  before = null;
});

test("the default policy's rules, a customer's switch and a rule copied as a customer policy", async ({ page }) => {
  await page.goto("/admin/alerts?tab=policies");
  before = (await relay(page, "policies.list", { limit: 1, filters: { kind: "default_alert" } })).body.data.items[0] as DefaultPolicy;
  const main = page.getByRole("main");
  const list = main.locator("section").filter({ has: page.getByRole("heading", { name: "Alert policies", exact: true }) });
  await list.getByRole("button", { name: /Default policy/ }).click();
  await page.waitForURL(/policyId=/);
  await expect(main.getByRole("heading", { name: "Default policy › Ventilation" })).toBeVisible();
  const rules = main.getByRole("region", { name: "Rules" });
  await expect(rules.getByRole("button")).toHaveCount(6);

  // a limit for every unit: Refrigerant low pressure ≤ 350 kPa → 340, saved, then put back
  await rules.getByRole("button", { name: /Refrigerant low pressure/ }).click();
  await expect(main).toContainText("rule 3 of 6");
  await expect(main.getByRole("textbox", { name: /^Metric/ })).toHaveValue("Refrigerant pressure"); // fixed for a default rule
  const threshold = main.getByRole("spinbutton", { name: /^Alert when ≤/ });
  await threshold.fill("340");
  await expect(main).toContainText("1 rule changed · Refrigerant low pressure");
  await main.getByRole("button", { name: "Save policy" }).click();
  await expect(main).toContainText(`Policy v${before.version + 1}`);
  await expect(main).toContainText("No unsaved changes");
  await threshold.fill("350");
  await main.getByRole("button", { name: "Save policy" }).click();
  await expect(main).toContainText(`Policy v${before.version + 2}`);

  // one customer's switch, with a reason, then back on
  const row = (name: string) => main.locator("div").filter({ has: page.getByRole("switch", { name: `Refrigerant low pressure for ${name}` }) }).last();
  await row("Demo Customer B").getByRole("switch").click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "Switch “Refrigerant low pressure” off for Demo Customer B" })).toBeVisible();
  await dialog.getByLabel(/^Reason · optional/).fill("E2E rooftop unit replaced");
  await dialog.getByRole("button", { name: "Switch off" }).click();
  await expect(row("Demo Customer B")).toContainText(/Off · by hq-operator, .* \(E2E rooftop unit replaced\)/);
  await row("Demo Customer B").getByRole("switch").click();
  await page.getByRole("dialog").getByRole("button", { name: "Switch on" }).click();
  await expect(row("Demo Customer B")).toContainText(/^.*On · by hq-operator/);

  // copy: Ventilation (CO₂) becomes a draft for the owner chosen; a metric no policy may watch cannot be copied
  await rules.getByRole("button", { name: /Clogged filter/ }).click();
  await expect(main).toContainText("is not a metric a customer policy can watch, so this rule cannot be copied.");
  await rules.getByRole("button", { name: /^Ventilation/ }).click();
  await main.getByRole("button", { name: "Copy as a customer policy →" }).click();
  const owner = page.getByRole("dialog").getByRole("combobox", { name: "Owner (customer)" });
  await owner.click();
  await owner.fill("Demo Customer A");
  await page.getByRole("dialog").getByRole("option", { name: "Demo Customer A" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Start the policy" }).click();
  await page.waitForURL((u) => u.searchParams.get("from") === "ventilation_co2" && !!u.searchParams.get("new"));
  await expect(main.getByRole("textbox", { name: /^Name/ })).toHaveValue("Ventilation");
  await expect(main.getByRole("spinbutton", { name: /^Alert when ≥/ })).toHaveValue("1000");
  await main.getByRole("button", { name: "Discard" }).click(); // nothing is created
  await page.waitForURL((u) => !u.searchParams.has("new") && !u.searchParams.has("from"));
});
