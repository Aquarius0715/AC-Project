// The notification inbox (FR-X07, Figma Client 10c, IR351): "Mark all as read" reads every listed unread notification,
// each at its version, and then hides itself; reading never resolves an alert. Like every shared spec it runs in each
// app's project as that project's user. With nothing unread the button is not offered.
import { test, expect } from "../../fixtures/test";

test("Mark all as read empties the Unread tab", async ({ page }) => {
  await page.goto("/notifications");
  const main = page.getByRole("main");
  await expect(main).toContainText("Reading a notification is separate from resolving the linked alert."); // no requirement IDs (IR580)
  const unreadTab = main.getByRole("tab", { name: /^Unread/ });
  const button = main.getByRole("button", { name: "Mark all as read" });
  const unread = Number((/\d+/.exec((await unreadTab.innerText())))?.[0] ?? "0");
  // The sidebar's Notifications badge counts every unread notification of the member (no badge for 0), while the page
  // lists the newest 100 only: the two agree when the whole inbox is listed.
  const listed = Number((/\d+/.exec(await main.getByRole("tab", { name: /^All/ }).innerText()))?.[0] ?? "0");
  const sidebarLink = page.getByRole("link", { name: /^Notifications/ });
  const badgeIs = async (n: number) => { await expect(sidebarLink).toHaveText(n === 0 ? /^Notifications$/ : new RegExp(String.raw`^Notifications\s*${String(n)}$`)); };
  if (unread === 0) {
    await expect(button).toHaveCount(0);
    if (listed < 100) await badgeIs(0);
    return;
  }
  if (listed < 100) await badgeIs(unread);
  await button.click();
  await expect(page.getByText(/^\d+ notifications? marked as read$|^1 notification marked as read$/)).toBeVisible();
  await expect(unreadTab).toHaveText(/Unread\s*0/);
  if (listed < 100) await badgeIs(0); // the layout was rendered again by the action's refresh()
  await expect(button).toHaveCount(0);
  await unreadTab.click();
  await expect(main).toContainText("No unread notifications");
});
