// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// The Phase 1A demo of Energy & cost (FR-C06): the units chosen (one always stays, up to four) and the figures summed
// over them, the chart or table, the period tabs and the export dialog.
import { EnergyDemo } from "../app/customer/energy/_components/energy-demo";

const tab = (name: string) => { fireEvent.click(screen.getByRole("tab", { name })); };
const unit = (name: string) => {
  const box = screen.getByText(name, { selector: "label" }).querySelector("[role=checkbox]");
  if (!box) throw new Error(`no checkbox ${name}`);
  return box;
};
const kpi = (label: string) => screen.getByText(label).parentElement?.parentElement?.textContent;

afterEach(() => { cleanup(); });

describe("EnergyDemo", () => {
  it("sums the chosen units; the last one cannot be taken out", () => {
    render(<EnergyDemo />);
    expect(kpi("Actual energy")).toContain("80.0kWhBedroom AC · 7 days · coverage 100%");
    expect(kpi("Savings vs baseline")).toContain("18.0kWh18.4% · 9.00 MYR vs 98.0 kWh baseline");
    expect(screen.getByText("80 kWh actual × factor")).toBeDefined();
    fireEvent.click(unit("Bedroom AC"));
    expect(unit("Bedroom AC").getAttribute("aria-checked")).toBe("true");
    fireEvent.click(unit("Kitchen AC"));
    expect(kpi("Actual energy")).toContain("131.9kWh2 units · 7 days");
    expect(screen.getByText("161 → 131.9 kWh")).toBeDefined();
    fireEvent.click(unit("Bedroom AC"));
    expect(kpi("Actual energy")).toContain("51.9kWhKitchen AC · 7 days");
    fireEvent.click(unit("Bedroom AC #2"));
    fireEvent.click(unit("Living room AC"));
    fireEvent.click(unit("Bedroom AC"));
    expect(kpi("Actual energy")).toContain("4 units");
  });

  it("switches to the table, the period and opens the export dialog", () => {
    render(<EnergyDemo />);
    tab("Table");
    expect(screen.getByRole("row", { name: /Thu 17/ }).textContent).toBe("Thu 1712.3141.7");
    tab("Chart");
    expect(screen.queryByRole("row", { name: /Thu 17/ })).toBeNull();
    tab("30d");
    expect(screen.getByRole("tab", { name: "30d" }).getAttribute("aria-selected")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Export" }));
    expect(screen.getByRole("dialog")).toBeDefined();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
