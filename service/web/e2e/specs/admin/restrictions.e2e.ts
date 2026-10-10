// HQ restrictions (FR-A09, DD-A09, SCR-A09, Figma Admin 38:8, IR321): signed in as the restriction manager in its own
// browser context, the Cause invoice filter offers the invoices that restrictions cite and keeps the choice in the URL
// (invoiceId); each unit of the selected restriction links its unit screen (Details →). Nothing is scheduled,
// executed or released.
import type { BrowserContext } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

const MANAGER = "hq-restriction-manager"; // restriction.read / .write / .override, asset.read (demo seed)
let context: BrowserContext | null = null;

test.afterEach(async () => {
  await context?.close();
  context = null;
});

test("the cause invoice filter and the units' Details links", async ({ browser, app }) => {
  test.setTimeout(120_000); // a one-time code is accepted once per 30 s window, so the sign-in may wait for the next one
  context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } }); // not the project's hq-operator session
  const page = await context.newPage();
  await signIn(page, app, MANAGER);
  await page.goto("/admin/restrictions");
  const main = page.getByRole("main");
  const cause = main.getByRole("combobox", { name: "Cause invoice" });
  await expect(cause.locator("option")).toContainText(["All cause invoices", "INV-202608-0001 · Demo Customer A"]);
  await cause.selectOption({ label: "INV-202608-0001 · Demo Customer A" });
  await page.waitForURL(/invoiceId=[0-9a-f-]{36}/);
  const list = main.locator("section").filter({ has: page.getByRole("heading", { name: "Restrictions", exact: true }) });
  await expect(list.locator("button[aria-pressed]")).toHaveCount(1);

  // the selected restriction's units open their unit screen, which finds the customer itself
  const units = main.locator("section").filter({ has: page.getByRole("heading", { name: "Units", exact: true }) });
  const details = units.getByRole("link", { name: "Details →" }).first();
  await expect(details).toHaveAttribute("href", /^\/admin\/units\?unitId=[0-9a-f-]{36}$/);
  await details.click();
  await page.waitForURL(/\/admin\/units\?customerId=[0-9a-f-]{36}&unitId=[0-9a-f-]{36}$/);

  // an invoice no restriction cites stays selectable from a link and says so
  await page.goto("/admin/restrictions?invoiceId=00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("combobox", { name: "Cause invoice" })).toContainText("no restriction cites it");
  await expect(main.getByRole("heading", { name: "○ No restrictions" })).toBeVisible();
});
