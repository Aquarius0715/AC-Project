// A customer's installation request (FR-C20, FR-A06; DEC-107, IR794): the owner of customer-a asks HQ to install a new
// aircon (the form refuses a bad number of units first), HQ finds the request in Jobs › Installation requests and
// declines it with a reason, and the owner's list then shows the request declined with HQ's reason. Making the units
// and jobs is covered by the API's integration test; a run leaves one declined request.
import type { Browser, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";
import { APPS, type App } from "../../fixtures/apps";
import { must } from "../../fixtures/must";

const app = (id: App["id"]) => must(APPS.find((a) => a.id === id));
const open = async (browser: Browser, a: App): Promise<Page> => {
  const context = await browser.newContext({ baseURL: a.url, viewport: { width: 1920, height: 1080 }, storageState: a.state });
  return context.newPage();
};

test("the owner asks for an installation and HQ declines it with a reason", async ({ browser }) => {
  test.setTimeout(180_000);
  const note = `E2E installation ${String(Date.now())}`;
  const reason = "E2E: no capacity this month";
  const customer = await open(browser, app("customer"));
  const dialog = customer.getByRole("dialog");
  let number = "";

  await test.step("the owner's form refuses a bad number of units, then sends the request", async () => {
    await customer.goto("/customer/installations");
    await expect(customer.getByRole("heading", { name: "Installation requests", level: 1 })).toBeVisible();
    await customer.getByRole("button", { name: "New request" }).click();
    await expectAccessible(customer, '[role="dialog"]');
    await dialog.getByLabel("Number of units (1–10)").fill("0");
    await dialog.getByRole("button", { name: "Send request" }).click();
    await expect(dialog).toContainText("A whole number of 1–10");
    await expect(dialog.getByLabel("Number of units (1–10)")).toBeFocused();
    await dialog.getByLabel("Number of units (1–10)").fill("2");
    await dialog.getByLabel("Install an IoT device with each unit").check();
    await dialog.getByLabel("Note (optional)").fill(note);
    await dialog.getByRole("button", { name: "Send request" }).click();
    await expect(customer.getByText(/^Request \S+ sent — HQ will decide/).first()).toBeVisible();
    const row = customer.getByRole("row").filter({ hasText: "Submitted" }).first();
    await expect(row).toContainText("with an IoT device");
    const shown = await row.locator("td").first().locator("b").innerText();
    number = shown.trim();
    expect(number).toMatch(/^[0-9a-f]{8}$/);
  });

  await test.step("HQ declines it with a reason", async () => {
    const admin = await open(browser, app("admin"));
    await admin.goto("/admin/jobs?tab=installations");
    const row = admin.getByRole("row").filter({ hasText: number });
    await expect(row).toContainText("Submitted");
    await row.getByRole("button", { name: "Decline" }).click();
    const decline = admin.getByRole("dialog");
    await decline.getByRole("button", { name: "Decline" }).click();
    await expect(decline).toContainText("A reason of 5–500 characters");
    await decline.getByRole("textbox", { name: /^Reason/ }).fill(reason);
    await decline.getByRole("button", { name: "Decline" }).click();
    await expect(admin.getByText(`Request ${number} declined`).first()).toBeVisible();
  });

  await test.step("the owner sees it declined, with HQ's reason", async () => {
    await customer.goto("/customer/installations");
    const row = customer.getByRole("row").filter({ hasText: number });
    await expect(row).toContainText("Declined");
    await expect(row).toContainText(reason);
    await expect(row.getByRole("button", { name: "Cancel request" })).toHaveCount(0);
  });
});
