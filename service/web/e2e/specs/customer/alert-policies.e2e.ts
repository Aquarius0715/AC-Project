// Customer alert policies on the Core API (FR-C15, DD-C15, IR243): an owner switches a default rule and back, and creates
// a policy (a recovery on the wrong side refused), edits it (threshold, weekdays only), switches it off and deletes it.
// Policies whose name starts with "E2E " are removed afterwards whatever happened.
import { test, expect } from "../../fixtures/test";
import type { Page } from "@playwright/test";

const NAME = `E2E too hot ${Date.now() % 100_000}`;

async function deleteE2EPolicies(page: Page) {
  await page.goto("/customer/alerts?tab=policies");
  for (let i = 0; i < 5; i++) {
    const card = page.locator("main section", { has: page.getByRole("switch", { name: /^E2E / }) }).first();
    if ((await page.getByRole("switch", { name: /^E2E / }).count()) === 0) return;
    await card.getByRole("button", { name: "Delete" }).first().click();
    await page.getByRole("dialog").getByRole("button", { name: "Delete policy" }).click();
    await expect(page.getByText(/deleted/).last()).toBeVisible();
    await page.reload();
  }
}

test.afterEach(async ({ page }) => deleteE2EPolicies(page));

test("an owner switches a default rule and back", async ({ page }) => {
  await page.goto("/customer/alerts?tab=policies");
  const rule = page.getByRole("switch", { name: "AC offline" });
  const before = (await rule.getAttribute("aria-checked")) === "true";
  for (const want of [!before, before]) {
    await rule.click();
    await expect(page.getByText(`Default rule “AC offline” switched ${want ? "on" : "off"}`)).toBeVisible();
    await expect(rule).toHaveAttribute("aria-checked", String(want));
  }
  await expect(page.getByRole("main")).toContainText(/\d of 6 rules on/);
});

test("a policy is created, edited, switched off and deleted", async ({ page }) => {
  await page.goto("/customer/alerts?tab=policies");
  await page.getByRole("link", { name: "+ Create policy" }).click();
  await page.waitForURL(/policyId=new/);
  await page.getByLabel("Name").fill(NAME);
  await page.getByLabel("Threshold").fill("30");
  await page.getByLabel("Recovery").fill("31"); // above the threshold: not the safe side for "above"
  await page.getByRole("button", { name: "Save policy" }).click();
  await expect(page.getByRole("main")).toContainText("✕");
  await page.getByLabel("Recovery").fill("28");
  await page.getByLabel("Email").check();
  await page.getByRole("button", { name: "Save policy" }).click();
  await expect(page.getByText(`“${NAME}” saved`)).toBeVisible();
  await expect(page.getByRole("switch", { name: NAME })).toHaveAttribute("aria-checked", "true");

  const card = page.locator("main section", { has: page.getByRole("switch", { name: NAME }) }); // each policy is a Card
  await card.getByRole("link", { name: "Edit" }).click();
  await page.waitForURL(/policyId=[0-9a-f-]{36}/);
  await page.getByLabel("Threshold").fill("29");
  await page.getByLabel("Only on some days and times").check();
  await page.getByRole("button", { name: "Mon–Fri" }).click();
  await expect(page.getByRole("main")).toContainText(/29/);
  await page.getByRole("button", { name: "Save policy" }).click();
  await expect(page.getByText(`“${NAME}” saved`)).toBeVisible();
  await expect(card).toContainText(/29/);

  await page.getByRole("switch", { name: NAME }).click();
  await expect(page.getByText(`“${NAME}” switched off`)).toBeVisible();
  await expect(page.getByRole("switch", { name: NAME })).toHaveAttribute("aria-checked", "false");

  await card.getByRole("button", { name: "Delete" }).first().click();
  await page.getByRole("dialog").getByRole("button", { name: "Delete policy" }).click();
  await expect(page.getByText(`“${NAME}” deleted`)).toBeVisible();
  await expect(page.getByRole("switch", { name: NAME })).toHaveCount(0);
});
