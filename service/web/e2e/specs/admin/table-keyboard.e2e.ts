// IR294: a table row that opens a record (DataTable onRowClick) is reached and opened by keyboard as well as by a
// click — focus the register's customer row and press Enter, then a unit row of that customer and press Space.
// Nothing is changed.
import { test, expect } from "../../fixtures/test";

test("table rows that open a record work with Enter and Space", async ({ page }) => {
  await page.goto("/admin/units");
  const main = page.getByRole("main");
  const customer = main.getByRole("row", { name: /Demo Customer A/ });
  await customer.focus();
  await expect(customer).toBeFocused(); // the row takes the focus (tabIndex 0)
  await page.keyboard.press("Enter");
  await page.waitForURL(/customerId=/);
  const unit = main.getByRole("row", { name: /Bedroom AC/ });
  await unit.focus();
  await expect(unit).toBeFocused();
  await page.keyboard.press(" ");
  await page.waitForURL(/unitId=/);
  await expect(main.getByRole("button", { name: /‹ back to / })).toBeVisible();
});
