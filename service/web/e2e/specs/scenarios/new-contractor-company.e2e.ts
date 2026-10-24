// A new contractor company from nothing, with the normal screens only (business operation review 2026-10-15, high-1
// and high-2; IR461, IR465): HQ adds the company with its profile in the contractor register, then its first
// contractor as a new person in Access & roles; the contractor sets a password from the set-up e-mail (Mailpit) and
// signs in to the partner app, which shows their company and their own account. Each run leaves one company and one
// person, under names of their own. Skipped on a stack without Mailpit.
import type { Browser, Page } from "@playwright/test";
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { APPS, type App } from "../../fixtures/apps";
import { mailpitUp, setupLink } from "../../fixtures/mailpit";

const app = (id: App["id"]) => APPS.find((a) => a.id === id)!;
const open = async (browser: Browser, a: App, signedIn = true): Promise<Page> =>
  (await browser.newContext({ baseURL: a.url, viewport: { width: 1920, height: 1080 }, ...(signedIn ? { storageState: a.state } : { storageState: { cookies: [], origins: [] } }) })).newPage();

test("HQ adds a contractor company and its first contractor, who signs in to the partner app", async ({ browser }) => {
  test.setTimeout(240_000);
  test.skip(!(await mailpitUp()), "needs the local Mailpit inbox");
  const stamp = Date.now();
  const company = `E2E Partner ${stamp}`;
  const email = `e2e-boss-${stamp}@example.com`;
  const admin = await open(browser, app("admin"));

  await test.step("HQ adds the company with its profile", async () => {
    await admin.goto("/admin/jobs?tab=contractors");
    await admin.getByRole("main").getByRole("button", { name: "Add contractor" }).click();
    const dialog = admin.getByRole("dialog");
    await dialog.getByRole("combobox", { name: /^Contractor organization/ }).selectOption({ label: "New company…" });
    await dialog.getByRole("textbox", { name: /^Company name/ }).fill(company);
    await dialog.getByRole("textbox", { name: /^Registration number/ }).fill("SSM 202609876543");
    await dialog.getByRole("textbox", { name: /^Service areas/ }).fill("Kuala Lumpur");
    await dialog.getByRole("textbox", { name: /^Contact e-mail/ }).fill("ops@partner.example");
    await dialog.getByRole("button", { name: "Add contractor" }).click();
    await admin.waitForURL(/contractorId=[0-9a-f-]{36}/);
  });

  await test.step("HQ adds its first contractor as a new person", async () => {
    await admin.goto("/admin/settings/access");
    const main = admin.getByRole("main");
    await main.getByRole("button", { name: "New membership" }).click();
    await admin.waitForURL(/membershipId=new/);
    await main.getByRole("button", { name: "New person" }).click();
    await main.getByRole("textbox", { name: /^Name/ }).fill("E2E Boss");
    await main.getByRole("textbox", { name: /^Email/ }).fill(email);
    await main.getByRole("button", { name: "Contractor", exact: true }).last().click(); // the role, after the role filter
    await main.getByRole("combobox", { name: /^Organization/ }).selectOption({ label: company });
    await main.getByLabel("Valid from").fill("2026-09-01T09:00"); // before the seed's demo clock
    await main.getByRole("textbox", { name: /^Change reason/ }).fill("E2E: the new company's first contractor");
    await main.getByRole("button", { name: "Create membership" }).click();
    await expect(admin.getByText("Membership created").first()).toBeVisible();
  });

  await test.step("the contractor sets a password and signs in to the partner app", async () => {
    const link = await setupLink(email);
    const me = await open(browser, app("partner"), false);
    const password = `E2e-${crypto.randomUUID()}`;
    await me.goto(link);
    const proceed = me.getByRole("link", { name: /Click here to proceed/ });
    if (await proceed.isVisible({ timeout: 5_000 }).catch(() => false)) await proceed.click();
    await me.locator("#password-new").fill(password);
    await me.locator("#password-confirm").fill(password);
    await me.locator("#password-new").press("Enter");
    await expect(me.getByText(/Your account has been updated/)).toBeVisible();
    const partner = app("partner");
    await me.goto(`${partner.url}/login`);
    await me.getByRole("link", { name: /^Continue as / }).click();
    await me.locator("#username").fill(email);
    await me.locator("#password").fill(password);
    await me.locator("#kc-login").click();
    await me.waitForURL((u) => u.origin === new URL(partner.url).origin && (u.pathname === partner.home || u.pathname.startsWith(partner.home + "/")));
    await expect(me.locator("body")).not.toContainText(FAILED_PAGE);
    await expect(me.getByText("Contractor — E2E Boss").first()).toBeVisible();
    await expect(me.locator("aside")).toContainText(new RegExp(company, "i"));
    await me.goto(`${partner.url}/partner/users`);
    await expect(me.getByRole("row").filter({ hasText: "E2E Boss (you)" })).toContainText("Your account");
    await me.context().close();
  });
  await admin.context().close();
});
