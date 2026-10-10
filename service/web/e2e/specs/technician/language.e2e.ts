// AT-X01-N ③ for the technician (FR-X01, FR-T01, FR-T04, IR44, IR281, IR282): with Malay and Asia/Tokyo saved in
// Preferences, the overview speaks Malay — tiles, tabs, sort and the side cards — while today's timeline stays in Kuala
// Lumpur hours and says so; an assigned job's workspace speaks Malay with its window in GMT+9. English and the earlier
// zone come back at the end, or in afterEach when the test fails.
import { test, expect } from "../../fixtures/test";
import { displayZone, ENGLISH, MALAY, setDisplay } from "../../fixtures/display";

let zoneBefore: string | null = null;
test.afterEach(async ({ page }) => {
  if (zoneBefore) await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
});

test("the technician overview in Malay keeps the timeline in Kuala Lumpur hours", async ({ page }) => {
  const zone = await displayZone(page);
  zoneBefore = zone;
  await setDisplay(page, MALAY, "Asia/Tokyo");
  await page.goto("/technician");
  const main = page.getByRole("main");
  await expect(page.getByRole("tab", { name: /^Hari ini \(\d+\)$/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("combobox", { name: "Isih" })).toContainText("Isih: Keterukan ↓");
  for (const name of ["Hari ini — diisih mengikut keterukan, tarikh akhir, kemajuan", /^Hari ini · /, /^Amaran pada unit saya \(\d+\)$/, "Laporan saya"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main).toContainText("Unit yang ditugaskan");
  await expect(main).toContainText("Garis masa dalam waktu Kuala Lumpur (Asia/Kuala_Lumpur)."); // another display zone
  await page.getByRole("tab", { name: /^Semua yang ditugaskan \(\d+\)$/ }).click();
  await page.waitForURL(/tab=all/);
  await expect(main.getByRole("heading", { name: "Semua yang ditugaskan — diisih mengikut keterukan" })).toBeVisible();
  const job = main.locator("a[href^='/technician/jobs/']").first();
  if (await job.count()) {
    await job.click();
    await page.waitForURL(/\/technician\/jobs\/[^/?]+$/);
    for (const name of ["Kemajuan senarai semak", "Kerja & unit", "Versi & simpan automatik"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
    await expect(main.getByRole("link", { name: "← Gambaran keseluruhan" })).toBeVisible();
    await expect(main).toContainText(/GMT\+9/); // the work window in the display zone
  }
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/technician");
  await expect(page.getByRole("main").getByRole("heading", { name: "Today — sorted by severity, deadline, progress" })).toBeVisible();
});
