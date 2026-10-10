// AT-X01-N ③ for HQ (FR-X01, FR-A01, FR-A03, FR-A06, FR-A07, FR-A08, FR-A11, FR-A13, FR-A14, FR-A15, FR-A23, IR44, IR289, IR290): with Malay and Asia/Tokyo saved in Preferences, the
// HQ overview speaks Malay — the scope and period filters, the KPI tiles, the forecast, the power and connection axes,
// the job statuses and the billing card — and its as-of time is in GMT+9 while the period stays Kuala Lumpur days, named
// so; the Jobs tab speaks Malay too — tabs, scope, filters, the list and one job's detail with its times in GMT+9 — and
// New job types its times in Asia/Tokyo; so do the Plans tab (its next date typed in Asia/Tokyo), the Contractors tab
// (its dates named Kuala Lumpur days) and the SLA tab with its targets dialog (IR291), the alerts with their policies
// (IR292), customers & units — the register, one customer's locations, users and policies, and warranty & coverage
// (IR293) — the device registry's three tabs with the new-campaign dialog (IR295), the energy analysis with its
// period named Kuala Lumpur time and the baselines with the form checks (IR296), and the MRV reports with a preview
// (a read) and the emission factors with the form checks (IR297), the offset demo's records, market concept and
// new-quote checks (IR298), and billing — invoices with their Kuala Lumpur dates named in another zone, a reminder
// preview (a read), the payment and invoice checks, inquiries and payouts (IR299), and contracts with their Kuala Lumpur
// days and the new-contract checks (IR300), the restrictions' no-access state (IR301), and access & roles with the
// valid period in the display zone and the new-membership checks (IR302), and the automation policies' editor and
// new-policy checks (IR303). Nothing is saved. English and the earlier zone come back at the end, or in
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
  // the Plans, Contractors and SLA tabs (IR291)
  await main.getByRole("tab", { name: /^Pelan/ }).click();
  await page.waitForURL(/tab=plans/);
  await expect(main.getByRole("heading", { name: "Pelan berulang", exact: true })).toBeVisible();
  if (await main.getByRole("button", { name: "Jana kerja" }).count()) await expect(main).toContainText("masa dalam Asia/Tokyo");
  await main.getByRole("tab", { name: /^Kontraktor/ }).click();
  await page.waitForURL(/tab=contractors/);
  await expect(main.getByRole("heading", { name: "Kontraktor", exact: true })).toBeVisible();
  await expect(main.getByRole("button", { name: "+ Tambah kontraktor" })).toBeVisible();
  if (await main.getByRole("heading", { name: "Juruteknik & sijil", exact: true }).count()) await expect(main).toContainText("Hari-hari itu ialah hari di Kuala Lumpur.");
  await main.getByRole("tab", { name: "SLA mengikut pelanggan" }).click();
  await page.waitForURL(/tab=sla/);
  await expect(main.getByRole("combobox", { name: "Tempoh" })).toContainText("Tempoh: 90 hari lepas");
  for (const name of ["Pelanggan", "Pelanggaran terkini"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
  await main.getByRole("button", { name: "Sunting sasaran SLA" }).click();
  const targets = page.getByRole("dialog", { name: "Sunting sasaran SLA" });
  await expect(targets).toContainText("masa dalam Asia/Tokyo");
  await targets.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(targets).toBeHidden();
  // the alerts and their policies (IR292): nothing acknowledged or resolved
  await page.goto("/admin/alerts");
  await expect(main.getByRole("tab", { name: /^Amaran/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "Amaran terbuka · semua pelanggan", exact: true })).toBeVisible();
  if (await main.getByRole("button", { name: "Akui", exact: true }).count()) {
    await expect(main.getByRole("button", { name: "Selesaikan…" })).toBeVisible();
    await expect(main).toContainText(/dikesan .*GMT\+9/); // the detection time in the display zone
  }
  await main.getByRole("tab", { name: /^Polisi/ }).click();
  await page.waitForURL(/tab=policies/);
  await expect(main.getByRole("heading", { name: "Polisi amaran", exact: true })).toBeVisible();
  // customers & units (IR293)
  await page.goto("/admin/units");
  await expect(main.getByRole("tab", { name: /^Pelanggan & unit/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "Pelanggan", exact: true })).toBeVisible();
  await expect(main.getByRole("combobox", { name: "Status" })).toContainText("Status: Aktif");
  await main.getByRole("row", { name: /Demo Customer A/ }).click();
  await page.waitForURL(/customerId=/);
  await expect(main.getByRole("tab", { name: /^Unit & lokasi/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "Lokasi", exact: true })).toBeVisible();
  await expect(main).toContainText(/pelanggan sejak /);
  await main.getByRole("tab", { name: /^Pengguna/ }).click();
  await page.waitForURL(/tab=users/);
  await expect(main.getByRole("heading", { name: /^Pengguna pelanggan / })).toBeVisible();
  await main.getByRole("tab", { name: /^Polisi amaran/ }).click();
  await page.waitForURL(/tab=policies/);
  await expect(main.getByRole("heading", { name: /^Polisi amaran / })).toBeVisible();
  await page.goto("/admin/units?tab=warranty");
  await expect(main.getByRole("combobox", { name: "Liputan" })).toContainText("Liputan: Semua");
  await expect(main.getByRole("heading", { name: "Unit mengikut tamat liputan", exact: true })).toBeVisible();
  // the device registry (IR295): models, IoT devices, firmware campaigns and the new-campaign dialog, closed unsaved
  await page.goto("/admin/devices");
  await expect(main.getByRole("tab", { name: /^Model/ })).toHaveAttribute("aria-selected", "true");
  for (const name of ["Identiti", "Kawalan", "Penderia"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
  await main.getByRole("tab", { name: /^Peranti IoT/ }).click();
  await page.waitForURL(/tab=devices/);
  await expect(main.getByRole("heading", { name: "Peristiwa peranti", exact: true })).toBeVisible();
  await expect(main).toContainText("Kali terakhir dilihat");
  await main.getByRole("tab", { name: /^Kempen perisian tegar/ }).click();
  await page.waitForURL(/tab=firmware/);
  await main.getByRole("button", { name: "+ Kempen baharu" }).click();
  const campaign = page.getByRole("dialog", { name: "Kempen perisian tegar baharu" });
  await expect(campaign).toContainText("masa dalam Asia/Tokyo"); // the start is typed in the display zone
  await expect(campaign).toContainText("Waktu tempatan peranti"); // the install window is the device's local time
  await campaign.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(campaign).toBeHidden();
  // the energy analysis (IR296): the scope with the period in Kuala Lumpur time, the figures and conditions, and the
  // baselines with the new-baseline checks, which stop before any call
  await page.goto("/admin/energy");
  await expect(main.getByRole("tab", { name: /^Analisis/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "Skop", exact: true })).toBeVisible();
  await expect(main.getByLabel("Dari (Kuala Lumpur)")).toBeVisible();
  for (const name of ["Kos", "Syarat pengiraan"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
  await expect(main).toContainText(/Tempoh\s*\d{4}-\d{2}-\d{2} \d{2}:\d{2} → .*\(Asia\/Kuala_Lumpur\)/); // named Kuala Lumpur time in another zone
  await main.getByRole("tab", { name: /^Garis dasar/ }).click();
  await page.waitForURL(/tab=baselines/);
  await expect(main.getByLabel("Tempoh bermula dari (Kuala Lumpur)")).toBeVisible();
  await main.getByRole("button", { name: "+ Baharu" }).click();
  await expect(main.getByRole("heading", { name: "Garis dasar baharu", exact: true })).toBeVisible();
  await main.getByRole("button", { name: "Cipta garis dasar" }).click();
  for (const text of ["Pilih 1–100 unit", "Tamat mesti selepas mula", "Nilai ≥ 0 kWh"]) await expect(main).toContainText(text);
  await main.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(main.getByRole("heading", { name: "Garis dasar baharu", exact: true })).toBeHidden();
  // the MRV workspace (IR297): the reports with the new-report checks and a preview — a read, so nothing is stored —
  // and the emission factors with the new-factor checks, which stop before any call
  await page.goto("/admin/mrv");
  await expect(main.getByRole("tab", { name: /^Laporan/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "Laporan Skop 2", exact: true })).toBeVisible();
  await expect(main.getByLabel("Tempoh bermula dari (Kuala Lumpur)")).toBeVisible();
  await main.getByRole("button", { name: "+ Baharu" }).click();
  const report = page.getByRole("dialog", { name: "Laporan Skop 2 baharu" });
  await report.getByRole("button", { name: "Pratonton", exact: true }).click();
  for (const text of ["Pilih organisasi pelanggan", "Pilih versi garis dasar", "Pilih versi faktor pelepasan"]) await expect(report).toContainText(text);
  await report.getByLabel("Organisasi pelanggan").selectOption({ label: "Demo Customer A" });
  await report.getByRole("checkbox").first().check();
  await report.getByLabel("Mula (Kuala Lumpur)").fill("2026-09-01T00:00");
  await report.getByLabel("Tamat", { exact: true }).fill("2026-09-08T00:00");
  await report.getByLabel("Versi garis dasar").selectOption({ index: 1 });
  await report.getByLabel("Versi faktor pelepasan").selectOption({ index: 1 });
  await report.getByLabel("Penerangan sempadan").fill("AC input electricity (E2E preview)");
  await report.getByRole("button", { name: "Pratonton", exact: true }).click();
  await expect(report.getByText("Pratonton · tidak disimpan", { exact: true })).toBeVisible();
  await expect(report).toContainText("Demo — belum disahkan");
  await expect(report.getByText(/^(Elektrik digunakan|Pengiraan tidak lengkap — )/).first()).toBeVisible(); // the figures, or why they are not shown
  await report.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(report).toBeHidden();
  await main.getByRole("tab", { name: /^Faktor pelepasan/ }).click();
  await page.waitForURL(/tab=factors/);
  await main.getByRole("button", { name: "+ Baharu" }).click();
  await expect(main.getByRole("heading", { name: "Faktor baharu", exact: true })).toBeVisible();
  await main.getByRole("button", { name: "Cipta faktor" }).click();
  for (const text of ["1–120 aksara", "Melebihi 0 dan paling banyak 10 kgCO₂e/kWh"]) await expect(main).toContainText(text);
  // the offset demo (IR298): the records and the market concept, and the new-quote checks, which stop before any call
  // (a quote is a write)
  await page.goto("/admin/offsets");
  await expect(main).toContainText("Demo sahaja — sebut harga, pembelian dan pembatalan disimulasikan.");
  await expect(main.getByRole("tab", { name: /^Rekod demo/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "Rekod ofset demo", exact: true })).toBeVisible();
  await main.getByRole("button", { name: "+ Sebut harga demo baharu" }).click();
  const quote = page.getByRole("dialog", { name: "Sebut harga demo baharu" });
  await expect(quote.getByLabel("Mula tempoh (Kuala Lumpur)")).toBeVisible(); // the quote's period is Kuala Lumpur time
  await quote.getByRole("button", { name: "Dapatkan sebut harga demo" }).click();
  for (const text of ["Pilih pelanggan", "Pilih 1–100 unit", "Tamat mesti selepas mula", "1–1000 aksara", "Melebihi 0, paling banyak 100000, sehingga 3 perpuluhan"]) await expect(quote).toContainText(text);
  await quote.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(quote).toBeHidden();
  await main.getByRole("tab", { name: "Konsep pasaran" }).click();
  await page.waitForURL(/tab=market/);
  await expect(main.getByRole("heading", { name: "Pasaran karbon — konsep masa depan", exact: true })).toBeVisible();
  // billing (IR299): the invoices with their Kuala Lumpur days named in another zone; a reminder preview, which is a read
  // (nothing is sent); the manual-payment and new-invoice checks, which stop before any call; inquiries and payouts
  await page.goto("/admin/billing");
  await expect(main.getByRole("tab", { name: /^Invois/ })).toHaveAttribute("aria-selected", "true");
  await expect(main).toContainText("Tempoh pengebilan dan tarikh akhir ialah tarikh Kuala Lumpur (Asia/Kuala_Lumpur).");
  for (const label of ["Belum dijelaskan", "Tertunggak", "Sedang diproses", "Dibayar"]) await expect(main).toContainText(label);
  if (await main.getByRole("heading", { name: "Peringatan pembayaran", exact: true }).count()) {
    await main.getByLabel("Sebab · cth. “Peringatan ke-2 selepas tarikh akhir”").fill("E2E preview");
    await main.getByRole("button", { name: "Pratonton", exact: true }).click();
    await expect(main.getByText("belum ada apa-apa dihantar", { exact: true })).toBeVisible();
    await expect(main).toContainText(/Helo, .+ perlu dibayar pada /); // the message the customer gets, in this screen's language
  }
  if (await main.getByRole("button", { name: "Rekod pembayaran manual…" }).count()) {
    await main.getByRole("button", { name: "Rekod pembayaran manual…" }).click();
    const pay = page.getByRole("dialog", { name: "Rekod pembayaran manual" });
    await pay.getByRole("button", { name: "Rekod pembayaran", exact: true }).click();
    for (const text of ["Rujukan diperlukan", "Sebab diperlukan"]) await expect(pay).toContainText(text);
    await pay.getByRole("button", { name: "Batal", exact: true }).click();
    await expect(pay).toBeHidden();
  }
  await main.getByRole("button", { name: "+ Cipta invois" }).click();
  const newInvoice = page.getByRole("dialog", { name: "Cipta invois" });
  await expect(newInvoice).toContainText("Tarikh ialah hari Kuala Lumpur (Asia/Kuala_Lumpur).");
  await newInvoice.getByRole("button", { name: "Cipta", exact: true }).click();
  await expect(newInvoice).toContainText("Diperlukan");
  await newInvoice.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(newInvoice).toBeHidden();
  await main.getByRole("tab", { name: /^Pertanyaan/ }).click();
  await page.waitForURL(/tab=inquiries/);
  await expect(main.getByRole("heading", { name: "Pertanyaan pelanggan", exact: true })).toBeVisible();
  await main.getByRole("tab", { name: /^Pembayaran kontraktor/ }).click();
  await page.waitForURL(/tab=payouts/);
  await expect(main.getByRole("heading", { name: "Penyata", exact: true })).toBeVisible();
  // contracts (IR300): the plan tabs, the Kuala Lumpur days, and the new-contract checks, which stop before any call
  await page.goto("/admin/billing/contracts");
  for (const name of ["RTO", "Umum", "Tenaga", "Alam sekitar"]) await expect(main.getByRole("tab", { name, exact: true })).toBeVisible();
  await expect(main.getByRole("tab", { name: /^Semua/ })).toHaveAttribute("aria-selected", "true");
  await main.getByRole("button", { name: "+ Baharu" }).click();
  await expect(main.getByRole("heading", { name: "Kontrak baharu", exact: true })).toBeVisible();
  await expect(main).toContainText("Tarikh ialah hari Kuala Lumpur (Asia/Kuala_Lumpur).");
  await main.getByRole("button", { name: "Cipta kontrak" }).click();
  for (const text of ["Pilih pelanggan", "Masukkan sekurang-kurangnya satu unit", "Tamat mesti selepas mula", "Harga ≥ 0 dengan paling banyak 2 perpuluhan"]) await expect(main).toContainText(text);
  await main.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(main.getByRole("heading", { name: "Kontrak baharu", exact: true })).toBeHidden();
  // restrictions (IR301): this account holds no restriction permission, so the screen says so in Malay
  // (restrictions-language.e2e.ts opens them as the restriction manager)
  await page.goto("/admin/restrictions");
  await expect(main).toContainText("Sekatan memerlukan restriction.read, restriction.write atau restriction.override.");
  // access & roles (IR302): the memberships, the selected one's valid period in GMT+9 and its form typed in Asia/Tokyo,
  // and the new-membership checks, which stop before any call
  await page.goto("/admin/settings/access");
  for (const name of ["Semua peranan", "Pentadbir", "Kontraktor", "Juruteknik"]) await expect(main.getByRole("button", { name, exact: true }).first()).toBeVisible();
  await expect(main.getByRole("heading", { name: "Keahlian", exact: true })).toBeVisible();
  await expect(main).toContainText(/GMT\+9 → /); // the valid period in the display zone
  await expect(main).toContainText("Masa dalam Asia/Tokyo");
  await expect(main.getByRole("columnheader", { name: "Baca" })).toBeVisible();
  await main.getByRole("button", { name: "+ Keahlian baharu" }).click();
  await page.waitForURL(/membershipId=new/);
  await expect(main.getByRole("heading", { name: "Keahlian baharu", exact: true })).toBeVisible();
  await main.getByRole("button", { name: "Cipta keahlian" }).click();
  for (const text of ["Pilih pengguna", "Pilih organisasi", "Sebab perubahan diperlukan (1–1000 aksara)"]) await expect(main).toContainText(text);
  // automation policies (IR303): the When / Then editor and its sentence, and the new-policy checks, which stop before
  // any call
  await page.goto("/admin/settings/automation");
  await expect(main.getByRole("heading", { name: "Polisi automasi HQ", exact: true })).toBeVisible();
  await main.getByRole("button", { name: "+ Baharu" }).click();
  await page.waitForURL(/policyId=new/);
  for (const name of ["Penghunian", "Tarif", "Puncak", "Solar", "Bateri"]) await expect(main.getByRole("button", { name, exact: true })).toBeVisible();
  await expect(main).toContainText("Apabila tarif elektrik > 0.6 MYR/kWh → tetapkan suhu kepada 26 °C.");
  await expect(main.getByRole("heading", { name: "Cara konflik diselesaikan", exact: true })).toBeVisible();
  await main.getByRole("button", { name: "Cipta polisi" }).click();
  for (const text of ["1–120 aksara", "Pilih sekurang-kurangnya satu unit"]) await expect(main).toContainText(text);
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto("/admin");
  await expect(page.getByRole("main").getByRole("combobox", { name: "Period" })).toContainText("Period: Today");
});
