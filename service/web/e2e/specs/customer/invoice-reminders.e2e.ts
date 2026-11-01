// Customer invoice reminders (FR-C11, IR578): the e-mail preview opens and sends nothing; WhatsApp reaches only an
// account with a phone number, so without one its preview is disabled and says why. Nothing is paid.
import { test, expect } from "../../fixtures/test";

test("the reminder previews offer only the channels that reach the customer", async ({ page }) => {
  await page.goto("/customer/payments");
  const pay = page.getByRole("main").getByRole("link", { name: "Pay now" }).first();
  await expect(pay).toBeVisible(); // the seed's overdue invoice
  await pay.click();
  const whatsapp = page.getByRole("button", { name: "Preview WhatsApp" });
  await expect(whatsapp).toBeVisible();
  if (await whatsapp.isDisabled()) await expect(page.getByRole("main")).toContainText("WhatsApp reminders need a phone number on your account");
  await page.getByRole("button", { name: "Preview email" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(/Subject: Payment reminder — /);
  await expect(dialog).toContainText("Preview — not sent.");
  await expect(page.getByText(/recipient ineligible/i)).toHaveCount(0);
});
