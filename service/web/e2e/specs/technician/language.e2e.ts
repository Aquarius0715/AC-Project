// AT-X01-N ③ for the technician (FR-X01, FR-T01, IR44, IR281): with Malay and Asia/Tokyo saved in Preferences, the
// overview speaks Malay — tiles, tabs, sort and the side cards — while today's timeline stays in Kuala Lumpur hours and
// says so. English and the earlier zone come back at the end, or in afterEach when the test fails.
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
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/technician");
  await expect(page.getByRole("main").getByRole("heading", { name: "Today — sorted by severity, deadline, progress" })).toBeVisible();
});
