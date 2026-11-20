// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { SummaryView } from "@ac/web/lib/energy";
import type { ReportFile, ReportInput } from "../app/customer/energy/actions";
import type { EnergyLive } from "../app/customer/energy/_components/energy-view";

// Energy & cost (FR-C06, FR-C16) in API mode: the period tabs and custom range, the baseline and unit pickers (up to 4),
// the KPIs, the daily chart or table for one unit or a comparison, the text for a period without valid readings, the
// highest-use list (IR803) and the report export with the monthly e-mail choice.
type Exported = { ok: true; value: ReportFile } | ({ ok: false } & ActionFailure);
type MonthlyEmail = { locale: string; timezone: string; value: boolean } | null;
const actions = vi.hoisted(() => ({ exportReport: vi.fn<(input: ReportInput, monthlyEmail: MonthlyEmail) => Promise<Exported>>() }));
vi.mock("../app/customer/energy/actions", () => actions);
const patch = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => patch }));
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Exported>, ok: string, after?: (v: ReportFile) => void) => {
    void fn().then((r) => {
      if (!r.ok) return;
      runs.toasts.push(ok);
      after?.(r.value);
    });
  }] as const,
}));

import { EnergyView } from "../app/customer/energy/_components/energy-view";

const view = (over: Partial<SummaryView> = {}): SummaryView => ({
  actual: "30.0 kWh", baseline: "70.0 kWh", difference: "Reduction 40.0 kWh", percent: "Reduction 57.1%", cost: "15.00 MYR", savedCost: "Reduction 20.00 MYR", emissions: "15.0 kgCO₂e",
  savedEmissions: "Reduction 20.0 kgCO₂e", coverage: "100%", conditions: [["Tariff", "demo-v1"]], warnings: [], comparable: true, increase: false, rate: "0.50 MYR/kWh", ...over,
});
const day = (label: string, actual: number | null, baseline: number | null = 10) => ({ label, actual, baseline });
const live = (over: Partial<EnergyLive> = {}): EnergyLive => ({
  units: [{ id: "u-1", name: "Bedroom AC" }, { id: "u-2", name: "Kitchen AC" }, { id: "u-3", name: "Hall AC" }, { id: "u-4", name: "Study AC" }, { id: "u-5", name: "Porch AC" }],
  unitIds: ["u-1"], baselines: [{ id: "bl-1", label: "demo_fixed · 70.0 kWh" }], baselineId: "bl-1",
  period: { kind: "7d", from: "2026-09-08T16:00:00.000Z", to: "2026-09-15T01:12:00.000Z", label: "Sep 9 – Sep 15 (7 days) · Asia/Kuala_Lumpur", days: 7 },
  summary: view(), raw: { kWh: 30, baseline: 70, tariff: "demo-v1", factor: "MY 2026 · 0.5 kgCO₂e/kWh (demo factor)", coverage: 1 },
  daily: [day("Mon 14", 0), day("Tue 15", 30)], top: [], isTopDefault: false, compare: null,
  properties: [{ id: "p-1", name: "Home" }, { id: "p-2", name: "Office" }], prefs: { locale: "ms", timezone: "Asia/Kuala_Lumpur", monthlyReportEmail: false },
  lastMonth: "2026-08", query: "unitIds=u-1&period=7d&baselineId=bl-1", ...over,
});
const file: ReportFile = { fileName: "energy-2026-08.csv", mime: "text/csv", size: 2048, generatedAt: "2026-09-15T01:13:00Z", isDemo: true };
const tab = (name: string) => { fireEvent.click(screen.getByRole("tab", { name })); };
/** the charts drawn: each BarChart keeps a Recharts container */
const charts = () => document.querySelectorAll(".recharts-responsive-container").length;
const headers = () => screen.getAllByRole("columnheader").map((h) => h.textContent);

beforeEach(() => {
  runs.toasts = [];
  patch.mockReset();
  actions.exportReport.mockReset().mockResolvedValue({ ok: true, value: file });
});
afterEach(() => { cleanup(); });

describe("EnergyView", () => {
  it("without units, says energy appears once HQ adds them", () => {
    render(<EnergyView live={live({ units: [] })} />);
    expect(screen.getByText(/No air conditioner is registered yet/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Export" })).toBeNull();
  });

  it("one unit against a baseline: the KPIs, the chart with a day of 0 kWh, the table with the baseline and the carbon card", () => {
    // every day measured 0 kWh (the AC stayed off): the days are measured, so the chart is drawn
    render(<EnergyView live={live({ summary: view({ warnings: ["Partial coverage", "Modeled baseline"] }), daily: [day("Mon 14", 0), day("Tue 15", 0)] })} />);
    expect(screen.getByText("Bedroom AC · 7 days · coverage 100%")).toBeDefined();
    expect(screen.getByText("avg 0.50 MYR/kWh · demo-v1 · taxes excluded")).toBeDefined();
    expect(screen.getByText("Reduction 57.1% · Reduction 20.00 MYR vs 70.0 kWh baseline")).toBeDefined();
    expect(screen.getByText("All expected minutes observed")).toBeDefined();
    expect(screen.getByText("Daily energy — Actual vs Baseline")).toBeDefined();
    expect(screen.getByText("kWh per day · Bedroom AC · baseline shown as an even daily share")).toBeDefined();
    // a day with 0 kWh is a measured day, so the chart is drawn
    expect(screen.queryByText(/No valid power readings/)).toBeNull();
    expect(charts()).toBe(1);
    expect(screen.getByText("Partial coverage · Modeled baseline")).toBeDefined();
    expect(screen.getByText("Reduction 20.0 kgCO₂e").className).toContain("text-ok");
    expect(screen.getByText("70.0 kWh → 30.0 kWh")).toBeDefined();
    expect(screen.getByText("30.0 kWh actual × factor")).toBeDefined();
    expect(screen.getByText("Same period & units as above · MY 2026 · 0.5 kgCO₂e/kWh (demo factor)")).toBeDefined();
    expect(screen.getByRole("link", { name: /Offset these emissions/ }).getAttribute("href")).toBe("/customer/energy/offsets?unitIds=u-1&period=7d&baselineId=bl-1");
    tab("Table");
    expect(headers()).toStrictEqual(["Day", "Actual (kWh)", "Baseline (kWh)"]);
    expect(screen.getByRole("row", { name: /Mon 14/ }).textContent).toBe("Mon 140.010.0");
  });

  it("an increase warns; no valid readings at all says so; a missing factor and a gap in the table show as such", () => {
    render(<EnergyView live={live({
      summary: view({ increase: true, coverage: "40%", rate: null, cost: "—" }), raw: { kWh: null, baseline: 70, tariff: "demo-v1", factor: null, coverage: 0.4 },
      daily: [day("Mon 14", null, null), day("Tue 15", null)],
    })} />);
    expect(screen.getByText(/No valid power readings in this period.*coverage 40%/)).toBeDefined();
    expect(charts()).toBe(0);
    expect(screen.getByText("Missing readings are not filled in")).toBeDefined();
    expect(screen.getByText("demo-v1 · taxes excluded")).toBeDefined();
    expect(screen.getByText("Reduction 20.0 kgCO₂e").className).toContain("text-warn");
    expect(screen.getByText("no valid readings")).toBeDefined();
    expect(screen.getByText("Same period & units as above · emission factor missing")).toBeDefined();
    tab("Table");
    expect(screen.getByRole("row", { name: /Mon 14/ }).textContent).toBe("Mon 14——");
  });

  it("without a baseline: only actual energy, no baseline column, and the savings say why", () => {
    render(<EnergyView live={live({ baselineId: null, baselines: [], raw: { kWh: 30, baseline: null, tariff: "demo-v1", factor: null, coverage: 1 }, summary: view({ comparable: false }) })} />);
    expect(screen.getByText("Daily energy")).toBeDefined();
    expect(screen.getByText("kWh per day · Bedroom AC")).toBeDefined();
    expect(screen.getByText("No baseline selected")).toBeDefined();
    expect(screen.getByText("needs a comparable baseline")).toBeDefined();
    expect(screen.getByText("Reduction 20.0 kgCO₂e").className).toContain("text-muted-foreground");
    expect(screen.getByText(/No baseline covers exactly these units/)).toBeDefined();
    expect(charts()).toBe(1);
    tab("Table");
    expect(headers()).toStrictEqual(["Day", "Actual (kWh)"]);
  });

  it("a chosen baseline that is not comparable says it cannot be calculated", () => {
    render(<EnergyView live={live({ raw: { kWh: 30, baseline: null, tariff: "demo-v1", factor: null, coverage: 1 }, summary: view({ comparable: false }) })} />);
    expect(screen.getByText("Cannot calculate — baseline not comparable for this period")).toBeDefined();
    expect(screen.getByText("Daily energy — Actual vs Baseline")).toBeDefined();
  });

  it("the period tabs, the baseline and the units patch the URL; a unit is added up to four", () => {
    render(<EnergyView live={live()} />);
    tab("30d");
    expect(patch).toHaveBeenLastCalledWith({ period: "30d", from: null, to: null });
    tab("7d");
    expect(patch).toHaveBeenCalledOnce(); // 7d is the period shown
    fireEvent.change(screen.getByRole("combobox", { name: "Baseline" }), { target: { value: "none" } });
    expect(patch).toHaveBeenLastCalledWith({ baselineId: "none" });
    const add = screen.getByRole("combobox", { name: "Compare unit" });
    expect([...add.querySelectorAll("option")].map((o) => o.textContent)).toStrictEqual(["Add a unit to compare", "Kitchen AC", "Hall AC", "Study AC", "Porch AC"]);
    fireEvent.change(add, { target: { value: "" } });
    expect(patch).toHaveBeenCalledTimes(2); // the empty choice adds nothing
    fireEvent.change(add, { target: { value: "u-2" } });
    expect(patch).toHaveBeenLastCalledWith({ unitIds: "u-1,u-2", baselineId: null });
    expect(screen.queryByRole("button", { name: /^Remove/ })).toBeNull();
    expect(screen.getByText("up to 4 units")).toBeDefined();
  });

  it("the period tabs reach today and the 7-day default from another period", () => {
    render(<EnergyView live={live({ period: { ...live().period, kind: "30d" } })} />);
    tab("7d");
    expect(patch).toHaveBeenLastCalledWith({ period: null, from: null, to: null });
    tab("Today");
    expect(patch).toHaveBeenLastCalledWith({ period: "today", from: null, to: null });
  });

  it("a custom range is typed in Kuala Lumpur time and applied once a bound changes; its error shows", () => {
    render(<EnergyView live={live({ period: { ...live().period, kind: "custom", error: "At most 366 days" } })} />);
    expect(screen.getByText("At most 366 days")).toBeDefined();
    const apply = screen.getByRole("button", { name: "Apply" });
    expect(apply.hasAttribute("disabled")).toBe(true);
    const [fromDate, toDate] = screen.getAllByLabelText("Date");
    if (!fromDate || !toDate) throw new Error("no date inputs");
    expect(fromDate).toHaveProperty("value", "2026-09-09");
    fireEvent.change(fromDate, { target: { value: "2026-09-01" } });
    fireEvent.change(toDate, { target: { value: "2026-09-10" } });
    expect(apply.hasAttribute("disabled")).toBe(false);
    fireEvent.click(apply);
    expect(patch).toHaveBeenLastCalledWith({ period: "custom", from: "2026-09-01T00:00", to: "2026-09-10T09:12" });
    tab("7d");
    expect(patch).toHaveBeenLastCalledWith({ period: null, from: null, to: null });
  });

  it("compares units: the per-unit chart up to 7 days, its table, and a unit removed", () => {
    const rows = [
      { unitId: "u-1", name: "Bedroom AC", actual: "30.0", baseline: "70.0", difference: "Reduction 40.0 kWh", tone: "ok" as const, coverage: "100%" },
      { unitId: "u-2", name: "Kitchen AC", actual: "—", baseline: "—", difference: "No baseline for this unit", tone: "muted" as const, coverage: "—" },
    ];
    render(<EnergyView live={live({ unitIds: ["u-1", "u-2"], compare: { rows, series: [[0, 30], [1, 2]] } })} />);
    expect(screen.getByText("Daily actual energy by unit")).toBeDefined();
    expect(screen.getByText("kWh per day · max 4 units · baselines are compared in the table")).toBeDefined();
    expect(screen.getByText("2 units · 7 days · coverage 100%")).toBeDefined();
    expect(screen.getByText(/Totals across units are shown only/)).toBeDefined();
    expect(charts()).toBe(1);
    tab("Table");
    expect(headers()).toStrictEqual(["Unit", "Actual (kWh)", "Baseline (kWh)", "Difference vs baseline", "Coverage"]);
    expect(screen.getByRole("row", { name: /Kitchen AC/ }).textContent).toBe("Kitchen AC——No baseline for this unit—");
    fireEvent.click(screen.getByRole("button", { name: "Remove Kitchen AC" }));
    expect(patch).toHaveBeenLastCalledWith({ unitIds: "u-1", baselineId: null });
  });

  it("over more than 7 days the comparison has no chart; with four units no unit can be added", () => {
    render(<EnergyView live={live({ unitIds: ["u-1", "u-2", "u-3", "u-4"], compare: { rows: [], series: null }, daily: [day("Mon 14", 3)] })} />);
    expect(screen.getByText("The per-unit daily chart covers up to 7 days — see the table for this period.")).toBeDefined();
    expect(charts()).toBe(0);
    expect(screen.queryByRole("combobox", { name: "Compare unit" })).toBeNull();
  });

  it("a comparison without its rows yet shows the daily table; an unknown unit is a dash", () => {
    render(<EnergyView live={live({ unitIds: ["u-1", "u-9"], compare: null })} />);
    tab("Table");
    expect(headers()).toStrictEqual(["Day", "Actual (kWh)", "Baseline (kWh)"]);
    tab("Chart");
    expect(screen.getByText("The per-unit daily chart covers up to 7 days — see the table for this period.")).toBeDefined();
    expect(screen.getByRole("button", { name: "Remove" })).toBeDefined();
  });

  it("the highest-use list: shown from two ACs, Show opens one, Compare adds one until four are shown", () => {
    const top = [
      { unitId: "u-1", name: "Bedroom AC", kWh: 30, share: 0.75, coverage: 1 },
      { unitId: "u-2", name: "Kitchen AC", kWh: 10, share: 0.25, coverage: 0.5 },
      { unitId: "u-3", name: "Hall AC", kWh: 0.1, share: 0, coverage: null },
    ];
    const { rerender } = render(<EnergyView live={live({ top, isTopDefault: true })} />);
    expect(screen.getByText("Showing the AC that used the most in this period · up to 4 units")).toBeDefined();
    const list = within(screen.getByRole("table", { name: "" }).closest("[data-slot=card]") ?? document.body);
    expect(list.getByRole("row", { name: /Bedroom AC/ }).textContent).toBe("Bedroom AC30.0 kWh75%100%Show");
    expect(list.getByText("50%").className).toBe("text-warn");
    expect(list.getAllByText("—")).toHaveLength(1);
    const bedroom = within(list.getByRole("row", { name: /Bedroom AC/ }));
    expect(bedroom.getByRole("button", { name: "Show" }).hasAttribute("disabled")).toBe(true);
    const kitchen = within(list.getByRole("row", { name: /Kitchen AC/ }));
    fireEvent.click(kitchen.getByRole("button", { name: "Show" }));
    expect(patch).toHaveBeenLastCalledWith({ unitIds: "u-2", baselineId: null });
    fireEvent.click(kitchen.getByRole("button", { name: "Compare" }));
    expect(patch).toHaveBeenLastCalledWith({ unitIds: "u-1,u-2", baselineId: null });
    rerender(<EnergyView live={live({ top, unitIds: ["u-1", "u-2", "u-4", "u-5"], compare: { rows: [], series: null } })} />);
    expect(screen.queryByRole("button", { name: "Compare" })).toBeNull();
    expect(screen.getAllByRole("button", { name: "Show" }).every((b) => !b.hasAttribute("disabled"))).toBe(true);
    rerender(<EnergyView live={live({ top: top.slice(0, 1) })} />);
    expect(screen.queryByText("Highest use in this period")).toBeNull();
  });

  it("a one-day period says day; a unit no longer listed is named by its id in the chart and a dash in the subject", () => {
    const { rerender } = render(<EnergyView live={live({ period: { ...live().period, kind: "today", days: 1 } })} />);
    expect(screen.getByText("Bedroom AC · 1 day · coverage 100%")).toBeDefined();
    rerender(<EnergyView live={live({ unitIds: ["u-1", "u-9"], compare: { rows: [], series: [[1, 2], [3, 4]] } })} />);
    expect(charts()).toBe(1);
    expect(screen.getByRole("button", { name: "Remove" })).toBeDefined();
    rerender(<EnergyView live={live({ unitIds: [] })} />);
    expect(screen.getByText("— · 7 days · coverage 100%")).toBeDefined();
  });

  it("without a summary only the pickers and the refusal are shown", () => {
    render(<EnergyView live={live({ summary: null, error: "Unit ids — boundary mismatch" })} />);
    expect(screen.getByText("Unit ids — boundary mismatch")).toBeDefined();
    expect(screen.queryByText("Actual energy")).toBeNull();
  });
});

const openExport = () => {
  fireEvent.click(screen.getByRole("button", { name: "Export" }));
  return within(screen.getByRole("dialog"));
};
const box = (dialog: ReturnType<typeof openExport>, name: string) => {
  const label = dialog.getByText(name).closest("label");
  const check = label?.querySelector("[role=checkbox]");
  if (!check) throw new Error(`no checkbox ${name}`);
  return check;
};
/** the footer's Close (the dialog's corner button has the same name) */
const close = (dialog: ReturnType<typeof openExport>) => {
  const button = dialog.getAllByRole("button", { name: "Close" }).find((b) => b.textContent === "Close");
  if (!button) throw new Error("no Close");
  fireEvent.click(button);
};

describe("EnergyView export", () => {
  it("offers the 12 completed months, needs a location and a section, and downloads with the monthly e-mail turned on", async () => {
    render(<EnergyView live={live()} />);
    const dialog = openExport();
    const month = dialog.getByRole("combobox");
    expect([...month.querySelectorAll("option")].map((o) => o.getAttribute("value"))).toStrictEqual([
      "2026-08", "2026-07", "2026-06", "2026-05", "2026-04", "2026-03", "2026-02", "2026-01", "2025-12", "2025-11", "2025-10", "2025-09",
    ]);
    expect(month.querySelector("option")?.textContent).toBe("August 2026");
    fireEvent.change(month, { target: { value: "2026-07" } });
    const download = dialog.getByRole("button", { name: "Download" });
    fireEvent.click(box(dialog, "Home"));
    fireEvent.click(box(dialog, "Office"));
    expect(dialog.getByText("Choose at least one location")).toBeDefined();
    expect(download.hasAttribute("disabled")).toBe(true);
    fireEvent.click(box(dialog, "Office"));
    for (const s of ["Energy & cost by AC (kWh, MYR)", "Comparison with last month", "CO₂ and offsets"]) fireEvent.click(box(dialog, s));
    expect(dialog.getByText("Choose at least one section")).toBeDefined();
    fireEvent.click(box(dialog, "Alerts & maintenance done"));
    fireEvent.click(dialog.getByRole("button", { name: "CSV (raw data)" }));
    fireEvent.click(box(dialog, "Email this report to me every month"));
    fireEvent.click(download);
    await waitFor(() => { expect(dialog.getByText("Report ready")).toBeDefined(); });
    expect(actions.exportReport).toHaveBeenCalledWith({ propertyIds: ["p-2"], month: "2026-07", sections: ["alerts_maintenance"], format: "csv" }, { locale: "ms", timezone: "Asia/Kuala_Lumpur", value: true });
    expect(runs.toasts).toStrictEqual(["Report ready"]);
    expect(dialog.getByText("energy-2026-08.csv · 2 KB · CSV — a demo file: nothing is downloaded or e-mailed.")).toBeDefined();
    expect(dialog.getByText("Monthly e-mail turned on in your preferences.")).toBeDefined();
    expect(dialog.queryByRole("button", { name: "Download" })).toBeNull();
    close(dialog);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("an unchanged e-mail choice stores nothing; turning it off says so; an unknown locale is sent as English", async () => {
    render(<EnergyView live={live({ prefs: { locale: "xx", timezone: "Asia/Kuala_Lumpur", monthlyReportEmail: true } })} />);
    let dialog = openExport();
    fireEvent.click(box(dialog, "Email this report to me every month"));
    actions.exportReport.mockResolvedValueOnce({ ok: true, value: { ...file, mime: "application/pdf", size: 10 } });
    fireEvent.click(dialog.getByRole("button", { name: "Download" }));
    await waitFor(() => { expect(dialog.getByText("Monthly e-mail turned off in your preferences.")).toBeDefined(); });
    expect(dialog.getByText(/1 KB · PDF/)).toBeDefined();
    expect(actions.exportReport.mock.lastCall?.[1]).toStrictEqual({ locale: "en", timezone: "Asia/Kuala_Lumpur", value: false });
    close(dialog);
    dialog = openExport();
    fireEvent.click(dialog.getByRole("button", { name: "Download" }));
    await waitFor(() => { expect(actions.exportReport).toHaveBeenCalledTimes(2); });
    expect(actions.exportReport.mock.lastCall).toStrictEqual([{ propertyIds: ["p-1", "p-2"], month: "2026-08", sections: ["energy_cost", "month_comparison", "co2_offsets"], format: "pdf" }, null]);
  });

  it("without preferences there is no e-mail choice; a refused export keeps the form", async () => {
    actions.exportReport.mockResolvedValueOnce({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: {} });
    render(<EnergyView live={live({ prefs: null })} />);
    const dialog = openExport();
    expect(dialog.queryByText("Email this report to me every month")).toBeNull();
    fireEvent.click(dialog.getByRole("button", { name: "Download" }));
    await waitFor(() => { expect(actions.exportReport).toHaveBeenCalledOnce(); });
    expect(actions.exportReport.mock.lastCall?.[1]).toBeNull();
    expect(dialog.getByRole("button", { name: "Download" })).toBeDefined();
    expect(runs.toasts).toStrictEqual([]);
  });
});
