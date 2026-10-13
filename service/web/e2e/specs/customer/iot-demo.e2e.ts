// IoT demo (docker/iot-demo.sh): customer B sees the mock fleet's long series — 30 days of daily energy and a week of
// CO2 — read from the readings the simulated IoT modules send through the broker and the bridge, joined to the seeded
// history. Skipped on a stack without the mock fleet (it is loaded only with SEED_MOCK_FLEET=1).
import { createHash } from "node:crypto";
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

/** The API ID of a fixture-style name (seed.ID: UUIDv5 in the seed namespace). */
function fixtureId(name: string): string {
  const ns = Buffer.from("6f1c3a526d0b5c1e9a570d1b7a4c2e10", "hex");
  const h = createHash("sha1").update(Buffer.concat([ns, Buffer.from(name)])).digest();
  h[6] = (h[6] & 0x0f) | 0x50;
  h[8] = (h[8] & 0x3f) | 0x80;
  const x = h.subarray(0, 16).toString("hex");
  return `${x.slice(0, 8)}-${x.slice(8, 12)}-${x.slice(12, 16)}-${x.slice(16, 20)}-${x.slice(20)}`;
}

test("the mock fleet's readings make a month of energy and a week of CO2", async ({ browser, app }) => {
  test.skip(app.id !== "customer", "client only");
  test.setTimeout(120_000);
  const unit = fixtureId("unit-mock-cool-on");
  expect(unit).toBe("d795afbf-17a2-5797-9191-28040315244b");
  const context = await browser.newContext({ storageState: { cookies: [], origins: [] } });
  const page = await context.newPage();
  await signIn(page, app, "customer-b");

  await page.goto(`${app.url}/customer/energy?unitIds=${unit}&period=30d`);
  test.skip(!(await page.getByText("Mock · cooling, on").first().isVisible().catch(() => false)), "no mock fleet on this stack (SEED_MOCK_FLEET)");
  await expect(page.locator("body")).not.toContainText(FAILED_PAGE);
  await expect(page.getByText("30 days", { exact: false }).first()).toBeVisible();
  await expect(page.locator("body")).toContainText(/Actual energy\s*\d+(\.\d)?\s*kWh/); // the month's total, from the readings
  await expect(page.getByText("No valid power readings in this period")).toHaveCount(0);

  await page.goto(`${app.url}/customer/air-quality?unitId=${unit}&metric=co2&period=7d`);
  await expect(page.locator("body")).not.toContainText(FAILED_PAGE);
  await expect(page.getByText("CO2 — last 7 days")).toBeVisible();
  await expect(page.locator("svg path").first()).toBeVisible(); // the week's line
  await context.close();
});
