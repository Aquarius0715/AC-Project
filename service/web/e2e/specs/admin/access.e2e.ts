// HQ access & roles (FR-A03, DD-A03, SCR-A03, Figma Admin 539:16956, IR321): the organization filter is a
// search-select in the URL (organizationId) beside the role and status filters, and a membershipId the list does not
// hold says so instead of opening another membership. Nothing is saved, except by the last two tests: one signs
// hq-override-only out everywhere (IR384, IR385), which is no project's shared user; the other adds a new person and
// revokes the membership again (IR461).
import { test, expect } from "../../fixtures/test";

test("the organization filter narrows the memberships; an unknown membership is not opened", async ({ page }) => {
  await page.goto("/admin/settings/access");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "Memberships", exact: true })).toBeVisible();

  // the search-select: typing narrows the options, a click chooses one
  const org = main.getByRole("combobox", { name: "Filter by organization" });
  await expect(org).toHaveValue("Organization: All");
  await org.click();
  await org.fill("contractor a");
  const list = page.getByRole("listbox", { name: "Filter by organization" });
  await expect(list.getByRole("option")).toHaveCount(1);
  await list.getByRole("option", { name: "Demo Contractor A" }).click();
  await page.waitForURL(/organizationId=[0-9a-f-]{36}/);
  await expect(org).toHaveValue("Demo Contractor A");
  const rows = main.locator("button[aria-pressed]");
  await expect(rows.first()).toBeVisible();
  for (const label of await rows.locator("div.text-\\[11px\\]").allInnerTexts()) expect(label).toContain("Demo Contractor A");
  // Escape closes the list and keeps the choice
  await org.click();
  await org.press("Escape");
  await expect(list).toHaveCount(0);
  await expect(org).toHaveValue("Demo Contractor A");

  // the keyboard: typing narrows, Enter takes the active (first) match
  const before = new URL(page.url()).searchParams.get("organizationId");
  await org.click();
  await org.fill("demo hq");
  await org.press("ArrowDown"); // stays on the only match
  await org.press("Enter");
  await page.waitForURL((u) => !!u.searchParams.get("organizationId") && u.searchParams.get("organizationId") !== before);
  await expect(org).toHaveValue("Demo HQ");
  for (const label of await rows.locator("div.text-\\[11px\\]").allInnerTexts()) expect(label).toContain("Demo HQ");

  // a membership the list does not hold: no other membership opens in its place
  await page.goto("/admin/settings/access?role=admin&membershipId=00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "That membership is not in this list" })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Identity & role" })).toHaveCount(0);
  await main.getByRole("button", { name: "Clear the filters" }).click();
  await page.waitForURL((u) => !u.searchParams.has("role") && u.searchParams.get("membershipId") === "00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "That membership is not in this list" })).toBeVisible();
});

// Figma Admin 03-2 → 03-8 (IR384, IR385): Sign out everywhere asks for a reason, then ends every session of the user
test("Sign out everywhere asks for a reason and ends the member's sessions", async ({ page }) => {
  await page.goto("/admin/settings/access");
  const main = page.getByRole("main");
  await main.locator("button[aria-pressed]", { hasText: "hq-override-only" }).first().click();
  await page.waitForURL(/membershipId=[0-9a-f-]{36}/);
  await main.getByRole("button", { name: "Sign out everywhere…" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(/Sign out .+ everywhere\?/);
  const confirm = dialog.getByRole("button", { name: "Sign out everywhere", exact: true });
  await expect(confirm).toBeDisabled(); // a reason first
  await dialog.getByRole("textbox").fill("E2E: laptop reported lost");
  await confirm.click();
  await expect(page.getByText(/Signed out everywhere — every session of .+ has ended/).first()).toBeVisible();
  await expect(dialog).toHaveCount(0);
});

// IR461: a membership for a new person — the account is created at once and the sign-in service e-mails the set-up
// link, which reaches Mailpit — then revoking ends the membership again, turns the sign-in off and lists it as Revoked
// (IR508).
test("HQ adds a new person with a membership, and revoking it ends the membership", async ({ page }) => {
  await page.goto("/admin/settings/access");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "New membership" }).click();
  await page.waitForURL(/membershipId=new/);
  await main.getByRole("button", { name: "New person" }).click();
  const email = `e2e-hq-${Date.now()}@example.com`;
  await main.getByRole("textbox", { name: /^Name/ }).fill("E2E Colleague");
  await main.getByRole("textbox", { name: /^Email/ }).fill(email);
  await main.getByRole("button", { name: "Admin", exact: true }).last().click(); // the role, after the role filter
  await main.getByRole("combobox", { name: /^Organization/ }).selectOption({ label: "Demo HQ" });
  await main.getByLabel("Valid from").fill("2026-09-01T09:00"); // before the seed's demo clock
  await main.getByRole("textbox", { name: /^Change reason/ }).fill("E2E: a new colleague");
  await main.getByRole("button", { name: "Create membership" }).click();
  await expect(page.getByText("Membership created").first()).toBeVisible();
  await page.waitForURL(/membershipId=[0-9a-f-]{36}/);
  await expect(main.locator('button[aria-pressed="true"]', { hasText: "E2E Colleague" })).toBeVisible(); // the new one is selected (an earlier run's stays listed, revoked)
  const search = `http://localhost:8025/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`;
  if ((await fetch(search).catch(() => null))?.ok) {
    await expect.poll(async () => ((await (await fetch(search)).json()) as { messages_count: number }).messages_count, { timeout: 30_000 }).toBeGreaterThanOrEqual(1);
  }
  await main.getByRole("button", { name: "Revoke access…" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("turns their sign-in off. Only HQ can give the access back");
  await dialog.getByRole("textbox", { name: /^Reason/ }).fill("E2E: not joining after all");
  await dialog.getByRole("button", { name: "Revoke", exact: true }).click();
  await expect(page.getByText("Access revoked — the membership is now inactive").first()).toBeVisible();
  await expect(main.locator('button[aria-pressed="true"]', { hasText: "E2E Colleague" })).toContainText("Revoked");
  await expect(main).toContainText("the sign-in is off, and their company or customer cannot extend or enable it again");
});
