// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

const patch = vi.fn<(p: Record<string, string | null>) => void>();
// the English text with its {placeholders} filled, as the real translator does for the English display language
function fill(s: string, values: Record<string, string | number> = {}) {
  let text = s;
  for (const [k, x] of Object.entries(values)) text = text.split(`{${k}}`).join(String(x));
  return text;
}
vi.mock("@ac/web/components/I18n", () => ({ useT: () => fill }));
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => patch }));

import { TechUnitView } from "../app/technician/units/[id]/_components/unit-view";
import { TechUnitDemo } from "../app/technician/units/[id]/_components/unit-demo";
import type { UnitLive } from "../app/technician/units/[id]/_lib/load";

// the technician's unit page (FR-T02, FR-T03): the tab, the period and the chart's metrics go to the URL; the page
// says in words why a chip is off, which series have no readings or are cut, and what is missing
const line = (metric: string, over: Partial<UnitLive["chart"]["lines"][number]> = {}): UnitLive["chart"]["lines"][number] => ({
  metric, name: metric, unit: "°C", points: [1, null, 2], axis: "left", empty: false, count: null, gap: null, cut: null, ...over,
});
const tile = (label: string): UnitLive["tiles"]["temperature"] => ({ label, value: `${label} value`, sub: `${label} sub` });
function live(over: Partial<UnitLive> = {}): UnitLive {
  return {
    kind: "live", short: "unit-a01", name: "Bedroom AC", connection: "online",
    tabs: [{ id: "register", label: "Register" }, { id: "monitoring", label: "Monitoring" }],
    periods: [{ id: "1h", label: "1h" }, { id: "24h", label: "24h" }, { id: "7d", label: "7d" }],
    stopped: null,
    register: { rows: [["Model", "AC-Co v3"]], missing: false, note: "From the register" },
    components: [], componentCount: 0,
    tiles: { temperature: { ...tile("Temperature"), tone: "crit" }, power: { ...tile("Power"), tone: "warn" }, operation: tile("Operation"), connection: tile("Connection") },
    chart: { title: "Temperature · 24 h", labels: [], times: ["a", "b", "c"], lines: [line("temperature")] },
    others: [], events: [], history: [], alerts: [],
    controlHref: null, metric: "temperature",
    choices: [{ metric: "temperature", name: "Temperature", unit: "°C", next: [], on: true }, { metric: "humidity", name: "Humidity", unit: "%RH", next: ["temperature", "humidity"], on: false }],
    id: "unit-a01-xyz", back: "/technician/jobs/job-1", tab: "register", metrics: ["temperature"], period: "24h", alertsHref: "/technician/units/unit-a01-xyz/alerts",
    ...over,
  };
}
const tab = (name: string) => screen.getByRole("tab", { name });
afterEach(() => { cleanup(); patch.mockClear(); });

describe("TechUnitView", () => {
  it("opens Monitoring with the chart's metrics and goes back to the register without them", () => {
    const { rerender } = render(<TechUnitView live={live({ metrics: ["temperature", "humidity"] })} />);
    fireEvent.click(tab("Monitoring"));
    expect(patch).toHaveBeenLastCalledWith({ metric: "temperature,humidity" });
    rerender(<TechUnitView live={live({ tab: "monitoring" })} />);
    fireEvent.click(tab("Register"));
    expect(patch).toHaveBeenLastCalledWith({ metric: null });
  });

  it("puts a period other than 24 h in the URL and clears it for 24 h", () => {
    const { rerender } = render(<TechUnitView live={live()} />);
    fireEvent.click(tab("7d"));
    expect(patch).toHaveBeenLastCalledWith({ period: "7d" });
    rerender(<TechUnitView live={live({ period: "7d" })} />);
    fireEvent.click(tab("24h"));
    expect(patch).toHaveBeenLastCalledWith({ period: null });
  });

  it("toggles a metric chip and says why a chip is off", () => {
    const { rerender } = render(<TechUnitView live={live()} />);
    const group = screen.getByRole("group", { name: "Metrics in the chart" });
    expect(within(group).getByRole("button", { name: "Temperature" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(group).getByRole("button", { name: "Humidity" }));
    expect(patch).toHaveBeenLastCalledWith({ metric: "temperature,humidity" });
    expect(screen.queryByText("A chart shows two units at most — take one out first")).toBeNull();
    rerender(<TechUnitView live={live({ choices: [{ metric: "power", name: "Power", unit: "W", next: null, on: false }] })} />);
    const off = screen.getByRole("button", { name: "Power" });
    expect(off.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("A chart shows two units at most — take one out first")).toBeTruthy();
  });

  it("shows the register's empty states and the note on missing values", () => {
    render(<TechUnitView live={live({ register: { rows: [["Model", "Not registered"]], missing: true, note: "n" } })} />);
    expect(screen.getByText("Diagnostic control opens for a job you are assigned to on this AC (assigned or in progress).")).toBeTruthy();
    expect(screen.queryByRole("link", { name: /Diagnostics/ })).toBeNull();
    expect(screen.getByText("Missing values are never filled from similar models or today’s date.")).toBeTruthy();
    expect(screen.getByText("No components in this unit’s service scope.")).toBeTruthy();
    expect(screen.getByText("No jobs of yours on this unit.")).toBeTruthy();
    expect(screen.getByText("No open alerts.")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Back" }).getAttribute("href")).toBe("/technician/jobs/job-1");
  });

  it("lists components, history and alerts with their links, the diagnostics link and the stopped banner", () => {
    render(<TechUnitView live={live({
      stopped: "Updates stopped", controlHref: "/technician/units/u/control?jobId=j", componentCount: 2,
      components: [{ group: "indoor", title: "Indoor", names: "Filter, Fan" }],
      history: [{ id: "job-9", title: "Preventive", sub: "today", badge: { text: "Done", tone: "ok" } }],
      alerts: [{ id: "al-1", title: "Too hot", severity: "warning", sub: "30 °C", ack: "Nobody yet", href: "/technician/units/u/alerts?alertId=al-1" }],
    })} />);
    expect(screen.getByText("Updates stopped")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Diagnostics/ }).getAttribute("href")).toBe("/technician/units/u/control?jobId=j");
    expect(screen.getByText("Components (2)")).toBeTruthy();
    expect(screen.getByText("Filter, Fan")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Preventive/ }).getAttribute("href")).toBe("/technician/jobs/job-9");
    expect(screen.getByRole("link", { name: /Too hot/ }).getAttribute("href")).toBe("/technician/units/u/alerts?alertId=al-1");
    expect(screen.getByText("Open alerts (1)")).toBeTruthy();
  });

  it("names the series with no readings, the cut ones, their counts and gaps when the chart has two lines", () => {
    render(<TechUnitView live={live({ chart: { title: "t", labels: [], times: [], lines: [
      line("temperature", { count: "3 measurements", gap: "no data 03:00–05:00" }),
      line("humidity", { empty: true, points: [null], cut: "Showing the latest 1 of 9" }),
    ] } })} />);
    expect(screen.getByText("humidity: No valid readings in this period.")).toBeTruthy();
    expect(screen.getByText("humidity: Showing the latest 1 of 9")).toBeTruthy();
    expect(screen.getByText(/^temperature: 3 measurements · temperature: no data 03:00–05:00 · Gaps in missing data/)).toBeTruthy();
  });

  it("shows one box without a name when the only line has no readings", () => {
    render(<TechUnitView live={live({ chart: { title: "t", labels: [], times: [], lines: [line("temperature", { empty: true, points: [null], count: "0 measurements" })] } })} />);
    expect(screen.getByText("No valid readings in this period.")).toBeTruthy();
    expect(screen.getByText(/^0 measurements · Gaps/)).toBeTruthy();
  });

  it("charts another metric from its card, with or without a chip for it", () => {
    render(<TechUnitView live={live({
      tab: "monitoring",
      others: [
        { metric: "humidity", label: "Humidity", unit: "%RH", value: "60 %RH", sub: "observed 09:41", points: [50, 60], cut: true, tone: "warn" },
        { metric: "supplyAir", label: "Supply air", unit: "°C", value: "—", sub: "none", points: [null, null], cut: false },
      ],
      events: [{ id: "e1", at: "x", time: "09:12", title: "Alert raised", sub: "temp ≥ 30", badge: { text: "Alert", tone: "warn" } }, { id: "e2", at: "y", time: "05:10", title: "Data resumed", sub: "", badge: { text: "Connectivity", tone: "primary" } }],
    })} />);
    expect(screen.getByText("Earlier readings are not loaded")).toBeTruthy();
    const supply = screen.getByText("Supply air").closest("[data-slot=card]") ?? document.body;
    expect(within(supply instanceof HTMLElement ? supply : document.body).getByText("No valid readings in this period.")).toBeTruthy();
    const charts = screen.getAllByRole("button", { name: /Chart/ });
    fireEvent.click(charts[0] ?? document.body);
    expect(patch).toHaveBeenLastCalledWith({ metric: "temperature,humidity" });
    fireEvent.click(charts[1] ?? document.body);
    expect(patch).toHaveBeenLastCalledWith({ metric: "supplyAir" });
    expect(screen.getByText("09:12 · Alert raised")).toBeTruthy();
    expect(screen.getByText("temp ≥ 30")).toBeTruthy();
    expect(screen.getByText("Operation value")).toBeTruthy();
  });

  it("says when the period has no events, and shows the stopped banner and a disabled chip on Monitoring", () => {
    render(<TechUnitView live={live({ tab: "monitoring", stopped: "Updates stopped", choices: [{ metric: "power", name: "Power", unit: "W", next: null, on: false }] })} />);
    expect(screen.getByText("No alert or device events in this period.")).toBeTruthy();
    expect(screen.getByText("Updates stopped")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Power" }));
    expect(patch).not.toHaveBeenCalled();
    expect(screen.getByText("A chart shows two units at most — take one out first")).toBeTruthy();
    fireEvent.click(tab("1h"));
    expect(patch).toHaveBeenLastCalledWith({ period: "1h" });
  });

  it("charts a metric chip on Monitoring", () => {
    render(<TechUnitView live={live({ tab: "monitoring" })} />);
    fireEvent.click(screen.getByRole("button", { name: "Humidity" }));
    expect(patch).toHaveBeenLastCalledWith({ metric: "temperature,humidity" });
  });
});

describe("TechUnitDemo", () => {
  it("refuses a unit outside the assignments", () => {
    render(<TechUnitDemo id="unit-other-customer" />);
    expect(screen.getByText("This page isn’t available")).toBeTruthy();
  });

  it("shows “Not registered” for a unit without its register values and toggles the offline preview", () => {
    render(<TechUnitDemo id="unit-non-rto" />);
    expect(screen.getAllByText("Not registered")).toHaveLength(3);
    expect(screen.queryByText(/Communication lost/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "preview: communication lost" }));
    expect(screen.getByText(/Communication lost/)).toBeTruthy();
    expect(screen.getAllByText("Offline")).toHaveLength(2); // the badge and the Live card's Connection row
    fireEvent.click(tab("Monitoring"));
    expect(screen.getAllByText("Offline")).toHaveLength(2); // the badge and the Connection tile
  });

  it("opens Monitoring and switches the window", () => {
    render(<TechUnitDemo id="unit-a01" />);
    expect(screen.getByText("AC-Co / ventilation-demo v3")).toBeTruthy();
    fireEvent.click(tab("Monitoring"));
    expect(screen.getByText("Other metrics")).toBeTruthy();
    fireEvent.click(tab("7d"));
    expect(tab("7d").getAttribute("aria-selected")).toBe("true");
  });
});
