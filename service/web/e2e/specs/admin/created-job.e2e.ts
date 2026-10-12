// A job the new-job dialog just created opens with its result: ?jobId=<id>&created=requested says it was saved as
// requested (the dialog opened from the SLA or Contractors tabs hands it over in the URL, not in module state). The
// result shows once; the parameter is dropped when the detail takes it, so a reload does not repeat it.
import { test, expect } from "../../fixtures/test";

test("a job created as requested shows its result, once, from the URL", async ({ page }) => {
  await page.goto("/admin/jobs");
  const link = page.locator('a[href*="jobId="]').first();
  await expect(link).toBeVisible();
  const jobId = new URL((await link.getAttribute("href")) ?? "", page.url()).searchParams.get("jobId");
  expect(jobId).toBeTruthy();
  await page.goto(`/admin/jobs?jobId=${jobId}&created=requested`);
  await expect(page.getByText(/Job created as requested/).first()).toBeVisible();
  await page.waitForURL((u) => !u.searchParams.has("created"));
  await expect(page.getByText(/Job created as requested/).first()).toBeVisible();
  await page.reload();
  await expect(page.getByText(/Job created as requested/)).toHaveCount(0);
});
