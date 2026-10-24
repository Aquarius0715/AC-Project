// IR461, IR463: a technician the contractor adds sets their password from the set-up e-mail (the local Mailpit inbox)
// and signs in to the technician app as themselves: the account the sign-in service opened carries the tenant,
// membership and role claims the apps read, so the app frame shows their role, name and company, and the contractor's list
// then shows them as Active. The contractor removes them again. Skipped on a stack without Mailpit.
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { APPS } from "../../fixtures/apps";

const MAILPIT = "http://localhost:8025/api/v1";

/** The set-up link of the newest e-mail to `email`. */
async function setupLink(email: string): Promise<string> {
  const search = `${MAILPIT}/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  let id = "";
  await expect.poll(async () => {
    id = ((await (await fetch(search)).json()) as { messages: { ID: string }[] }).messages[0]?.ID ?? "";
    return id;
  }, { timeout: 30_000 }).not.toBe("");
  const text = ((await (await fetch(`${MAILPIT}/message/${id}`)).json()) as { Text: string }).Text;
  const link = text.match(/https?:\/\/\S+\/login-actions\/action-token\?\S+/)?.[0];
  if (!link) throw new Error("the set-up e-mail has no link");
  return link;
}

test("a technician the contractor adds sets a password from the e-mail and signs in as themselves", async ({ page, app, browser }) => {
  test.skip(app.id !== "partner", "partner only");
  test.skip(!(await fetch(`${MAILPIT}/messages?limit=1`).catch(() => null))?.ok, "needs the local Mailpit inbox");
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
  await own.close();

  // the first sign-in made them active; the contractor removes them again
  await page.reload();
  const row = page.getByRole("row").filter({ hasText: email });
  await expect(row).toContainText("Active");
  await row.getByRole("button", { name: "Remove" }).click();
  await dialog.getByLabel("Reason").fill("E2E: the sign-in was checked");
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(row).toHaveCount(0);
});
