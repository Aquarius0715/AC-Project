// AT-X01-N ③ for the contractor (FR-X01, FR-P01, FR-P02, FR-P03, FR-P05, FR-P06, IR44, IR270–IR276): with Malay and
// Asia/Tokyo saved in Preferences, the overview, the job list, a job's page, a completed job's quality review, the
// schedule and the team speak Malay; the update time and the delegation windows are in GMT+9, while the period,
// today's timeline and the team's weeks stay Kuala Lumpur days and hours — the screens say so.
// English and the earlier zone come back at the end, or in afterEach when the test fails.
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
  await page.goto("/partner/jobs?tab=completed");
  const done = main.locator("a[href^='/partner/jobs/']").first(); // a completed job inside its delegation: its quality review
  if (await done.count()) {
    await page.goto(`${(await done.getAttribute("href"))!.split("?")[0]}/review`);
    await expect(main.getByRole("heading", { name: /^(Semakan bukti|Tiada laporan untuk disemak)$/ })).toBeVisible();
  }
  await page.goto("/partner/schedule");
  await expect(main.getByRole("heading", { name: /^(Kerja yang diterima|○ Tiada kerja yang diterima)$/ })).toBeVisible();
  if (await main.getByRole("heading", { name: "Kerja yang diterima", exact: true }).count()) { // not the empty state's "○ Tiada kerja yang diterima"
    await expect(main.getByRole("heading", { name: "Tetingkap delegasi" })).toBeVisible();
    await expect(main.getByRole("combobox", { name: "Isih" })).toContainText("Status ↑");
    await expect(main).toContainText(/GMT\+9/); // the delegation windows in the display zone
    const week = main.getByRole("heading", { name: /^Jadual pasukan — minggu \d{1,2} \S+$/ });
    if (await week.count()) await expect(main).toContainText("Hari dan masa dalam waktu Kuala Lumpur (Asia/Kuala_Lumpur).");
  }
  await page.goto("/partner/team");
  await expect(main.getByRole("heading", { name: /^Juruteknik — .+ · \d+ aktif$/ })).toBeVisible();
  await expect(main.getByRole("heading", { name: /^Minggu \d{1,2} \S+ — jam ditugaskan \/ tersedia$/ })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Kelayakan", exact: true })).toBeVisible();
  await expect(main).toContainText("Hari dan jam dalam waktu Kuala Lumpur (Asia/Kuala_Lumpur)."); // another display zone
  await page.goto("/partner/team?tab=certifications");
  await expect(main.getByRole("heading", { name: /^Sijil — / })).toBeVisible();
  await expect(main.getByRole("heading", { name: "Kesan penugasan" })).toBeVisible();
  await expect(main.getByRole("region", { name: "Menunggu pengesahan HQ" })).toBeVisible(); // a KPI
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/partner");
  await expect(page.getByRole("main").getByRole("heading", { name: /^Job progress — this week$/ })).toBeVisible();
});
