// HQ signs a client user out everywhere (FR-A17, Figma Admin 02-15 → 03-8; IR384–IR386). The user's open page asks to
// sign in again; identity has ended the user's sessions at the identity provider too, so it asks for the password at
// once, without a single sign-on answer for the BFF to refuse; with it the user is back. customer-b is not a project's shared user, so the other tests keep their sessions; a run
// leaves one audit entry and a revocation time on customer-b's membership.
import { test, expect } from "../../fixtures/test";
import { APPS, type App } from "../../fixtures/apps";
import { enterPassword, signIn } from "../../fixtures/auth";

const app = (id: App["id"]) => APPS.find((a) => a.id === id)!;

test("HQ signs a client user out everywhere, and the password signs them in again", async ({ browser }) => {
  test.setTimeout(180_000);
  const customer = app("customer"), hq = app("admin");
  const userContext = await browser.newContext({ baseURL: customer.url, storageState: { cookies: [], origins: [] } });
  const adminContext = await browser.newContext({ baseURL: hq.url, viewport: { width: 1920, height: 1080 }, storageState: hq.state });
  try {
    const user = await userContext.newPage();
    await test.step("customer-b signs in to the customer app", async () => {
      await signIn(user, customer, "customer-b");
      await expect(user).toHaveURL(/\/customer$/);
    });

    let revokedAt = 0;
    await test.step("HQ signs customer-b out everywhere from the customer's Users tab", async () => {
      const admin = await adminContext.newPage();
      await admin.goto("/admin/units");
      await admin.getByRole("main").getByRole("row", { name: /Demo Customer B/ }).click();
      await admin.waitForURL(/customerId=/);
      await admin.getByRole("tab", { name: /^Users/ }).click();
      await admin.waitForURL(/tab=users/);
      await admin.getByRole("button", { name: "Actions for customer-b@ac.local" }).click();
      await admin.getByRole("menuitem", { name: "Sign out everywhere…" }).click();
      const dialog = admin.getByRole("dialog");
      await expect(dialog).toContainText("Sign out customer-b@ac.local everywhere?");
      await dialog.getByRole("button", { name: "Sign out everywhere" }).click();
      await expect(dialog.getByText(/required|1–1000/).first()).toBeVisible(); // the reason is required
      await dialog.getByRole("textbox").fill("E2E: phone reported lost");
      await dialog.getByRole("button", { name: "Sign out everywhere" }).click();
      await expect(admin.getByText("Signed out everywhere — every session of customer-b@ac.local has ended").first()).toBeVisible();
      revokedAt = Date.now();
    });

    await test.step("customer-b's next pages ask to sign in", async () => {
      // the first page drops the refused session and its error state sends the browser to sign-in; a page that showed
      // no error leaves it to the next one, which finds no session
      await user.goto("/customer");
      if (!(await user.waitForURL(/\/login/, { timeout: 10_000 }).then(() => true, () => false))) await user.goto("/customer");
      await expect(user).toHaveURL(/\/login/);
    });

    await test.step("the identity provider asks for the password at once; with it customer-b is back", async () => {
      // auth_time has whole seconds, so a sign-in within the revocation's own second is refused too (IR385); a person
      // cannot sign in again that fast, the test can. The second also gives identity time to end the provider's sessions.
      await user.waitForTimeout(Math.max(0, revokedAt + 1_100 - Date.now()));
      const refusedSso: string[] = []; // the BFF's prompt=login hop after a refused single sign-on answer (IR385)
      user.on("request", (r) => { if (/\/bff\/auth\/login\?(.*&)?reauth=1/.test(r.url())) refusedSso.push(r.url()); });
      await user.getByRole("link", { name: /^Continue as / }).click();
      await expect(user.locator("#password")).toBeVisible();
      expect(refusedSso, "the provider's single sign-on session was ended (IR386)").toEqual([]);
      await enterPassword(user, "customer-b");
      await expect(user).toHaveURL(/\/customer/);
      await expect(user.getByRole("banner")).toContainText("customer-b");
    });
  } finally {
    await userContext.close();
    await adminContext.close();
  }
});
