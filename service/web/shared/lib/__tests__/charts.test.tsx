// @vitest-environment jsdom
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { BarChart, LineChart, UtilBar } from "@ac/web/components/ui";

// jsdom lays nothing out: give the chart's box a size, as a browser would, so that Recharts draws (the legend stays 0 high)
const box = (width: number, height: number) => ({ width, height, top: 0, left: 0, right: width, bottom: height, x: 0, y: 0, toJSON: () => ({}) });
beforeAll(() => {
  globalThis.ResizeObserver ??= class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver;
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    return this.classList.contains("recharts-responsive-container") ? box(600, 200) : box(0, 0);
  });
});
afterAll(() => vi.restoreAllMocks());
afterEach(cleanup);

// the drawn pieces of a path: a move followed by at least one line (a lone move draws nothing)
const pieces = (d: string | null) => (d ?? "").split("M").filter((x) => x.includes("L")).length;

// The charts on shadcn/ui's chart and Recharts (DEC-75, IR403): what each one draws, and its name for assistive technology
describe("charts", () => {
  it("never joins a line across missing readings, and draws a reading between two gaps as a dot", () => {
    const { container } = render(<LineChart points={[1, 2, null, 4, 5, null, 7]} threshold={6} shadeGaps />);
    expect(screen.getByRole("application", { name: "Line chart" })).toBeTruthy(); // Recharts' keyboard layer moves the tooltip
    const line = container.querySelector(".recharts-line-curve");
    expect(pieces(line?.getAttribute("d") ?? null)).toBe(2); // 1–2 and 4–5; 7 stands alone
    expect(container.querySelectorAll("svg circle")).toHaveLength(1); // Recharts draws the dots in a layer of their own
    expect(container.querySelectorAll(".recharts-reference-area")).toHaveLength(2); // both gaps shaded
    expect(container.querySelectorAll(".recharts-reference-line")).toHaveLength(1); // the threshold
  });

  it("shades nothing and draws no gridline unless asked", () => {
    const { container } = render(<LineChart points={[3, null, 4, 5]} />);
    expect([container.querySelectorAll(".recharts-reference-area").length, container.querySelectorAll(".recharts-cartesian-grid").length]).toEqual([0, 0]);
  });

  it("draws one bar per label and series, and names the series in a legend", () => {
    const { container } = render(<BarChart labels={["Mon", "Tue", "Wed"]} series={[[1, 2, 3], [2, 3, 4]]} names={["7 days before", "Last 7 days"]} unit="kWh" />);
    expect(screen.getByRole("application", { name: "Bar chart" })).toBeTruthy();
    expect(container.querySelectorAll(".recharts-bar-rectangle")).toHaveLength(6);
    expect([...(container.querySelector(".recharts-legend-wrapper > div")?.children ?? [])].map((x) => x.textContent)).toEqual(["7 days before", "Last 7 days"]);
    expect(container.textContent).toContain("kWh");
  });

  it("labels a small scale with distinct ticks, not the same whole number twice", () => {
    const { container } = render(<BarChart labels={["a", "b"]} series={[[0.9, 0.4]]} />);
    // the value ticks are the numbers; the labels under the bars are the category names
    const ticks = [...container.querySelectorAll(".recharts-cartesian-axis-tick-value")].map((x) => x.textContent ?? "").filter((t) => /^\d/.test(t));
    expect(ticks.length).toBeGreaterThan(1);
    expect(new Set(ticks).size).toBe(ticks.length);
  });

  it("draws no legend for an unnamed series", () => {
    const { container } = render(<BarChart labels={["a", "b"]} series={[[1, 2]]} />);
    expect(container.querySelectorAll(".recharts-legend-wrapper")).toHaveLength(0);
  });

  it("names a progress bar and leaves an unknown value out", () => {
    render(<><UtilBar pct={40} tone="warn" label="Filter life" /><UtilBar pct={null} label="Unknown" /></>);
    const [known, unknown] = screen.getAllByRole("progressbar");
    expect([known.getAttribute("aria-label"), known.getAttribute("aria-valuenow"), unknown.getAttribute("aria-label"), unknown.getAttribute("aria-valuenow")]).toEqual(["Filter life", "40", "Unknown", null]);
  });
});
