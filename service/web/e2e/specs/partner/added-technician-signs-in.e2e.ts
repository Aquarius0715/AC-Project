// IR461, IR463: a technician the contractor adds sets their password from the set-up e-mail (the local Mailpit inbox)
// and signs in to the technician app as themselves: the account the sign-in service opened carries the tenant,
// membership and role claims the apps read, so the app frame shows their role, name and company, and the contractor's
// list then shows them as Active. The contractor disables them: their open session is refused at once and the sign-in
// service refuses a new sign-in (business operation review 2026-10-15). The contractor removes them at the end.
// Skipped on a stack without Mailpit.
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { APPS } from "../../fixtures/apps";
import { mailpitUp, setupLink } from "../../fixtures/mailpit";

test("a technician the contractor adds sets a password from the e-mail and signs in as themselves", async ({ page, app, browser }) => {
  test.skip(app.id !== "partner", "partner only");
  test.skip(!(await mailpitUp()), "needs the local Mailpit inbox");
  const email = `e2e-signin-${Date.now()}@example.com`;
  await page.goto("/partner/users");
  await page.getByRole("button", { name: "Add user" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name").fill("E2E Signer");
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("technician");
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(page.getByText(`E2E Signer added — a set-up e-mail is on its way to ${email}`).first()).toBeVisible();

  // the new technician, in a browser of their own: the e-mail's link, a password, then the technician app
  const link = await setupLink(email);
  const own = await browser.newContext({ storageState: { cookies: [], origins: [] } }); // not the contractor's sign-in
  const me = await own.newPage();
  const password = `E2e-${crypto.randomUUID()}`;
  await me.goto(link);
  const proceed = me.getByRole("link", { name: /Click here to proceed/ });
  if (await proceed.isVisible({ timeout: 5_000 }).catch(() => false)) await proceed.click(); // the actions are listed first
  await me.locator("#password-new").fill(password);
  await me.locator("#password-confirm").fill(password);
  await me.locator("#password-new").press("Enter");
  await expect(me.getByText(/Your account has been updated/)).toBeVisible();
  const tech = APPS.find((a) => a.id === "technician")!;
  await me.goto(`${tech.url}/login`);
  await me.getByRole("link", { name: /^Continue as / }).click();
  await me.locator("#username").fill(email);
  await me.locator("#password").fill(password);
  await me.locator("#kc-login").click();
  await me.waitForURL((u) => u.origin === new URL(tech.url).origin && (u.pathname === tech.home || u.pathname.startsWith(tech.home + "/")));
  await expect(me.locator("body")).not.toContainText(FAILED_PAGE);
  await expect(me.getByText("Technician — E2E Signer").first()).toBeVisible(); // their own name and company, from their membership
  await expect(me.locator("aside")).toContainText(/Demo Contractor A/i);

  // the first sign-in made them active; the contractor disables them: the open session is refused at once, and the
  // sign-in service refuses a new sign-in (IR461)
  await page.reload();
  const row = page.getByRole("row").filter({ hasText: email });
  await expect(row).toContainText("Active");
  await row.getByRole("button", { name: "Disable" }).click();
  await dialog.getByLabel("Reason").fill("E2E: the old session must end");
  await dialog.getByRole("button", { name: "Disable" }).click();
  await expect(row).toContainText("Disabled");
  await me.reload();
  await expect(me.getByText("Technician — E2E Signer")).toHaveCount(0);
  await me.goto(`${tech.url}/login`);
  await me.getByRole("link", { name: /^Continue as / }).click();
  if (await me.locator("#password").isVisible({ timeout: 5_000 }).catch(() => false)) {
    await me.locator("#username").fill(email);
    await me.locator("#password").fill(password);
    await me.locator("#kc-login").click();
  }
  await expect(me.getByText(/disabled/i).first()).toBeVisible();
  await own.close();

  // the contractor removes them
  await row.getByRole("button", { name: "Remove" }).click();
  await dialog.getByLabel("Reason").fill("E2E: the sign-in was checked");
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(row).toHaveCount(0);
});
