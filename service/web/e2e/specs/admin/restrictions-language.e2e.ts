// AT-X01-N ③ for HQ restrictions (FR-X01, FR-A09, FR-A10, IR301): signed in as the restriction manager in its own
// browser context, with Malay and Asia/Tokyo saved in its Preferences, the restriction list (state chips, the selected
// restriction's lifecycle, summary with its times in GMT+9, cause invoices and units), the schedule dialog — its times
// typed in Asia/Tokyo, its checks stopping before any call — and the exception screen (action cards, the end typed in
// Asia/Tokyo, the reason check, the summary and audit) speak Malay. Nothing is saved; the manager's English and earlier
// zone come back at the end, or in afterEach when the test fails.
import type { BrowserContext, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { signIn } from "../../fixtures/auth";
import { displayZone, ENGLISH, MALAY, setDisplay } from "../../fixtures/display";

const MANAGER = "hq-restriction-manager"; // restriction.read / .write / .override and audit.read (demo seed)
let context: BrowserContext | null = null;
let manager: Page | null = null;
let zoneBefore: string | null = null;

test.afterEach(async () => {
  if (manager && zoneBefore) await setDisplay(manager, ENGLISH, zoneBefore);
  await context?.close();
  context = manager = zoneBefore = null;
});

test("the HQ restriction screens in Malay with times in the display time zone", async ({ browser, app }) => {
  test.setTimeout(120_000); // a one-time code is accepted once per 30 s window, so the sign-in may wait for the next one
  context = await browser.newContext({ baseURL: app.url, storageState: { cookies: [], origins: [] } }); // not the project's hq-operator session
  const page = (manager = await context.newPage());
  await signIn(page, app, MANAGER);
  zoneBefore = await displayZone(page);
  await setDisplay(page, MALAY, "Asia/Tokyo");
  const main = page.getByRole("main");

  // the list and the selected restriction
  await page.goto("/admin/restrictions");
  for (const name of ["Dijadualkan", "Diminta", "Dikenakan", "Pelepasan diminta", "Dilepaskan", "Dibatalkan"]) await expect(main.getByRole("button", { name: new RegExp(`^${name}`) }).first()).toBeVisible();
  await expect(main.getByRole("heading", { name: "Sekatan", exact: true })).toBeVisible();
  if (await main.getByRole("heading", { name: /^Sekatan [0-9a-f]{8}$/ }).count()) {
    await expect(main).toContainText(/Laksana selepas\s*\d{1,2} \S+ \d{4}, .+ GMT\+9/); // an instant in the display zone
    for (const label of ["Niat pelepasan", "Invois punca"]) await expect(main).toContainText(label);
    await expect(main.getByRole("columnheader", { name: "Pengenaan" })).toBeVisible();
    const release = main.getByRole("button", { name: "Minta pelepasan" });
    if ((await release.count()) && (await release.isDisabled())) await expect(main).toContainText("Invois punca belum dibayar dan tiada tempoh tangguh atau pengecualian aktif");
  }

  // the schedule dialog: the execution time typed in the display zone; the checks stop before any call
  await main.getByRole("button", { name: "+ Jadualkan sekatan" }).click();
  const schedule = page.getByRole("dialog", { name: "Jadualkan sekatan" });
  await expect(schedule).toContainText("masa dalam Asia/Tokyo");
  await schedule.getByRole("button", { name: "Jadualkan", exact: true }).click();
  for (const text of ["Pilih kontrak RTO yang layak", "Unit yang dipilih memerlukan julat suhu", "1–1000 aksara, ditunjukkan kepada pelanggan"]) await expect(schedule).toContainText(text); // the units show once a contract is chosen
  await schedule.getByRole("button", { name: "Batal", exact: true }).click();
  await expect(schedule).toBeHidden();

  // the exception screen of the selected restriction
  const exception = main.getByRole("link", { name: "Pengecualian / pelepasan manual →" });
  if (await exception.count()) {
    await exception.click();
    await page.waitForURL(/\/admin\/restrictions\/[0-9a-f-]{36}$/);
    await expect(main.getByRole("link", { name: "← Sekatan" })).toBeVisible();
    await expect(main.getByRole("heading", { name: "Pilih tindakan", exact: true })).toBeVisible();
    for (const name of ["Tempoh tangguh", "Pengecualian", "Batal", "Lepaskan secara manual"]) await expect(main.getByRole("button", { name: new RegExp(`^${name}`) }).first()).toBeVisible();
    for (const name of ["Apa yang berlaku", "Ringkasan", "Audit"]) await expect(main.getByRole("heading", { name, exact: true })).toBeVisible();
    const grace = main.getByRole("button", { name: /^Tempoh tangguh/ });
    if (await grace.isEnabled()) {
      await grace.click();
      await expect(main.getByLabel("Sehingga · akan datang, paling lama 90 hari")).toBeVisible();
      await expect(main).toContainText("masa dalam Asia/Tokyo"); // the end is typed in the display zone
      await main.getByRole("button", { name: "Gunakan tempoh tangguh" }).click();
      await expect(main).toContainText("Sebab diperlukan (1–1000 aksara)"); // no reason: nothing is sent
    }
  }

  await setDisplay(page, ENGLISH, zoneBefore);
  zoneBefore = null;
  await page.goto("/admin/restrictions");
  await expect(page.getByRole("main").getByRole("heading", { name: "Restrictions", exact: true })).toBeVisible();
});
