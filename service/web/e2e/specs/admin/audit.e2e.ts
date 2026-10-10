// HQ audit explorer (FR-A16, DD-A16, SCR-A16, Figma Admin 337:2 / 338:x / 340:2, IR320): the read-only banner; the
// filters live in the URL (actor, target kind and ID, correlation ID, result, period) and so does the open entry
// (entryId); "Filter by …" of the open entry keeps it open; an unknown correlation ID and a reversed period say why
// nothing shows; the related records link the records' own screens. The device events tab reads events only for a
// picked device — no fallback — and links an event's audit trail. Nothing is written, except a display-zone round trip
// that gives an empty log (a fresh seed) its first entries and leaves the preferences as they were.
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test";
import { displayZone, ENGLISH, setDisplay } from "../../fixtures/display";

const card = (page: Page, heading: string | RegExp) => page.getByRole("main").locator("section").filter({ has: page.getByRole("heading", { name: heading, exact: typeof heading === "string" }) });
/** The value of a SummaryList row of the open entry. */
const fact = (detail: Locator, label: string) => detail.locator("dt", { hasText: new RegExp(`^${label}$`) }).locator("xpath=following-sibling::dd[1]");

test("the audit log keeps its filters and the open entry in the URL", async ({ page }) => {
  await page.goto("/admin/audit");
  const main = page.getByRole("main");
  await expect(main).toContainText("Read-only · entries are appended only by business events");
  await expect(main).toContainText("the log is append-only");
  if (await main.getByText("No audit entries match these filters").count()) { // a fresh log: two audited saves that end where they began
    const zone = await displayZone(page);
    await setDisplay(page, ENGLISH, zone === "Asia/Tokyo" ? "Asia/Kuala_Lumpur" : "Asia/Tokyo");
    await setDisplay(page, ENGLISH, zone);
    await page.goto("/admin/audit");
  }
  const list = card(page, "Audit log · newest first");
  const rows = list.locator("button[aria-pressed]");
  await expect(rows.first()).toBeVisible();
  expect(page.url()).not.toContain("entryId"); // the newest entry is open by default
  await expect(list.getByRole("button", { pressed: true })).toHaveCount(1);

  // opening an entry puts it in the URL; its detail names the actor, the recorded role and the target
  const second = (await rows.count()) > 1 ? rows.nth(1) : rows.first();
  const op = (await second.locator("b").innerText()).trim();
  await second.click();
  await page.waitForURL(/entryId=[0-9a-f-]{36}/);
  const detail = card(page, new RegExp(`^${op.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`));
  await expect(detail.first()).toBeVisible();
  const entryId = new URL(page.url()).searchParams.get("entryId");
  await expect(fact(detail.first(), "Role at the time")).toContainText("kept as recorded, not rewritten by the current membership");
  const target = (await fact(detail.first(), "Target").locator("span.font-mono").innerText()).trim(); // "kind · id"
  const [kind, id] = target.split(" · ");

  // Filter by target → keeps the entry open; every shown entry has that target; the chip clears it
  await detail.first().getByRole("button", { name: "Filter by target →" }).click();
  await page.waitForURL((u) => u.searchParams.get("targetId") === id && u.searchParams.get("targetKind") === kind && u.searchParams.get("entryId") === entryId);
  await expect(main.getByRole("combobox", { name: "Target" })).toHaveValue(kind);
  await expect(main).toContainText(`Target ID · ${kind} · ${id}`);
  await expect.poll(() => rows.evaluateAll((bs) => [...new Set(bs.map((b) => b.querySelector('div[class~="text-[11px]"]')?.textContent ?? ""))])).toEqual([target]);
  await main.getByRole("button", { name: "Clear the target ID" }).click();
  await page.waitForURL((u) => !u.searchParams.has("targetId") && u.searchParams.get("targetKind") === kind && !u.searchParams.has("entryId"));

  // the related records of the newest entry of that kind link the records' own screens
  const related = card(page, /./).filter({ has: page.getByRole("heading", { name: "Related records", exact: true }) });
  await expect(related.first()).toBeVisible();
  if (kind === "job") await expect(related.first().getByRole("link", { name: "Open job →" })).toHaveAttribute("href", /^\/admin\/jobs\?jobId=[0-9a-f-]{36}$/);
  if (kind === "unit") await expect(related.first().getByRole("link", { name: "Open unit →" })).toHaveAttribute("href", /^\/admin\/units\?unitId=[0-9a-f-]{36}$/);
  await expect(related.first()).toContainText("Links open the existing detail screens");

  // Filter by actor → and Filter by ID → (the correlation) keep the open entry; the list then names the correlation
  await main.getByRole("button", { name: "Reset" }).click();
  await page.waitForURL((u) => !u.search);
  const open = card(page, /./).filter({ has: page.getByText("Role at the time", { exact: true }) }).first();
  await open.getByRole("button", { name: "Filter by actor →" }).click();
  await page.waitForURL(/actorId=/);
  const actor = new URL(page.url()).searchParams.get("actorId")!;
  await expect(main.getByRole("combobox", { name: "Actor" })).toHaveValue(actor);
  await open.getByRole("button", { name: "Filter by ID →" }).click();
  await page.waitForURL(/correlationId=/);
  const corr = new URL(page.url()).searchParams.get("correlationId")!;
  await expect(main.getByRole("heading", { name: `Correlation ${corr}`, exact: true })).toBeVisible();
  await expect(main).toContainText(/\d+ entr(y|ies) with this correlation ID/);

  // an ID that matches nothing looks the same whether it does not exist or is out of scope; a reversed period searches nothing
  await page.goto("/admin/audit?correlationId=00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "○ No entries with this correlation ID" })).toBeVisible();
  await expect(main).toContainText("This looks the same for an ID that does not exist and for one outside your managed scope.");
  await page.goto("/admin/audit?from=2026-09-10&to=2026-09-01");
  await expect(main.getByRole("heading", { name: "○ Nothing was searched" })).toBeVisible();
  await expect(main).toContainText("Period must be start < end, at most 366 days");
});

test("device events load only for a picked device and link the event's audit trail", async ({ page }) => {
  await page.goto("/admin/audit?tab=devices");
  const main = page.getByRole("main");
  await expect(main.getByRole("tab", { name: /^Device events/ })).toHaveAttribute("aria-selected", "true");
  await expect(main.getByRole("heading", { name: "○ Pick a device" })).toBeVisible(); // no fallback device (IR33)
  await expect(main.getByRole("heading", { name: "Device events · newest first" })).toHaveCount(0);
  await page.goto("/admin/audit?tab=devices&deviceId=00000000-0000-4000-8000-000000000000");
  await expect(main.getByRole("heading", { name: "○ That device is not in your list" })).toBeVisible();

  // the demo device AC-DEMO-0001 (Bedroom AC); a year of events so the seed's are in the period
  await page.goto("/admin/audit?tab=devices&from=2026-01-01&to=2026-12-31");
  await main.getByRole("combobox", { name: "Device" }).selectOption({ label: "AC-DEMO-0001 · Bedroom AC" });
  await page.waitForURL(/deviceId=[0-9a-f-]{36}/);
  const list = card(page, "Device events · newest first");
  await expect(list).toBeVisible();
  await expect(main).toContainText(/\d+ events?/);
  const rows = list.locator("button[aria-pressed]");
  if (await rows.count()) {
    await rows.first().click();
    await page.waitForURL(/eventId=[0-9a-f-]{36}/);
    const eventId = new URL(page.url()).searchParams.get("eventId")!;
    const detail = card(page, /./).filter({ has: page.getByRole("heading", { name: "Linked alerts and audit", exact: true }) });
    for (const label of ["Event type", "Evidence", "Sequence", "Unit at the time", "Occurred", "Recovery"]) await expect(detail.locator("dt", { hasText: new RegExp(`^${label}$`) })).toBeVisible();
    await expect(detail.getByRole("link", { name: "Filter log →" })).toHaveAttribute("href", `/admin/audit?targetKind=device_event&targetId=${eventId}`);
    await expect(detail).toContainText("Notes are added from Devices & models (device.write), not here.");
  } else {
    await expect(main.getByRole("heading", { name: "○ No device events in this period" })).toBeVisible();
  }
});
