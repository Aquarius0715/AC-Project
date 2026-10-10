// AT-X01-N ③ for HQ (FR-X01, FR-A01, IR44, IR289): with Malay and Asia/Tokyo saved in Preferences, the HQ overview
// speaks Malay — the scope and period filters, the KPI tiles, the forecast, the power and connection axes, the job
// statuses and the billing card — and its as-of time is in GMT+9 while the period stays Kuala Lumpur days, named so.
// English and the earlier zone come back at the end, or in afterEach when the test fails.
import { test, expect } from "../../fixtures/test";
import { displayZone, ENGLISH, MALAY, setDisplay } from "../../fixtures/display";

let zoneBefore: string | null = null;
test.afterEach(async ({ page }) => {
  if (zoneBefore) await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
});

test("the HQ overview in Malay keeps the period in Kuala Lumpur days", async ({ page }) => {
  const zone = await displayZone(page);
  zoneBefore = zone;
  await setDisplay(page, MALAY, "Asia/Tokyo");
  await page.goto("/admin");
  const main = page.getByRole("main");
  await expect(main.getByRole("combobox", { name: "Tempoh" })).toContainText("Tempoh: Hari ini");
  for (const label of ["Pelanggan aktif", "Unit sasaran", "Kadar operasi", "Amaran belum selesai", "Tenaga digunakan (sebenar)"]) await expect(main).toContainText(label);
  await expect(main.getByRole("heading", { name: /^Ramalan penjimatan tenaga/ })).toBeVisible(); // the title carries the forecast badge
  for (const name of ["Operasi (keadaan kuasa)", "Sambungan", "Kerja penyelenggaraan mengikut status — tempoh ini"]) await expect(main.getByRole("heading", { name, exact: true }).first()).toBeVisible();
  await expect(main).toContainText(/Setakat .*GMT\+9 · .*Asia\/Kuala_Lumpur/); // the time in the display zone, the period in Kuala Lumpur days
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/admin");
  await expect(page.getByRole("main").getByRole("combobox", { name: "Period" })).toContainText("Period: Today");
});
