// The work report's checks (FR-T09, IR594): the workspace passes axe — its Next action controls are named — and a
// submit with empty checklist results is refused in words and takes focus to the first result to fix. Nothing is
// submitted. It needs tech-external-a inside the seed job's work window (the CI seed's clock); a dev stack whose clock
// has passed it skips. Checking in, when the job is not started yet, changes the job as report-autosave does.
import { test, expect } from "../../fixtures/test";
import { APPS } from "../../fixtures/apps";
import { signIn } from "../../fixtures/auth";
import { expectAccessible } from "../../fixtures/a11y";

test.describe.configure({ timeout: 180_000 });

test("an incomplete report names its issues in words and focuses the first one", async ({ browser }) => {
  const app = APPS.find((a) => a.id === "technician")!;
  const ctx = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] }, viewport: { width: 390, height: 844 } });
  const page = await ctx.newPage();
  try {
    await signIn(page, app, "tech-external-a");
    await page.goto("/technician/jobs/41de64d5-73e5-52a7-bdd1-7c6c2cf13cf6");
    const main = page.locator("main");
    const start = main.getByRole("button", { name: /^Start job/ });
    const result = main.getByRole("combobox", { name: /^Result of / }).first();
    const ended = main.getByText(/assignment on this job has ended|work window ended/i).first();
    await expect(start.or(result).or(ended).first()).toBeVisible();
    test.skip(await ended.isVisible(), "the seed job's work window has passed on this stack");
    if (await start.count()) {
      await start.first().click();
      const checkIn = page.getByRole("dialog", { name: "Check in at site" });
      await expect(checkIn.getByLabel("Distance to the site (m)")).toHaveValue(/\d/);
      await expect(checkIn.getByLabel("Scanned unit QR")).not.toHaveValue("");
      await checkIn.getByRole("button", { name: "Check in & start" }).click();
      await expect(page.getByText("Checked in — the job has started").first()).toBeVisible();
    }
    await expect(result).toBeVisible({ timeout: 30_000 });
    await expectAccessible(page);
    // an item without a result, so that the submit is refused (the report is never sent)
    await result.selectOption({ value: "" });
    await main.getByRole("button", { name: "Submit report" }).click();
    await expect(main).toContainText(/Not submitted — \d+ issues?:/);
    await expect(main).not.toContainText("VALIDATION");
    await expect(page.locator(":focus")).toHaveAttribute("aria-label", /^Result of /);
    await expect(page.getByRole("dialog")).toHaveCount(0);
  } finally {
    await ctx.close();
  }
});
