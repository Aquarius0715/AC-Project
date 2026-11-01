// Diagnostic control (FR-T10, IR590): a test run of 20 minutes without a reason is refused in words under its fields —
// no error code — and focus moves to the duration, the first field to fix. Nothing is sent. It needs tech-external-a
// inside the seed job's work window (14–20 Sept, the CI seed's clock); a dev stack whose clock has passed it skips.
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";

const BEDROOM_AC = "614da6ca-9ebc-5075-badc-9eec9eefd802";
const SEED_JOB = "41de64d5-73e5-52a7-bdd1-7c6c2cf13cf6";

test("a refused test run names its fields and focuses the duration", async ({ browser, app }) => {
  const ctx = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] }, viewport: { width: 390, height: 844 } });
  const page = await ctx.newPage();
  try {
    await signIn(page, app, "tech-external-a");
    await page.goto(`/technician/units/${BEDROOM_AC}/control?jobId=${SEED_JOB}`);
    const main = page.getByRole("main");
    const heading = main.getByRole("heading", { name: "Diagnostic control" });
    await expect(heading.or(main.getByText("This page isn’t available")).first()).toBeVisible();
    test.skip(!(await heading.isVisible()), "the seed job's work window has passed on this stack");
    const duration = main.getByRole("textbox", { name: /^Duration \(min\)/ });
    await duration.fill("20");
    await main.getByRole("button", { name: /^Start test run/ }).click();
    await expect(main).toContainText("A test run lasts 1–15 minutes");
    await expect(main).not.toContainText("VALIDATION");
    await expect(duration).toBeFocused();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  } finally {
    await ctx.close();
  }
});
