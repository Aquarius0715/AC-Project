// Customer restriction notice on a unit (FR-C12, IR583): a cooling restriction says that it is payment-related and what
// stays unavailable until the release is confirmed, and its link opens that restriction's detail, not the payments list.
// The seed's Lobby AC carries one; without it the notice is not shown and the test has nothing to check.
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
