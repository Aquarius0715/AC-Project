// Keyboard-only use of a dialog (NFR-01, UX-06, the WAI-ARIA dialog pattern, IR361): opened with Enter, focus moves into
// the dialog, Tab and Shift+Tab stay inside it, Escape closes it and focus returns to the trigger. One dialog per app;
// opening them changes nothing.
import { test, expect } from "../../fixtures/test";

const DIALOGS: Record<string, [path: string, trigger: string | RegExp]> = {
  customer: ["/customer/users", "+ Invite member"],
  partner: ["/partner/team", "+ Unavailable days"],
  technician: ["/technician", /Scan QR/],
  admin: ["/admin/alerts?tab=policies", "+ New policy"],
};

test("a dialog opened by keyboard keeps focus inside and returns it to its trigger", async ({ page, app }) => {
  const [path, name] = DIALOGS[app.id];
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
    expect(await inside(), `Tab ${i + 1} stays in the dialog`).toBe(true);
  }
  await page.keyboard.press("Shift+Tab");
  expect(await inside(), "Shift+Tab stays in the dialog").toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(trigger).toBeFocused();
});
