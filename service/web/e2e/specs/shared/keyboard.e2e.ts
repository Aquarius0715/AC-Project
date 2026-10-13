// Keyboard-only use of a dialog (NFR-01, UX-06, the WAI-ARIA dialog pattern, IR361): opened with Enter, focus moves into
// the dialog, Tab and Shift+Tab stay inside it, Escape closes it and focus returns to the trigger. One dialog per app;
// opening them changes nothing, and nothing in them is refused by the CSP.
import { test, expect } from "../../fixtures/test";

const DIALOGS: Record<string, [path: string, trigger: string | RegExp]> = {
  customer: ["/customer/users", "Invite user"],
  partner: ["/partner/team", "Unavailable days"],
  technician: ["/technician", /Scan QR/],
  admin: ["/admin/alerts?tab=policies", "New policy"],
};

test("a dialog opened by keyboard keeps focus inside and returns it to its trigger", async ({ page, app }) => {
  const [path, name] = DIALOGS[app.id];
  // the dialog's parts (the shadcn/ui Dialog on Base UI, DEC-75) add nothing the CSP refuses
  await page.addInitScript(() => document.addEventListener("securitypolicyviolation", (e) => ((window as unknown as { refused: string[] }).refused ??= []).push(e.violatedDirective)));
  await page.goto(path);
  const trigger = page.getByRole("button", { name }).first();
  await trigger.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const inside = () => page.evaluate(() => !!document.activeElement?.closest('[role="dialog"]'));
  await expect.poll(inside).toBe(true);
  for (let i = 0; i < 12; i++) {
    await page.keyboard.press("Tab");
    // past the last control focus lands on the dialog's focus guard, which hands it to the first one on the next frame
    await expect.poll(inside, { message: `Tab ${i + 1} stays in the dialog`, timeout: 1000 }).toBe(true);
  }
  await page.keyboard.press("Shift+Tab");
  await expect.poll(inside, { message: "Shift+Tab stays in the dialog", timeout: 1000 }).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
  expect(await page.evaluate(() => (window as unknown as { refused?: string[] }).refused ?? []), "CSP violations").toEqual([]);
});
