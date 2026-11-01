// Energy-saving analysis (FR-A13, DD-A13, IR586): the analysis offers each baseline with its dates; a new baseline saved
// with nothing filled in asks for both ends of the period and takes focus to the first field to fix — the units —
// which stops below the sticky header. Nothing is saved.
import { test, expect } from "../../fixtures/test";

test("a baseline names its dates, and an empty new one takes focus to the first field to fix", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/admin/energy");
  const main = page.getByRole("main");
  const options = await main.getByRole("combobox", { name: "Baseline" }).evaluate((s: HTMLSelectElement) => [...s.options].map((o) => o.text));
  expect(options[0]).toBe("No baseline");
  for (const o of options.slice(1)) expect(o).toMatch(/^[0-9a-f]{8} · \d{4}-\d\d-\d\d → \d{4}-\d\d-\d\d · [\d,.]+ kWh · \d+ units?$/);

  await page.goto("/admin/energy?tab=baselines");
  await main.getByRole("button", { name: "New baseline" }).click();
  await main.getByRole("button", { name: "Create baseline" }).click();
  await expect(main.getByText("Enter the start and the end")).toBeVisible();
  await expect(main.getByText("Choose 1–100 units")).toBeVisible();
  const first = main.getByRole("checkbox").first();
  await expect(first).toBeFocused();
  expect((await first.boundingBox())!.y).toBeGreaterThanOrEqual(64); // not behind the 64 px sticky header
  await main.getByRole("button", { name: "Cancel" }).click();
  await expect(main.getByRole("button", { name: "Create baseline" })).toHaveCount(0);
});
