// Across the four apps (FR-C09 → FR-A06 → FR-P02 / FR-P03 / FR-P04 → FR-T01; IR113, IR124, IR225–IR232, IR280): a
// customer's request is offered by HQ to contractor-a at the customer's first preferred time; the partner accepts it,
// gives it to tech-external-a and opens the unit's diagnosis view; the technician accepts the assignment (受領), after
// which the partner sees it accepted and the customer the technician's name. HQ cancels it with a reason at the end,
// so a run leaves one cancelled job. Starting the work needs the visit window, which only a jump of the
// never-returning demo clock would reach, so the scenario stops here. The seed keeps tech-external-a on its own job
// until 20 Sept (KL), so a visit before that would find them busy: the preferred times move after it (IR316).
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
      if ((await dialog.getByLabel("Date 1", { exact: true }).inputValue()) <= "2026-09-20") {
        for (const [n, date] of [[1, "2026-09-21"], [2, "2026-09-22"], [3, "2026-09-23"]] as const) await dialog.getByLabel(`Date ${n}`, { exact: true }).fill(date);
      }
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

    await test.step("the partner opens the unit — diagnosis only (FR-P04, IR280)", async () => {
      await partner.goto(`/partner/jobs/${jobId}`);
      await partner.getByRole("link", { name: "Open unit →" }).first().click();
      await partner.waitForURL(/\/partner\/units\/[^/?]+\?jobId=/);
      const main = partner.getByRole("main");
      for (const name of ["Unit register", "Job context", "Alert evidence", "Readings (diagnosis only)"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
      await expect(main).toContainText(`Diagnosis-scoped view for ${jobId!.slice(0, 8)}`);
      await expect(main).toContainText("Read-only — no remote-control actions available to contractors.");
      await expect(main.getByRole("button", { name: /power|temperature/i })).toHaveCount(0); // no control
    });

    await test.step("tech-external-a accepts the assignment", async () => {
      const tech = await open(browser, app("technician"), false);
      await signIn(tech, app("technician"), "tech-external-a");
      await expect(tech.locator(`main a[href="/technician/jobs/${jobId}"]`).first()).toBeVisible();
      await tech.locator(`main a[href="/technician/jobs/${jobId}"]`).first().click();
      await expect(tech.getByRole("main")).not.toContainText("This page isn’t available");
      await tech.getByRole("button", { name: "✓ Accept assignment" }).click();
      await expect(tech.getByText("Assignment accepted").first()).toBeVisible();
      await expect(tech.getByRole("button", { name: "✓ Accept assignment" })).toHaveCount(0);
    });

    await test.step("the partner and the customer see it accepted", async () => {
      await partner.goto(`/partner/schedule?jobId=${jobId}`);
      await expect(partner.getByRole("main")).toContainText("The current technician accepted the assignment ✓");
      await customer.goto(`/customer/maintenance?jobId=${jobId}`);
      await expect(customer.getByRole("main")).toContainText("tech-external-a");
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
