// @vitest-environment jsdom
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { AirChartSeries, AirMetric, ApiVentilationLog, MetricCard } from "@ac/web/lib/air";

// Air quality (FR-C07) in API mode: the room and sensor pickers, the metric cards and chips that put a metric in the
// chart or take it out (each press from the last choice), the chart or table of the shown metrics, the texts of a metric
// without valid readings or without a sensor, the ventilation history, and the Log ventilation dialog (IR110)
type Logged = { ok: true; value: ApiVentilationLog } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({ logVentilation: vi.fn<(input: unknown) => Promise<Logged>>() }));
vi.mock("../app/customer/air-quality/actions", () => actions);
const patch = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => patch }));
const router = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
const runs = vi.hoisted((): { toasts: string[] } => ({ toasts: [] }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Logged>, ok: (v: ApiVentilationLog) => string, after?: () => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => {
      if (r.ok) { runs.toasts.push(ok(r.value)); after?.(); } else failed?.(r);
    });
  }] as const,
}));

import { AirQualityView, type AirLive } from "../app/customer/air-quality/_components/air-quality-view";

const card = (metric: AirMetric, label: string, value: string | null, over: Partial<MetricCard> = {}): MetricCard => ({
  metric, label, unit: metric === "co2" ? "ppm" : "%", value, feed: { text: "Live", tone: "ok", icon: "live" }, status: null, sub: `${label} sub`, advice: `${label} advice`, warn: false, ...over,
});
const line = (metric: AirMetric, name: string, unit: string, over: Partial<AirChartSeries> = {}): AirChartSeries => ({
  metric, name, unit, points: [1, null, 2], guide: null, axis: "left", hasSensor: true, valid: true, readings: 3, ...over,
});
const live = (over: Partial<AirLive> = {}): AirLive => ({
  rooms: [
    { key: "r-bed", spaceId: "s-bed", label: "Bedroom", units: [{ id: "u-1", name: "Bedroom AC" }, { id: "u-2", name: "Fan" }] },
    { key: "r-hall", spaceId: "s-hall", label: "Hall", units: [{ id: "u-3", name: "Hall AC" }] },
    { key: "r-empty", spaceId: null, label: "Store", units: [] },
  ],
  roomKey: "r-bed", unitId: "u-1", unitName: "Bedroom AC", path: "Home › Bedroom", spaceId: "s-bed", connection: "online", updated: "09:12", freshAir: false,
  cards: [
    card("co2", "CO2", "1000", { status: { text: "High", tone: "warn", icon: "warning" }, warn: true }),
    card("humidity", "Humidity", null),
  ],
  strip: { tone: "warn", title: "Ventilation recommended", text: "CO2 is high." }, clean: null,
  allergen: { title: "Allergen", badge: { text: "Not measured", tone: "unknown", icon: "info" }, text: "No sensor" },
  metric: "co2", metrics: ["co2"], period: "24h",
  chart: { title: "CO2 — last 24 hours", sub: "ppm", labels: ["a", "b"], times: ["t1", "t2", "t3"], series: [line("co2", "CO2", "ppm", { guide: 1000 })], gaps: false },
  tables: [
    { metric: "co2", label: "CO2", unit: "ppm", rows: [{ key: "k1", time: "09:10", value: "1000", note: "3 readings", gap: false }, { key: "k2", time: "09:05", value: "No data", note: "gap", gap: true }] },
    { metric: "humidity", label: "Humidity", unit: "%", rows: [{ key: "h1", time: "09:10", value: "60.0", note: "1 reading", gap: false }] },
  ],
  co2Now: "1000 ppm · 09:12",
  history: { rows: [{ id: "v-1", text: "Window opened · 15 min", co2: "CO2 980 ppm", by: "You", when: "Yesterday 18:10" }], total: 1 },
  ...over,
});
const logged = (co2: number | null): ApiVentilationLog => ({
  id: "v-2", spaceId: "s-bed", unitId: "u-1", method: "door_opened", durationMinutes: 30, co2AtLog: co2 === null ? null : { value: co2, unit: "ppm", observedAt: "2026-09-15T01:12:00Z" }, loggedByMembershipId: "m-1", loggedAt: "2026-09-15T01:13:00Z",
});
/** the charts drawn: each LineChart keeps a Recharts container, laid out or not */
const charts = () => document.querySelectorAll(".recharts-responsive-container").length;
const chips = () => within(screen.getByRole("group", { name: "Metrics in the chart" }));

beforeEach(() => {
  runs.toasts = [];
  patch.mockReset();
  router.refresh.mockReset();
  actions.logVentilation.mockReset().mockResolvedValue({ ok: true, value: logged(980.4) });
});
afterEach(() => { cleanup(); });

describe("AirQualityView", () => {
  it("without units, points to Units & locations", () => {
    render(<AirQualityView live={null} />);
    expect(screen.getByText("No air conditioners yet")).toBeDefined();
    expect(screen.getByRole("link", { name: "Units & locations" }).getAttribute("href")).toBe("/customer/properties");
  });

  it("picks a room by its first sensor, ignores a room without units, picks a sensor, links the control and refreshes", () => {
    render(<AirQualityView live={live()} />);
    const room = screen.getByRole("combobox", { name: "Room" });
    fireEvent.change(room, { target: { value: "r-hall" } });
    expect(patch).toHaveBeenLastCalledWith({ spaceId: "s-hall", unitId: "u-3" });
    fireEvent.change(room, { target: { value: "r-empty" } });
    expect(patch).toHaveBeenCalledOnce();
    const sensor = screen.getByRole("combobox", { name: "Sensor" });
    expect([...sensor.querySelectorAll("option")].map((o) => o.textContent)).toStrictEqual(["Sensor: Bedroom AC", "Sensor: Fan"]);
    fireEvent.change(sensor, { target: { value: "u-2" } });
    expect(patch).toHaveBeenLastCalledWith({ unitId: "u-2" });
    expect(screen.getByRole("link", { name: /Open Bedroom AC’s control/ }).getAttribute("href")).toBe("/customer/units/u-1");
    fireEvent.click(screen.getByRole("button", { name: /Refresh/ }));
    expect(router.refresh).toHaveBeenCalledOnce();
  });

  it("a room key the rooms do not hold falls back to the first room's sensors, and no unit hides the control link", () => {
    render(<AirQualityView live={live({ roomKey: "r-gone", unitId: "" })} />);
    const sensor = screen.getByRole("combobox", { name: "Sensor" });
    expect(sensor.querySelectorAll("option")).toHaveLength(2);
    expect(screen.queryByRole("link", { name: /control/ })).toBeNull();
  });

  it("no rooms at all leaves the sensor list empty", () => {
    render(<AirQualityView live={live({ rooms: [] })} />);
    expect(screen.getByRole("combobox", { name: "Sensor" }).querySelectorAll("option")).toHaveLength(0);
  });

  it("shows the cards, a missing value as a dash, and presses build on the last choice until the server answers", () => {
    const { rerender } = render(<AirQualityView live={live()} />);
    const co2 = screen.getByRole("button", { name: /^CO2.*1000/ });
    expect(co2.getAttribute("aria-pressed")).toBe("true");
    expect(co2.textContent).toContain("High");
    const humidity = screen.getByRole("button", { name: /^Humidity.+advice/ });
    expect(humidity.getAttribute("aria-pressed")).toBe("false");
    expect(humidity.textContent).toContain("—");
    expect(humidity.textContent).not.toContain("%");
    fireEvent.click(humidity);
    expect(patch).toHaveBeenLastCalledWith({ metric: "co2,humidity" });
    fireEvent.click(chips().getByRole("button", { name: "PM2.5" }));
    expect(patch).toHaveBeenLastCalledWith({ metric: "co2,humidity,pm25" });
    // the server's next page resets the choice
    rerender(<AirQualityView live={live({ metrics: ["humidity"] })} />);
    fireEvent.click(chips().getByRole("button", { name: "Temp" }));
    expect(patch).toHaveBeenLastCalledWith({ metric: "humidity,temperature" });
    expect(chips().getByRole("button", { name: "Humidity" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("All shows the four metrics, and pressed again keeps only the first", () => {
    render(<AirQualityView live={live({ metrics: ["co2", "pm25", "temperature", "humidity"] })} />);
    const all = chips().getByRole("button", { name: "All" });
    expect(all.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(all);
    expect(patch).toHaveBeenLastCalledWith({ metric: "co2" });
    fireEvent.click(chips().getByRole("button", { name: "All" }));
    expect(patch).toHaveBeenLastCalledWith({ metric: "co2,pm25,temperature,humidity" });
  });

  it("changes the period, and the table shows only the shown metrics' tables, a gap muted", () => {
    render(<AirQualityView live={live()} />);
    fireEvent.click(screen.getByRole("tab", { name: "7d" }));
    expect(patch).toHaveBeenLastCalledWith({ period: "7d" });
    expect(screen.getByText("- - 1000 ppm ventilation guide")).toBeDefined();
    expect(screen.getByText("— CO2 (ppm)")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: "Table" }));
    const regions = screen.getAllByRole("region");
    expect(regions.map((r) => r.getAttribute("aria-label"))).toStrictEqual(["CO2 — last 24 hours"]);
    expect(within(regions[0] ?? document.body).getByText("No data").className).toContain("text-muted-foreground");
    expect(screen.queryByText("60.0")).toBeNull();
  });

  it("names each table by its metric when several are shown, and stacks three units into separate charts", () => {
    const series = [line("co2", "CO2", "ppm", { guide: 1000 }), line("humidity", "Humidity", "%"), line("pm25", "PM2.5", "µg/m³", { guide: 35 })];
    render(<AirQualityView live={live({ metrics: ["co2", "humidity", "pm25"], chart: { ...live().chart, series, gaps: true } })} />);
    expect(screen.getByText("CO2 (ppm)")).toBeDefined();
    expect(screen.getByText("PM2.5 (µg/m³)")).toBeDefined();
    expect(screen.getByText("- - 35 µg/m³ cleaning guide")).toBeDefined();
    expect(charts()).toBe(3);
    expect(screen.queryByText("— CO2 (ppm)")).toBeNull(); // the stacked charts carry headings, not the legend line
    expect(screen.getByText("No data (gap, not joined)")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: "Table" }));
    expect(screen.getAllByRole("region").map((r) => r.getAttribute("aria-label"))).toStrictEqual(["CO2 — last 24 hours · CO2", "CO2 — last 24 hours · Humidity"]);
  });

  it("two units share one chart without a legend line", () => {
    const series = [line("co2", "CO2", "ppm"), line("humidity", "Humidity", "%", { axis: "right" })];
    render(<AirQualityView live={live({ metrics: ["co2", "humidity"], chart: { ...live().chart, series } })} />);
    expect(charts()).toBe(1);
    expect(screen.queryByText("— CO2 (ppm)")).toBeNull();
    expect(screen.queryByText("CO2 (ppm)")).toBeNull();
  });

  it("says when a metric has no valid readings (none, one, many) or no sensor, and draws no chart then", () => {
    const series = [
      line("co2", "CO2", "ppm", { valid: false, readings: 0, points: [null] }), line("pm25", "PM2.5", "µg/m³", { valid: false, readings: 1, points: [null] }),
      line("humidity", "Humidity", "%", { valid: false, readings: 4, points: [null] }), line("temperature", "Temperature", "°C", { hasSensor: false, valid: false, readings: 0 }),
    ];
    render(<AirQualityView live={live({ chart: { ...live().chart, series } })} />);
    expect(screen.getByText("No valid CO2 readings in this window — missing data is never drawn as a line.")).toBeDefined();
    expect(screen.getByText("No valid PM2.5 readings in this window (1 reading without a valid value) — missing data is never drawn as a line.")).toBeDefined();
    expect(screen.getByText("No valid Humidity readings in this window (4 readings without a valid value) — missing data is never drawn as a line.")).toBeDefined();
    expect(screen.getByText("Bedroom AC has no Temperature sensor — nothing to chart.")).toBeDefined();
    expect(charts()).toBe(0);
    fireEvent.click(screen.getByRole("tab", { name: "Table" }));
    expect(screen.queryByRole("region")).toBeNull();
  });

  it("shows the cleaning note, the history count, Showing the latest, and an empty history", () => {
    const { rerender } = render(<AirQualityView live={live({ clean: "Clean the filter", history: { rows: live().history?.rows ?? [], total: 3 } })} />);
    expect(screen.getByText("Clean the filter")).toBeDefined();
    expect(screen.getByRole("link", { name: "Maintenance" }).getAttribute("href")).toBe("/customer/maintenance");
    expect(screen.getByText(/Home › Bedroom · 3 logs · manual records/)).toBeDefined();
    expect(screen.getByText("Showing the latest 1.")).toBeDefined();
    expect(screen.getByText("Window opened · 15 min")).toBeDefined();
    rerender(<AirQualityView live={live({ history: { rows: [], total: 0 } })} />);
    expect(screen.getByText("No ventilation logged in this room yet.")).toBeDefined();
    rerender(<AirQualityView live={live()} />);
    expect(screen.getByText(/Home › Bedroom · 1 log · manual records/)).toBeDefined();
    expect(screen.queryByText(/Showing the latest/)).toBeNull();
  });

  it("a unit outside a room cannot log ventilation and says why", () => {
    render(<AirQualityView live={live({ spaceId: null, history: null, strip: { tone: "primary", title: "Air is fine", text: "Nothing to do." } })} />);
    const button = screen.getByRole("button", { name: "Log ventilation" });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Logs belong to a room — this unit is not in one")).toBeDefined();
    expect(screen.getByText(/ask HQ to place this unit in one/)).toBeDefined();
    expect(screen.getByText("Ventilation logs belong to a room — Bedroom AC is not in one.")).toBeDefined();
    fireEvent.click(button);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("logs ventilation with the chosen method and duration and says the CO2 it recorded", async () => {
    render(<AirQualityView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: "Log ventilation" }));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("1000 ppm · 09:12")).toBeDefined();
    fireEvent.change(dialog.getByRole("combobox", { name: "Method" }), { target: { value: "door_opened" } });
    fireEvent.change(dialog.getByRole("combobox", { name: "Duration" }), { target: { value: "30" } });
    fireEvent.click(dialog.getByRole("button", { name: "Save log" }));
    await waitFor(() => { expect(screen.queryByRole("dialog")).toBeNull(); });
    expect(actions.logVentilation).toHaveBeenCalledWith({ spaceId: "s-bed", unitId: "u-1", method: "door_opened", durationMinutes: 30 });
    expect(runs.toasts).toStrictEqual(["Ventilation logged · CO2 980 ppm recorded"]);
  });

  it("without a current CO2 the dialog and the toast say none is recorded, and Cancel closes", async () => {
    actions.logVentilation.mockResolvedValue({ ok: true, value: logged(null) });
    render(<AirQualityView live={live({ co2Now: null })} />);
    fireEvent.click(screen.getByRole("button", { name: "Log ventilation" }));
    expect(screen.getByText("Not measured — the log keeps no CO2 value")).toBeDefined();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Save log" }));
    await waitFor(() => { expect(runs.toasts).toStrictEqual(["Ventilation logged · no current CO2 reading recorded"]); });
    fireEvent.click(screen.getByRole("button", { name: "Log ventilation" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("a refused log keeps the dialog and shows the method and duration errors", async () => {
    actions.logVentilation.mockResolvedValue({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { method: "invalid", durationMinutes: "out_of_range" } });
    render(<AirQualityView live={live()} />);
    fireEvent.click(screen.getByRole("button", { name: "Log ventilation" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Save log" }));
    await waitFor(() => { expect(screen.getByText("Choose a method")).toBeDefined(); });
    expect(screen.getByText("1–240 minutes")).toBeDefined();
    expect(screen.getByRole("dialog")).toBeDefined();
  });
});

// jsdom lays nothing out: give the chart's box a size, as a browser would, so that Recharts draws
const box = (width: number, height: number) => ({ width, height, top: 0, left: 0, right: width, bottom: height, x: 0, y: 0, toJSON: () => ({}) });
// Recharts' keyboard layer moves the tooltip to the second point
const hover = (layer: HTMLElement) => { fireEvent.focus(layer); fireEvent.keyDown(layer, { key: "ArrowRight" }); };

// IR44 in the tooltip: a value is rounded by its metric, not shown as received
describe("AirQualityView chart tooltip", () => {
  beforeAll(() => {
    vi.stubGlobal("ResizeObserver", class { observe = vi.fn(); unobserve = vi.fn(); disconnect = vi.fn(); });
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      return this.classList.contains("recharts-responsive-container") ? box(600, 200) : box(0, 0);
    });
  });
  afterAll(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  it("rounds a CO2 average to a whole ppm", () => {
    render(<AirQualityView live={live({ chart: { ...live().chart, series: [line("co2", "CO2", "ppm", { points: [800.2, 912.6, 950] })] } })} />);
    hover(screen.getByRole("application", { name: "Line chart" }));
    expect(document.body.textContent).toContain("913 ppm");
    expect(document.body.textContent).not.toContain("912.6");
  });
});
