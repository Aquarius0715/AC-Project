// The technician's work report saves itself 20 s after the last edit while the work window is open (DD-T04, IR89): the
// spec checks in to an assigned job, records one result, and waits for the autosave request. It must not come before the
// 20 s, and it comes once. This changes the dev database (the job is in progress afterwards): reset the seed before the
// next full run (memory dev-demo-clock).
import { test, expect } from "../../fixtures/test";
import { APPS } from "../../fixtures/apps";
import { signIn } from "../../fixtures/auth";

test.describe.configure({ timeout: 180_000 });

test("the report autosaves once, 20 s after the last edit", async ({ browser }) => {
  const app = APPS.find((a) => a.id === "technician")!;
  const ctx = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] }, viewport: { width: 1920, height: 1080 } });
  const page = await ctx.newPage();
  try {
    await signIn(page, app, "tech-external-a");
    // the seed's job-contractor-a (assigned to tech-external-a, a checklist with results), not whichever job the list
    // shows first: other specs change the list's order
    await page.goto("/technician/jobs/41de64d5-73e5-52a7-bdd1-7c6c2cf13cf6");
    const main = page.locator("main");
    // a job already checked in (an earlier run) has no start button: the report is open either way
    const start = main.getByRole("button", { name: /^Start job/ });
    // wait until the page shows one or the other (count() does not wait: right after the navigation it read 0)
    await expect(start.or(main.locator("select").first()).first()).toBeVisible();
    if (await start.count()) {
      await start.first().click();
      const checkIn = page.getByRole("dialog", { name: "Check in at site" });
      // the demo location service and the camera scan fill these a moment after the dialog opens: checking in before
      // that is refused (a fast CI runner did)
      await expect(checkIn.getByLabel("Distance to the site (m)")).toHaveValue(/\d/);
      await expect(checkIn.getByLabel("Scanned unit QR")).not.toHaveValue("");
      await checkIn.getByRole("button", { name: "Check in & start" }).click();
      await expect(page.getByText("Checked in — the job has started").first()).toBeVisible();
    }
    const result = main.locator("select").first();
    await expect(result).toBeVisible({ timeout: 30_000 });
    const saves: number[] = [];
    page.on("request", (r) => { if (r.method() === "POST" && new URL(r.url()).pathname.startsWith("/technician/jobs/")) saves.push(Date.now()); });
    const edited = Date.now();
    await result.selectOption({ label: "Normal" });
    await page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname.startsWith("/technician/jobs/") && r.request().postData() !== null, { timeout: 60_000 });
    const firstSave = saves[0];
    expect(firstSave - edited).toBeGreaterThanOrEqual(18_000);
    await page.waitForTimeout(3000);
    expect(saves.length).toBe(1);
  } finally {
    await ctx.close();
  }
});
