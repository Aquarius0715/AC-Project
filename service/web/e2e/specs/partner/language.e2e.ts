// AT-X01-N ③ for the contractor (FR-X01, FR-P01, FR-P02, IR44, IR270–IR272): with Malay and Asia/Tokyo saved in
// Preferences, the overview, the job list and a job's page speak Malay; the update time is in GMT+9, while the period
// and today's timeline stay Kuala Lumpur days and hours — the timeline says so. English and the earlier zone come back
// at the end, or in afterEach when the test fails.
import { test, expect } from "../../fixtures/test";
import { displayZone, ENGLISH, MALAY, setDisplay } from "../../fixtures/display";

let zoneBefore: string | null = null;
test.afterEach(async ({ page }) => {
  if (zoneBefore) await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
});

test("the contractor screens in Malay keep Kuala Lumpur days and show instants in the display zone", async ({ page }) => {
  const zone = await displayZone(page);
  zoneBefore = zone;
  await setDisplay(page, MALAY, "Asia/Tokyo");
  await page.goto("/partner");
  const main = page.getByRole("main");
  for (const name of [/^Kemajuan kerja — /, /^Perlu tindakan anda \(\d+\)$/, /^Kapasiti pasukan — /, /^Hari ini · /, "Aktiviti terkini"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main).toContainText(/Dikemas kini \d{1,2}:\d{2} (PG|PTG) GMT\+9 · kerja syarikat anda sahaja/);
  await expect(main).toContainText("Garis masa dalam waktu Kuala Lumpur (Asia/Kuala_Lumpur)."); // another display zone
  await expect(main.getByRole("combobox", { name: "Tempoh" })).toContainText("Minggu ini");
  await page.goto("/partner/jobs");
  await expect(main.getByRole("heading", { name: /^Kerja — syarikat anda sahaja · \d+ minggu ini$/ })).toBeVisible();
  await expect(page.getByRole("tab", { name: /^Semua/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("combobox", { name: "Isih" })).toContainText("Isih: Status ↑");
  await expect(main).toContainText(/Halaman \d+ daripada \d+/);
  const job = main.locator("a[href^='/partner/jobs/']").first(); // a job of the list: its offer, detail or the history it leads to
  if (await job.count()) {
    await job.click();
    await page.waitForURL(/\/partner\/(jobs\/[^/?]+|history|schedule)/);
    if (/\/partner\/jobs\/[^/?]+$/.test(new URL(page.url()).pathname)) await expect(main.getByRole("link", { name: "← Kerja" })).toBeVisible(); // the offer, detail or history page
  }
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/partner");
  await expect(page.getByRole("main").getByRole("heading", { name: /^Job progress — this week$/ })).toBeVisible();
});
