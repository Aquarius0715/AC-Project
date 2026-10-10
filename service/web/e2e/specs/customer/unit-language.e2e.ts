// AT-X01-N ③ (FR-X01, IR44, NFR-08, IR259–IR267): with Malay and another display time zone saved in Preferences, every
// customer screen — overview, units & locations, automations, alerts, maintenance, energy & cost, carbon offsets, air
// quality, contracts & payments with an invoice, users and the unit screen — speaks Malay and shows its times in that
// zone with the zone's abbreviation. The request form takes its times in that zone too, while the energy periods, the
// 7-day air window and the due dates stay Kuala Lumpur days; the unit's ID and units (°C) stay as stored. English and the
// earlier zone come back at the end, or in afterEach when the test fails.
import { test, expect } from "../../fixtures/test";
import { displayZone, ENGLISH, MALAY, setDisplay } from "../../fixtures/display";

let zoneBefore: string | null = null;
test.afterEach(async ({ page }) => {
  if (zoneBefore) await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
});

test("the customer screens in Malay keep IDs and units and use the display time zone", async ({ page }) => {
  const zone = await displayZone(page);
  await page.goto("/customer"); // the overview lists every AC with a link to its screen
  const href = await page.locator('main a[href^="/customer/units/"]').first().getAttribute("href");
  expect(href).toBeTruthy();
  zoneBefore = zone;
  await setDisplay(page, MALAY, "Asia/Tokyo");
  await page.goto("/customer");
  const main = page.getByRole("main");
  for (const name of ["Tenaga digunakan", "Anggaran pelepasan", "Perlu perhatian", "Automasi"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main).toContainText(/Dikemas kini \d{1,2}:\d{2} (PG|PTG) GMT\+9/); // the overview's read time in Asia/Tokyo
  await expect(main).toContainText("Asia/Kuala_Lumpur"); // the period note: the periods are Kuala Lumpur days
  await page.goto("/customer/properties");
  await expect(main.getByRole("heading", { name: "Premis saya" })).toBeVisible();
  await main.locator("button[aria-pressed]").first().click(); // the first property of the tree: its summary
  await expect(main.getByText("Pilih bilik untuk melihat penyaman udaranya")).toBeVisible();
  await page.goto("/customer/automations");
  await expect(main.getByRole("heading", { name: "Automasi anda" })).toBeVisible();
  await page.goto("/customer/automations?automationId=new");
  // a new rule takes the Preferences zone (Asia/Tokyo here) and its runs read in that zone, with the weekday in Malay
  await expect(main.getByText("Larian seterusnya", { exact: true }).locator("..")).toContainText(/(Isn|Sel|Rab|Kha|Jum), \d{1,2} \w+, 18:00 GMT\+9 · 22:00 GMT\+9/);
  await page.goto("/customer/alerts");
  for (const name of ["Perlu perhatian", "Peringatan & maklumat"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main).toContainText(/Dikemas kini \d{1,2}:\d{2} (PG|PTG) GMT\+9/);
  await page.getByRole("tab", { name: /^Polisi amaran/ }).click();
  await page.waitForURL(/tab=policies/);
  await expect(main.getByRole("heading", { name: "Polisi amaran" })).toBeVisible();
  await expect(main.getByRole("columnheader", { name: "Peraturan" })).toBeVisible();
  await page.goto("/customer/maintenance");
  await expect(page.getByRole("tab", { name: "Permintaan saya" })).toHaveAttribute("aria-selected", "true");
  await page.getByRole("button", { name: "+ Permintaan penyelenggaraan baharu" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "Permintaan penyelenggaraan baharu" })).toBeVisible();
  await expect(dialog).toContainText("masa dalam Asia/Tokyo"); // the preferred times are typed in the display time zone
  await dialog.getByRole("button", { name: "Batal" }).click();
  await expect(dialog).toBeHidden();
  await page.goto("/customer/energy");
  for (const name of [/^Tenaga (harian|sebenar harian)/, "Kesan karbon"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main).toContainText(/\(\d+ hari\) · Asia\/Kuala_Lumpur/); // the business period stays Kuala Lumpur days, labelled in Malay
  await expect(main.getByRole("button", { name: "↓ Eksport" })).toBeVisible();
  await page.goto("/customer/energy/offsets");
  for (const name of ["Berdasarkan Tenaga & kos", "Berapa banyak yang anda ingin ofset?", "Rekod ofset demo"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await page.goto("/customer/air-quality");
  await expect(main.getByRole("heading", { name: "Sejarah pengudaraan" })).toBeVisible();
  await expect(main).toContainText(/tetingkap bergerak berakhir \d{1,2}:\d{2} (PG|PTG) GMT\+9/); // the 24-hour window ends now, in Asia/Tokyo
  await page.getByRole("tab", { name: "7 hari" }).click();
  await page.waitForURL(/period=7d/);
  await expect(main).toContainText("hari kalendar dalam Asia/Kuala_Lumpur"); // the 7-day window starts on a Kuala Lumpur day
  await main.getByRole("button", { name: "Rekod pengudaraan" }).click(); // opened and cancelled: a log cannot be removed
  await expect(dialog).toContainText("CO2 semasa");
  await dialog.getByRole("button", { name: "Batal" }).click();
  await expect(dialog).toBeHidden();
  await page.goto("/customer/payments");
  for (const name of [/^KONTRAK \(\d+\)$/, "Invois untuk kontrak ini"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  await expect(main).toContainText("Tarikh akhir dan tempoh kontrak ialah tarikh Kuala Lumpur (Asia/Kuala_Lumpur)."); // another display zone: the due dates stay KL days
  await main.locator('a[href^="/customer/payments/"]').first().click(); // the newest invoice of the contract
  await page.waitForURL(/\/customer\/payments\/[^/?]+/);
  await expect(page.getByRole("tab", { name: "Pembayaran" })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "Sejarah pembayaran" })).toBeVisible();
  await expect(main).toContainText("tarikh Kuala Lumpur (Asia/Kuala_Lumpur)");
  await page.goto("/customer/users");
  await expect(main.getByRole("heading", { name: "Pengguna", level: 1 })).toBeVisible();
  await expect(main.getByRole("columnheader", { name: "Peranan" })).toBeVisible();
  await page.getByRole("button", { name: "+ Jemput ahli" }).click(); // opened and cancelled: nothing is invited
  await expect(dialog.getByRole("heading", { name: "Jemput ahli" })).toBeVisible();
  await dialog.getByRole("button", { name: "Batal" }).click();
  await expect(dialog).toBeHidden();
  await page.goto(href!);
  for (const name of ["KAWALAN JAUH", "Sejarah arahan", "Telemetri langsung"]) await expect(main.getByRole("heading", { name })).toBeVisible();
  expect(new URL(page.url()).pathname).toBe(href); // the ID is the stored one
  await expect(main).toContainText("°C");
  await expect(main).toContainText("GMT+9"); // Asia/Tokyo, not the Malaysian MYT
  await expect(main).not.toContainText("MYT");
  await setDisplay(page, ENGLISH, zone);
  zoneBefore = null;
  await page.goto(href!);
  await expect(page.getByRole("main").getByRole("heading", { name: "REMOTE CONTROL" })).toBeVisible();
});
