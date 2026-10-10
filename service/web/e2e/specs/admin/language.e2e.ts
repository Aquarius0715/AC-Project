// AT-X01-N ③ for HQ (FR-X01, FR-A01, FR-A06, IR44, IR289, IR290): with Malay and Asia/Tokyo saved in Preferences, the
// HQ overview speaks Malay — the scope and period filters, the KPI tiles, the forecast, the power and connection axes,
// the job statuses and the billing card — and its as-of time is in GMT+9 while the period stays Kuala Lumpur days, named
// so; the Jobs tab speaks Malay too — tabs, scope, filters, the list and one job's detail with its times in GMT+9 — and
// New job types its times in Asia/Tokyo. Nothing is saved. English and the earlier zone come back at the end, or in
// afterEach when the test fails.
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
  // the Jobs tab (IR290): the tabs, the scope and the filters, the list, and the first job's detail
  await page.goto("/admin/jobs");
  for (const name of ["Kerja", "Pelan", "Kontraktor", "SLA mengikut pelanggan"]) await expect(main.getByRole("tab", { name: new RegExp(`^${name}`) })).toBeVisible();
  await expect(main.getByRole("tab", { name: /^Kerja/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("combobox", { name: "Pelanggan" })).toContainText("Pelanggan: Semua");
  await expect(main.getByRole("combobox", { name: "Isih" })).toContainText("Isih: Status ↑");
  await expect(main.getByRole("heading", { name: "Kerja · semua pelanggan", exact: true })).toBeVisible();
  await expect(main).toContainText(/\d+ kerja dalam skop/);
  if (await main.getByText("Tetingkap yang diminta", { exact: true }).count()) {
    await expect(main.getByRole("heading", { name: "Kos", exact: true })).toBeVisible();
    await expect(main.getByRole("heading", { name: "Sejarah", exact: true })).toBeVisible();
    await expect(main).toContainText(/GMT\+9/); // the requested window in the display zone
  }
  // New job: its texts and that the typed times are in the display zone; closed without saving
  await main.getByRole("button", { name: "+ Kerja baharu" }).click();
  const create = page.getByRole("dialog", { name: "Kerja penyelenggaraan baharu · langkah 1 daripada 2" });
  await expect(create).toContainText("masa dalam Asia/Tokyo");
  await expect(create.getByRole("button", { name: "Simpan sebagai diminta" })).toBeVisible();
  await create.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(create).toBeHidden();
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/admin");
  await expect(page.getByRole("main").getByRole("combobox", { name: "Period" })).toContainText("Period: Today");
});
