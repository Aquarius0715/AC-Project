// Partner Users (FR-P11, IR416, IR423, IR432, IR461, IR462): contractor-a adds a technician (the account is created at once
// and the sign-in service e-mails the set-up link, which reaches Mailpit), is told to wait before resending it, moves
// the technician's access end and removes the account; its booked technician offers no Disable or Remove, and its own
// row has no actions.
import { test, expect, FAILED_PAGE } from "../../fixtures/test";
import { expectAccessible } from "../../fixtures/a11y";

test("a contractor adds a technician, waits to resend the set-up e-mail, moves the access end and removes them, and cannot disable its booked technician", async ({ page, app }) => {
  test.skip(app.id !== "partner", "partner only");
  await page.goto("/partner/users");
  await expect(page.locator("body")).not.toContainText(FAILED_PAGE);
  const row = (text: string) => page.getByRole("row").filter({ hasText: text });
  await expect(row("contractor-a (you)")).toContainText("Your account");
  const email = `e2e-${Date.now()}@example.com`;
  await page.getByRole("button", { name: "Add user" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(dialog.getByText("A name of 1–120 characters")).toBeVisible(); // nothing typed yet
  await dialog.getByLabel("Name").fill("E2E Technician");
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("technician");
  await expect(dialog.getByLabel("Access until")).not.toHaveValue(""); // three months ahead is offered
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(page.getByText(`E2E Technician added — a set-up e-mail is on its way to ${email}`).first()).toBeVisible();
  await expect(row(email)).toContainText("Not signed in yet");
  await row(email).getByRole("button", { name: "Resend set-up e-mail" }).click();
  // one e-mail per 5 minutes (IR423): the resend right after the adding is refused with the wait
  await expect(page.getByText("The set-up e-mail was sent a moment ago — send it again in 5 min").first()).toBeVisible();
  // the sign-in service sends the set-up e-mail to the local Mailpit inbox (IR461); skipped on a stack without Mailpit
  const search = `http://localhost:8025/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  if ((await fetch(search).catch(() => null))?.ok) {
    await expect.poll(async () => ((await (await fetch(search)).json()) as { messages_count: number }).messages_count, { timeout: 30_000 }).toBeGreaterThanOrEqual(1);
  }

  // the access end moves, with a reason
  await row(email).getByRole("button", { name: "Access" }).click();
  await expectAccessible(page, '[role="dialog"]'); // the access end dialog (IR360, IR461)
  await dialog.getByLabel("Access until").fill("2026-12-31");
  await dialog.getByRole("button", { name: "Save access" }).click();
  await expect(dialog.getByText("A reason of 1–1000 characters")).toBeVisible(); // no reason yet
  await dialog.getByLabel("Reason").fill("Contract until year end");
  await dialog.getByRole("button", { name: "Save access" }).click();
  await expect(row(email)).toContainText("31 Dec 2026");

  await row(email).getByRole("button", { name: "Remove" }).click();
  await dialog.getByLabel("Reason").fill("Should have been a contractor");
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(row(email)).toHaveCount(0);

  // the same address again, with the right role: the same person comes back with a new account set-up (IR462)
  await page.getByRole("button", { name: "Add user" }).click();
  await dialog.getByLabel("Name").fill("E2E Contractor");
  await dialog.getByLabel("Email").fill(email);
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("contractor");
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(page.getByText(`E2E Contractor added — a set-up e-mail is on its way to ${email}`).first()).toBeVisible();
  await expect(row(email)).toContainText("Contractor");
  await expect(row(email)).toContainText("Not signed in yet");
  if ((await fetch(search).catch(() => null))?.ok) {
    await expect.poll(async () => ((await (await fetch(search)).json()) as { messages_count: number }).messages_count, { timeout: 30_000 }).toBeGreaterThanOrEqual(2);
  }
  await row(email).getByRole("button", { name: "Remove" }).click();
  await dialog.getByLabel("Reason").fill("Not joining after all");
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(row(email)).toHaveCount(0);

  // the technician holds an assigned job: Disable and Remove are not offered, the row says what to do, and it stays
  // active (IR421, IR432)
  const tech = row("tech-external-a");
  await expect(tech).toContainText("Has an assigned job — reassign it first");
  await expect(tech.getByRole("button", { name: "Disable" })).toHaveCount(0);
  await expect(tech.getByRole("button", { name: "Remove" })).toHaveCount(0);
  await expect(tech).toContainText("Active");
});

// IR461: a technician's access ends at most a year ahead; the form says so before anything is sent.
test("a contractor cannot give a technician access for more than a year", async ({ page, app }) => {
  test.skip(app.id !== "partner", "partner only");
  await page.goto("/partner/users");
  await page.getByRole("button", { name: "Add user" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Name").fill("Too Long");
  await dialog.getByLabel("Email").fill(`e2e-long-${Date.now()}@example.com`);
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("technician");
  await expectAccessible(page, '[role="dialog"]'); // the add dialog with the access end (IR360, IR461)
  await dialog.getByLabel("Access until").fill("2030-01-01");
  await dialog.getByRole("button", { name: "Add user" }).click();
  await expect(dialog.getByText("At most a year ahead")).toBeVisible();
  // a contractor account has no access end
  await dialog.getByRole("combobox", { name: "Role" }).selectOption("contractor");
  await expect(dialog.getByLabel("Access until")).toHaveCount(0);
  await dialog.getByRole("button", { name: "Cancel" }).click();
});
