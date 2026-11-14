// Customer maintenance request (FR-C09, DD-C09, IR113, IR237): New request refuses short symptoms and contact details in
// the contact window — each time focusing the field to fix (IR573) — is sent with three preferred times, takes a note to the coordinator, and is cancelled with a
// reason. Whatever happens, the request this test sent is cancelled at the end.
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

async function cancelRequest(page: Page, jobId: string, reason: string) {
  await page.goto(`/customer/maintenance?jobId=${jobId}`);
  const cancel = page.getByRole("button", { name: "Cancel request" });
  if ((await cancel.count()) === 0) return; // already cancelled
  await cancel.click();
  const dialog = page.getByRole("dialog");
  await expectAccessible(page, '[role="dialog"]'); // the open dialog (IR360)
  await dialog.getByLabel(/Reason/).fill(reason);
  await dialog.getByRole("button", { name: "Cancel request" }).click();
  await expect(page.getByText("Request cancelled.").first()).toBeVisible();
}

test("a request is validated, sent with three preferred times, noted and cancelled", async ({ page }) => {
  let jobId: string | null = null;
  try {
    await page.goto("/customer/maintenance");
    await page.getByRole("button", { name: "New maintenance request" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Submit request" }).click();
    // no AC is chosen for the user (UI/UX loop R8): the unit is the first field to fix
    await expect(dialog).toContainText("Choose the AC that needs service");
    await expect(dialog.getByRole("combobox", { name: /^Unit/ })).toBeFocused();
    await dialog.getByRole("combobox", { name: /^Unit/ }).selectOption({ index: 1 });
    await dialog.getByRole("button", { name: "Submit request" }).click();
    await expect(dialog).toContainText("Symptoms must be 10–2000 characters");
    // a failed submit takes focus to the first field to fix and marks it invalid (IR573)
    await expect(dialog.getByLabel(/Symptoms/)).toBeFocused();
    await expect(dialog.getByLabel(/Symptoms/)).toHaveAttribute("aria-invalid", "true");
    await dialog.getByLabel(/Symptoms/).fill("E2E: the unit makes a rattling noise at high fan speed.");
    await dialog.getByLabel(/Contact window/).fill("call 0123456789");
    await dialog.getByRole("button", { name: "Submit request" }).click();
    await expect(dialog).toContainText("No e-mail addresses or phone numbers");
    await expect(dialog.getByLabel(/Contact window/)).toBeFocused();
    await expect(dialog.getByLabel(/Symptoms/)).not.toHaveAttribute("aria-invalid");
    await dialog.getByLabel(/Contact window/).fill("Weekdays 09:00-18:00");
    await dialog.getByRole("button", { name: "Submit request" }).click();
    await page.waitForURL(/jobId=/);
    jobId = new URL(page.url()).searchParams.get("jobId");
    await expect(page.getByText(/sent with 3 preferred times/).first()).toBeVisible();
    await expect(page.getByRole("main")).toContainText("Your preferred times");

    await page.getByRole("button", { name: "Add note" }).click();
    await dialog.getByLabel(/Note to the HQ coordinator/).fill("E2E: the room is free after 15:00 on weekdays.");
    await dialog.getByRole("button", { name: "Send note" }).click();
    await expect(page.getByText("Note sent").first()).toBeVisible();
    await expect(page.getByRole("main")).toContainText("the room is free after 15:00");

    await page.getByRole("button", { name: "Cancel request" }).click();
    await dialog.getByRole("button", { name: "Cancel request" }).click();
    await expect(dialog).toContainText("A reason is required");
    await dialog.getByLabel(/Reason/).fill("E2E run: no visit needed.");
    await dialog.getByRole("button", { name: "Cancel request" }).click();
    await expect(page.getByText("Request cancelled.").first()).toBeVisible();
  } finally {
    if (jobId) await cancelRequest(page, jobId, "E2E run: cleaning up.");
  }
});
