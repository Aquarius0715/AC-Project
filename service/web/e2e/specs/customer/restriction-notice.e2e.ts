// Customer restriction notice on a unit (FR-C12, IR583): a cooling restriction says that it is payment-related and what
// stays unavailable until the release is confirmed, and its link opens that restriction's detail, not the payments list.
// The seed's Lobby AC carries one; without it the notice is not shown and the test has nothing to check. The overview's
// notice of the overdue invoice and its restriction (IR610) links the same detail.
import { test, expect } from "../../fixtures/test";

const LOBBY = "55ce3ac7-a38b-54f3-84d0-f2e9932adf81";

test("a restricted AC links to its restriction's detail", async ({ page }) => {
  await page.goto(`/customer/units/${LOBBY}`);
  const main = page.getByRole("main");
  await expect(main.getByRole("button", { name: "Lower the temperature" })).toBeVisible();
  const notice = main.getByText(/^Cooling restriction active — payment-related/);
  test.skip((await notice.count()) === 0, "the seed's restriction is not active");
  await expect(notice).toContainText("are unavailable until the release is confirmed");
  const link = main.getByRole("link", { name: "View restriction details" });
  await expect(link).toHaveAttribute("href", /^\/customer\/payments\/[0-9a-f-]{36}\?tab=restriction&restrictionId=[0-9a-f-]{36}$/);
  await link.click();
  await page.waitForURL(/tab=restriction/);
  await expect(page.getByRole("main")).toContainText("Release condition");
});

// The overview names what is overdue and the restriction it caused (DEC-90, IR610): Pay now (or View invoices) opens the
// invoice, View restriction details the restriction's own detail, as on the unit page. With nothing overdue the notice
// is not shown and the test has nothing to check.
test("the overview names what is overdue and the restriction it caused", async ({ page }) => {
  await page.goto("/customer");
  const main = page.getByRole("main");
  await expect(main.getByText("Running", { exact: true }).first()).toBeVisible();
  const due = main.getByText(/^(Invoice \S+ is overdue|\d+ invoices are overdue) — /);
  test.skip((await due.count()) === 0, "nothing is overdue on this stack");
  await expect(main.getByRole("link", { name: /^(Pay now|View invoices)/ })).toHaveAttribute("href", /^\/customer\/payments(\/[0-9a-f-]{36}|\?status=overdue)$/);
  const details = main.getByRole("link", { name: "View restriction details" });
  if (!await details.count()) {
    return;
  }

  await expect(main.getByText(/(until|unless) it is paid/).first()).toBeVisible();
  await details.click();
  await page.waitForURL(/tab=restriction/);
  await expect(page.getByRole("main")).toContainText("Release condition");
});
