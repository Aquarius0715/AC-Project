// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo of air quality (FR-C07): the metric, window and view tabs, the missing-data state that never shows a
// current normal value and hides the ventilation advice, and the Log ventilation dialog that adds a record on top.
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { AirQualityDemo } from "../app/customer/air-quality/_components/air-quality-demo";

const tab = (name: string) => { fireEvent.click(screen.getByRole("tab", { name })); };
/** the Ventilation history card: its rows, one div each with the record and its time */
function historyRows() {
  const card = screen.getByRole("heading", { name: "Ventilation history" }).closest<HTMLElement>("[data-slot=card]");
  if (card === null) throw new TypeError("no Ventilation history card");
  return [...card.querySelectorAll(":scope [data-slot=card-content] > div")].map((r) => [...r.children].map((c) => c.textContent));
}

beforeEach(() => { toast.mockReset(); });
afterEach(() => { cleanup(); });

describe("AirQualityDemo", () => {
  it("switches the metric, the window and the chart or table", () => {
    render(<AirQualityDemo />);
    expect(screen.getByText("CO2 — last 24 hours")).toBeDefined();
    tab("PM2.5");
    tab("1h");
    expect(screen.getByText("PM2.5 — last hour")).toBeDefined();
    tab("7d");
    expect(screen.getByText("PM2.5 — last 7 days")).toBeDefined();
    tab("Table");
    expect(screen.getByRole("columnheader", { name: "PM2.5 (µg/m³)" })).toBeDefined();
    expect(screen.getByRole("cell", { name: "9:12" })).toBeDefined();
    expect(screen.getByRole("cell", { name: "1:12" })).toBeDefined();
    tab("Chart");
    expect(screen.queryByRole("columnheader", { name: "PM2.5 (µg/m³)" })).toBeNull();
    expect(screen.getByText("- - 35 µg/m³ guide")).toBeDefined();
  });

  it("the missing-data state shows no CO2 value and no ventilation advice, and turns back", () => {
    render(<AirQualityDemo />);
    expect(screen.getByText("1000")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "show missing-data state" }));
    expect(screen.queryByText("1000")).toBeNull();
    expect(screen.getByText("Not measured")).toBeDefined();
    expect(screen.getByText("No data in last 15 min — never shown as normal")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Log ventilation" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "show normal" }));
    expect(screen.getByText("1000")).toBeDefined();
    expect(screen.getByRole("button", { name: "Log ventilation" })).toBeDefined();
  });

  it("Log ventilation adds the chosen method on top with a toast; Cancel adds nothing", () => {
    render(<AirQualityDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Log ventilation" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toast).not.toHaveBeenCalled();
    expect(screen.queryByText("Today 09:15")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Log ventilation" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toast).not.toHaveBeenCalled();
    expect(historyRows()).toStrictEqual([["Ran ventilation fan · 20 min", "Yesterday 18:10"]]);
    fireEvent.click(screen.getByRole("button", { name: "Log ventilation" }));
    const dialog = within(screen.getByRole("dialog"));
    fireEvent.change(dialog.getByLabelText("What did you do?"), { target: { value: "Manual ventilation" } });
    fireEvent.click(dialog.getByRole("button", { name: "Save record" }));
    expect(toast.mock.calls).toStrictEqual([["Ventilation logged"]]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(historyRows()).toStrictEqual([["Manual ventilation", "Today 09:15"], ["Ran ventilation fan · 20 min", "Yesterday 18:10"]]);
  });
});
