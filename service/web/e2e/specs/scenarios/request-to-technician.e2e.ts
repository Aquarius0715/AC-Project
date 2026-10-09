// Across the four apps (FR-C09 → FR-A06 → FR-P02 / FR-P03 → FR-T01; IR113, IR124, IR225–IR232): a customer's request is
// offered by HQ to contractor-a at the customer's first preferred time; the partner accepts it and gives it to
// tech-external-a, who then has it to accept. HQ cancels it with a reason at the end, so a run leaves one cancelled job.
import type { Browser, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { APPS, type App } from "../../fixtures/apps";
import { signIn } from "../../fixtures/auth";

const app = (id: App["id"]) => APPS.find((a) => a.id === id)!;
const open = async (browser: Browser, a: App, signedIn = true): Promise<Page> =>
  (await browser.newContext({ baseURL: a.url, viewport: { width: 1920, height: 1080 }, ...(signedIn ? { storageState: a.state } : {}) })).newPage();

test("a customer request reaches the partner's technician", async ({ browser }) => {
  test.setTimeout(240_000);
  const customer = await open(browser, app("customer"));
  const admin = await open(browser, app("admin"));
  let jobId: string | null = null;
  try {
    await test.step("the customer asks for a visit", async () => {
      await customer.goto("/customer/maintenance");
      await customer.getByRole("button", { name: "+ New maintenance request" }).click();
      const dialog = customer.getByRole("dialog");
      await dialog.getByLabel(/Symptoms/).fill("E2E scenario: water drips from the indoor unit.");
      await dialog.getByLabel(/Contact window/).fill("Weekdays 09:00-18:00");
      await dialog.getByRole("button", { name: "Submit request" }).click();
      await customer.waitForURL(/jobId=/);
      jobId = new URL(customer.url()).searchParams.get("jobId");
      expect(jobId).toMatch(/^[0-9a-f-]{36}$/);
    });

    await test.step("HQ offers the first preferred time to contractor-a", async () => {
      await admin.goto(`/admin/jobs?jobId=${jobId}`);
      await admin.getByRole("button", { name: "Use this time" }).first().click();
      const dialog = admin.getByRole("dialog");
      await dialog.getByRole("button", { name: "Offer to contractor" }).click();
      const org = dialog.getByLabel("Contractor organization");
      await org.selectOption((await org.locator("option", { hasText: /Contractor A/ }).first().getAttribute("value"))!);
      await dialog.getByRole("button", { name: "Send offer" }).click();
      await expect(admin.getByText(/They accept, decline or propose/)).toBeVisible();
    });

    const partner = await open(browser, app("partner"));
    await test.step("the partner accepts the offer", async () => {
      await partner.goto(`/partner/jobs/${jobId}`);
      await expect(partner.getByRole("main")).toContainText("Offer from HQ");
      await partner.getByRole("button", { name: "Accept job" }).click();
      await expect(partner.getByText("Acceptance does not assign a technician")).toBeVisible();
    });

    await test.step("and gives it to tech-external-a", async () => {
      await partner.goto(`/partner/schedule?jobId=${jobId}`);
      await partner.getByRole("radiogroup", { name: "Technician" }).getByRole("radio", { name: /tech-external-a/ }).click();
      await partner.getByRole("button", { name: "Confirm assignment" }).click();
      await expect(partner.getByText("Technician assigned — waiting for acceptance")).toBeVisible();
    });

    await test.step("tech-external-a has it to accept", async () => {
      const tech = await open(browser, app("technician"), false);
      await signIn(tech, app("technician"), "tech-external-a");
      await expect(tech.locator(`main a[href="/technician/jobs/${jobId}"]`).first()).toBeVisible();
      await tech.locator(`main a[href="/technician/jobs/${jobId}"]`).first().click();
      await expect(tech.getByRole("main")).not.toContainText("This page isn’t available");
    });
  } finally {
    if (jobId) {
      await admin.goto(`/admin/jobs?jobId=${jobId}`);
      await admin.getByRole("button", { name: "Cancel…" }).click();
      await admin.getByLabel(/Reason · required/).fill("E2E scenario finished.");
      await admin.getByRole("button", { name: "Confirm" }).click();
      await expect(admin.getByText("Job cancelled.").first()).toBeVisible();
    }
  }
});
