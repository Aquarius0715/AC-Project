// Deleting what is still in use (FR-A02, IR591): a room that still holds a unit cannot be deleted and says so in
// words; a unit with an unexpired contract and a bound device lists both, and neither "Delete unit" nor "Remove
// permanently" can be pressed. The seed's Meeting room holds Meeting room AC, and Lobby AC has its RTO contract and
// AC-DEMO-0004. Nothing is deleted.
import { test, expect } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

const CUSTOMER_A = "d67d153b-fa85-57be-aa40-c8aa09e3d02c"; // the customer record of Demo Customer A
const MEETING_ROOM = "971554c4-e36b-5d0b-a8fe-c61445c649c5";
const LOBBY_AC = "55ce3ac7-a38b-54f3-84d0-f2e9932adf81";

test("a room with a unit and a unit in use cannot be deleted, and say why in words", async ({ page }) => {
  const main = page.getByRole("main");
  await page.goto(`/admin/units?customerId=${CUSTOMER_A}&locationId=${MEETING_ROOM}`);
  await main.getByRole("button", { name: "Delete", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Meeting room still contains 1 unit. Move or delete them first (unit › Location).");
  await expect(dialog.getByRole("button", { name: "Delete room" })).toBeDisabled();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);

  await page.goto(`/admin/units?customerId=${CUSTOMER_A}&unitId=${LOBBY_AC}`);
  await main.getByRole("button", { name: "Delete unit…" }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("This unit is still in use");
  await expect(dialog).toContainText(/Unexpired contract/);
  await expect(dialog).not.toContainText(/CONFLICT|IR\d+|unit_archived|units\.delete/);
  await expect(dialog.getByRole("button", { name: "Delete unit" })).toBeDisabled();
  await dialog.getByText("Registered by mistake?").click();
  await expect(dialog.getByRole("button", { name: "Remove permanently" })).toBeDisabled();
  await expectAccessible(page, '[role="dialog"]');
  await dialog.getByRole("button", { name: "Close" }).last().click();
  await expect(dialog).toHaveCount(0);
});
