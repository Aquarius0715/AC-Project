// HQ's revoke of a contractor's technician (IR508, DEC-83): contractor-a adds a technician; HQ revokes the access in
// Access & roles; contractor-a's Users list then shows "Access ended by HQ" with only Remove, and once removed the
// address cannot come back as a new account. Each run leaves one revoked person under a name of its own.
import type { Browser, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { APPS, type App } from "../../fixtures/apps";

const app = (id: App["id"]) => APPS.find((a) => a.id === id)!;
const open = async (browser: Browser, a: App): Promise<Page> =>
  (await browser.newContext({ baseURL: a.url, viewport: { width: 1920, height: 1080 }, storageState: a.state })).newPage();

test("HQ revokes a contractor's technician, who then stays out: the contractor cannot bring them back", async ({ browser }) => {
  test.setTimeout(180_000);
  const stamp = Date.now();
  const name = `E2E Revoked ${stamp}`;
  const email = `e2e-revoked-${stamp}@example.com`;
  const partner = await open(browser, app("partner"));
  const row = (text: string) => partner.getByRole("row").filter({ hasText: text });
  const dialog = partner.getByRole("dialog");

  await test.step("contractor-a adds a technician", async () => {
    await partner.goto("/partner/users");
    await partner.getByRole("button", { name: "Add user" }).click();
    await dialog.getByLabel("Name").fill(name);
    await dialog.getByLabel("Email").fill(email);
    await dialog.getByRole("combobox", { name: "Role" }).selectOption("technician");
    await dialog.getByRole("button", { name: "Add user" }).click();
    await expect(row(email)).toContainText("Not signed in yet");
  });

  await test.step("HQ revokes the access in Access & roles", async () => {
    const admin = await open(browser, app("admin"));
    await admin.goto("/admin/settings/access");
    const main = admin.getByRole("main");
    await main.getByPlaceholder("Search name or organization…").fill(name);
    await main.locator("button[aria-pressed]", { hasText: name }).click();
    await admin.waitForURL(/membershipId=[0-9a-f-]{36}/);
    await main.getByRole("button", { name: "Revoke access…" }).click();
    const revoke = admin.getByRole("dialog");
    await revoke.getByRole("textbox", { name: /^Reason/ }).fill("E2E: the contract ended early");
    await revoke.getByRole("button", { name: "Revoke", exact: true }).click();
    await expect(main.locator('button[aria-pressed="true"]', { hasText: name })).toContainText("Revoked");
  });

  await test.step("the contractor sees the access ended by HQ, with only Remove", async () => {
    await partner.reload();
    await expect(row(email)).toContainText("Access ended by HQ");
    await expect(row(email)).toContainText("Only HQ can give the access back");
    for (const button of ["Access", "Enable", "Disable", "Resend set-up e-mail"]) await expect(row(email).getByRole("button", { name: button })).toHaveCount(0);
  });

  await test.step("removed, the person cannot come back as a new account", async () => {
    await row(email).getByRole("button", { name: "Remove" }).click();
    await dialog.getByLabel("Reason").fill("E2E: left the company");
    await dialog.getByRole("button", { name: "Remove" }).click();
    await expect(row(email)).toHaveCount(0);
    await partner.getByRole("button", { name: "Add user" }).click();
    await dialog.getByLabel("Name").fill(name);
    await dialog.getByLabel("Email").fill(email);
    await dialog.getByRole("combobox", { name: "Role" }).selectOption("technician");
    await dialog.getByRole("button", { name: "Add user" }).click();
    await expect(partner.getByText("HQ ended this person's access — only HQ can give it back").first()).toBeVisible();
    await expect(row(email)).toHaveCount(0);
  });
});
