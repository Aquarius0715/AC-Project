// AT-X01-N ③ for the contractor (FR-X01, FR-P01, IR44, IR270): with Malay and Asia/Tokyo saved in Preferences, the
// overview speaks Malay, its update time is in GMT+9, and the period and today's timeline stay Kuala Lumpur days and
// hours — the timeline says so. English and the earlier zone come back at the end, or in afterEach when the test fails.
import { test, expect } from "../../fixtures/test";
import { displayZone, ENGLISH, MALAY, setDisplay } from "../../fixtures/display";

let zoneBefore: string | null = null;
test.afterEach(async ({ page }) => {
  if (zoneBefore) await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
});

test("the contractor overview in Malay keeps Kuala Lumpur days and shows the update time in the display zone", async ({ page }) => {
  const zone = await displayZone(page);
  zoneBefore = zone;
  await setDisplay(page, MALAY, "Asia/Tokyo");
  await page.goto("/partner");
  const main = page.getByRole("main");
  for (const name of [/^Kemajuan kerja — /, /^Perlu tindakan anda \(\d+\)$/, /^Kapasiti pasukan — /, /^Hari ini · /, "Aktiviti terkini"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main).toContainText(/Dikemas kini \d{1,2}:\d{2} (PG|PTG) GMT\+9 · kerja syarikat anda sahaja/);
  await expect(main).toContainText("Garis masa dalam waktu Kuala Lumpur (Asia/Kuala_Lumpur)."); // another display zone
  await expect(main.getByRole("combobox", { name: "Tempoh" })).toContainText("Minggu ini");
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/partner");
  await expect(page.getByRole("main").getByRole("heading", { name: /^Job progress — this week$/ })).toBeVisible();
});
