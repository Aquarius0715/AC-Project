// The notification inbox (FR-X07, Figma Client 10c, IR351): "Mark all as read" reads every listed unread notification,
// each at its version, and then hides itself; reading never resolves an alert. Like every shared spec it runs in each
// app's project as that project's user. With nothing unread the button is not offered.
import { test, expect } from "../../fixtures/test";

test("Mark all as read empties the Unread tab", async ({ page }) => {
  await page.goto("/notifications");
  const main = page.getByRole("main");
  const unreadTab = main.getByRole("tab", { name: /^Unread/ });
  const button = main.getByRole("button", { name: "Mark all as read" });
  const unread = Number((await unreadTab.innerText()).match(/\d+/)?.[0] ?? "0");
  if (unread === 0) {
    await expect(button).toHaveCount(0);
    return;
  }
  await button.click();
  await expect(page.getByText(/^\d+ notifications? marked as read$|^1 notification marked as read$/)).toBeVisible();
  await expect(unreadTab).toHaveText(/Unread\s*0/);
  await expect(button).toHaveCount(0);
  await unreadTab.click();
  await expect(main).toContainText("No unread notifications");
});
