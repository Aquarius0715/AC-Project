// AT-X01-N ③ for the technician (FR-X01, FR-T01, FR-T02, FR-T03, FR-T04, IR44, IR281–IR283): with Malay and
// Asia/Tokyo saved in Preferences, the overview speaks Malay — tiles, tabs, sort and the side cards — while today's
// timeline stays in Kuala Lumpur hours and says so; an assigned job's workspace speaks Malay with its window in GMT+9,
// and so does its unit, on both tabs and with the 7-day period. English and the earlier zone come back at the end, or
// in afterEach when the test fails.
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
    // the job's unit (IR283): the register, the history and alerts, then Monitoring with another period in the URL
    await main.getByRole("link", { name: "Unit →", exact: true }).click();
    await page.waitForURL(/\/technician\/units\/[^/?]+\?jobId=/);
    for (const name of ["Daftar unit", "Pemantauan siri masa", "Sejarah penyelenggaraan", /^Amaran terbuka \(\d+\)$/, /^Komponen \(\d+\)$/]) await expect(main.getByRole("heading", { name })).toBeVisible();
    await expect(main.getByRole("tab", { name: "Daftar", exact: true })).toHaveAttribute("aria-selected", "true");
    await main.getByRole("tab", { name: "Pemantauan", exact: true }).click();
    await page.waitForURL(/metric=/);
    await expect(main.getByRole("heading", { name: /· 24 j terakhir \(bergerak\)$/ })).toBeVisible();
    await expect(main.getByRole("heading", { name: "Peristiwa dalam tempoh ini" })).toBeVisible();
    await main.getByRole("tab", { name: "7 hari", exact: true }).click();
    await page.waitForURL(/period=7d/);
    await expect(main.getByRole("heading", { name: /· 7 hari terakhir \(hari kalendar Kuala Lumpur\)$/ })).toBeVisible();
  }
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/technician");
  await expect(page.getByRole("main").getByRole("heading", { name: "Today — sorted by severity, deadline, progress" })).toBeVisible();
});
