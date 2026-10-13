// Partner Users (FR-P11, IR416, IR418, IR423): contractor-a invites a technician (the invitation e-mail reaches
// Mailpit), is told to wait before resending it, edits the invite, removes it, and is refused when it disables its booked technician; its own row has no
// actions.
import { test, expect, FAILED_PAGE } from "../../fixtures/test";

test("a contractor invites, edits and removes a user, waits to resend, and cannot disable its booked technician", async ({ page, app }) => {
  test.skip(app.id !== "partner", "partner only");
  await page.goto("/partner/users");
  await expect(page.locator("body")).not.toContainText(FAILED_PAGE);
  const row = (text: string) => page.getByRole("row").filter({ hasText: text });
  await expect(row("contractor-a (you)")).toContainText("Your account");
  const email = `e2e-${Date.now()}@example.com`;
  await page.getByRole("button", { name: "Invite user" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("technician");
  await dialog.getByRole("button", { name: "Send invite" }).click();
  await expect(page.getByText(`Invitation e-mail queued for ${email}`).first()).toBeVisible();
  await expect(row(email)).toContainText("Invite pending");
  await row(email).getByRole("button", { name: "Resend invite" }).click();
  // one invitation e-mail per 5 minutes (IR423): the resend right after the invite is refused with the wait
  await expect(page.getByText("The invitation was e-mailed a moment ago — send it again in 5 min").first()).toBeVisible();
  // the notification worker sends it to the local Mailpit inbox (IR418); skipped on a stack without Mailpit
  const search = `http://localhost:8025/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  if ((await fetch(search).catch(() => null))?.ok) {
    await expect.poll(async () => ((await (await fetch(search)).json()) as { messages_count: number }).messages_count, { timeout: 30_000 }).toBe(1);
  }

  // a pending invite's role can change, with a reason
  await row(email).getByRole("button", { name: "Edit" }).click();
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("contractor");
  await dialog.getByRole("button", { name: "Save invite" }).click();
  await expect(dialog.getByText("A reason of 1–1000 characters")).toBeVisible(); // no reason yet
  await dialog.getByLabel("Reason").fill("Will run the second crew");
  await dialog.getByRole("button", { name: "Save invite" }).click();
  await expect(row(email)).toContainText("Contractor");

  await row(email).getByRole("button", { name: "Remove" }).click();
  await dialog.getByLabel("Reason").fill("Sent to the wrong address");
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(row(email)).toHaveCount(0);

  // the technician holds an assigned job: disabling is refused with what to do, and it stays active (IR421)
  const tech = row("tech-external-a");
  await tech.getByRole("button", { name: "Disable" }).click();
  await dialog.getByLabel("Reason").fill("On leave");
  await dialog.getByRole("button", { name: "Disable" }).click();
  await expect(page.getByText("This technician still has an assigned job — reassign it first").first()).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(tech).toContainText("Active");
});
