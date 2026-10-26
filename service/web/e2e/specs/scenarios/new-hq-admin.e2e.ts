// A new HQ administrator from Access & roles signs in with two steps (business operation review 2026-10-15, high-1;
// IR461, IR468): HQ adds a new person as Admin; their set-up e-mail (Mailpit) sets the password and a one-time-code
// device — the realm asks for a code only once a device is set up, so an HQ account without one would sign in with a
// password alone — and the admin app then asks for the password and a code. Each run leaves one person with a
// membership, revoked at the end. Skipped on a stack without Mailpit.
import type { Browser, Page } from "@playwright/test";
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { APPS, type App } from "../../fixtures/apps";
import { totpBase32 } from "../../fixtures/auth";
import { mailpitUp, setupLink } from "../../fixtures/mailpit";

const app = (id: App["id"]) => APPS.find((a) => a.id === id)!;
const open = async (browser: Browser, a: App, signedIn = true): Promise<Page> =>
  (await browser.newContext({ baseURL: a.url, viewport: { width: 1920, height: 1080 }, ...(signedIn ? { storageState: a.state } : { storageState: { cookies: [], origins: [] } }) })).newPage();

test("a new HQ administrator sets a password and a one-time-code device, then signs in with both", async ({ browser }) => {
  test.setTimeout(240_000);
  test.skip(!(await mailpitUp()), "needs the local Mailpit inbox");
  const email = `e2e-admin-${Date.now()}@example.com`;
  const admin = await open(browser, app("admin"));
  const main = admin.getByRole("main");

  await test.step("HQ adds the person as Admin", async () => {
    await admin.goto("/admin/settings/access");
    await main.getByRole("button", { name: "New membership" }).click();
    await admin.waitForURL(/membershipId=new/);
    await main.getByRole("button", { name: "New person" }).click();
    await main.getByRole("textbox", { name: /^Name/ }).fill("E2E Second Admin");
    await main.getByRole("textbox", { name: /^Email/ }).fill(email);
    await main.getByRole("button", { name: "Admin", exact: true }).last().click(); // the role, after the role filter
    await main.getByRole("combobox", { name: /^Organization/ }).selectOption({ label: "Demo HQ" });
    await main.getByLabel("Valid from").fill("2026-09-01T09:00"); // before the seed's demo clock
    await main.getByRole("textbox", { name: /^Change reason/ }).fill("E2E: a second administrator");
    await main.getByRole("button", { name: "Create membership" }).click();
    await expect(admin.getByText("Membership created").first()).toBeVisible();
    await admin.waitForURL(/membershipId=[0-9a-f-]{36}/);
  });

  const password = `E2e-${crypto.randomUUID()}`;
  let key = "";
  const me = await open(browser, app("admin"), false);
  await test.step("the set-up e-mail sets the password and a one-time-code device", async () => {
    await me.goto(await setupLink(email));
    // the identity provider may list the actions first: wait for that page or the first step (isVisible does not wait)
    const proceed = me.getByRole("link", { name: /Click here to proceed/ });
    await expect(proceed.or(me.locator("#password-new")).or(me.locator("#totp")).first()).toBeVisible();
    if (await proceed.isVisible()) await proceed.click();
    // the identity provider orders the steps itself (the device came first): take each page as it comes
    const done = me.getByText(/Your account has been updated/);
    for (let step = 0; step < 4 && !(await done.isVisible().catch(() => false)); step++) {
      const page = await Promise.race([
        me.locator("#password-new").waitFor().then(() => "password" as const),
        me.locator("#totp").waitFor().then(() => "device" as const),
        done.waitFor().then(() => "done" as const),
      ]);
      if (page === "password") {
        await me.locator("#password-new").fill(password);
        await me.locator("#password-confirm").fill(password);
        await me.locator("#password-new").press("Enter");
      } else if (page === "device") { // the key in text instead of the QR code
        await me.getByRole("link", { name: /Unable to scan/ }).click();
        key = (await me.locator("#kc-totp-secret-key").innerText()).trim();
        expect(key).toMatch(/^[A-Z2-7 ]{16,}$/);
        await me.locator("#totp").fill(totpBase32(key));
        await me.locator("#userLabel").fill("E2E phone");
        await me.locator("#saveTOTPBtn").click();
        await me.locator("#totp").waitFor({ state: "detached" });
      }
    }
    await expect(done).toBeVisible();
    expect(key).not.toBe(""); // a device was set up
  });

  await test.step("the admin app asks for the password and a code", async () => {
    const hq = app("admin");
    await me.goto(`${hq.url}/login`);
    await me.getByRole("link", { name: /^Continue as / }).click();
    await me.locator("#username").fill(email);
    await me.locator("#password").fill(password);
    await me.locator("#kc-login").click();
    await expect(me.locator("#otp")).toBeVisible(); // two steps
    // a code is accepted once per 30 s window: the set-up used this window's, so take the next one when it starts
    const window = Math.floor(Date.now() / 30_000);
    await expect.poll(() => Math.floor(Date.now() / 30_000), { timeout: 35_000, intervals: [1_000] }).toBeGreaterThan(window);
    await me.locator("#otp").fill(totpBase32(key));
    await me.locator("#kc-login").click();
    await me.waitForURL((u) => u.origin === new URL(hq.url).origin && (u.pathname === hq.home || u.pathname.startsWith(hq.home + "/")));
    await expect(me.locator("body")).not.toContainText(FAILED_PAGE);
    await expect(me.getByText("Admin — E2E Second Admin").first()).toBeVisible();
    await me.context().close();
  });

  await test.step("HQ revokes the membership again", async () => {
    await main.getByRole("button", { name: "Revoke access…" }).click();
    const dialog = admin.getByRole("dialog");
    await dialog.getByRole("textbox", { name: /^Reason/ }).fill("E2E: done");
    await dialog.getByRole("button", { name: "Revoke", exact: true }).click();
    await expect(admin.getByText("Access revoked — the membership is now inactive").first()).toBeVisible();
  });
  await admin.context().close();
});
