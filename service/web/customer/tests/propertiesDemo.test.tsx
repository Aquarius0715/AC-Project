// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo of Units & locations (fixtures, DATA_SOURCE≠API): Home A's summary with its unassigned Guest AC; a
// room lists its ACs; a space with a group switches to Group control; the unassigned row has no Rename; Rename asks
// for a name and refuses a second “Office A”.
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { PropertiesDemo } from "../app/customer/properties/_components/properties-demo";

const row = (name: string) => screen.getByText(name, { selector: "span.truncate" }).closest("button") ?? document.body;
const panel = () => within(document.querySelectorAll<HTMLElement>("[data-slot=card]")[1] ?? document.body);

beforeEach(() => { toast.mockReset(); });
afterEach(() => { cleanup(); });

describe("PropertiesDemo", () => {
  it("starts on Home A: its summary and the unassigned Guest AC", () => {
    render(<PropertiesDemo />);
    expect(panel().getByText("Home A · Home")).toBeDefined();
    expect(panel().getByText("5 in rooms · 1 unassigned")).toBeDefined();
    expect(panel().getByText("Select a room to see its air conditioners")).toBeDefined();
    expect(panel().getByRole("link", { name: /Guest AC/ }).getAttribute("href")).toBe("/customer/units/unit-guest");
    // the tree: counts in the singular and plural, and no Rename on the unassigned row
    expect(screen.getAllByText("· 2 units")).toHaveLength(2);
    expect(screen.getAllByText("· 1 unit")).toHaveLength(4);
    expect(screen.queryByRole("button", { name: "Rename Unassigned units" })).toBeNull();
  });

  it("a room lists its ACs; a room with a group switches to Group control and back", () => {
    render(<PropertiesDemo />);
    fireEvent.click(row("Kitchen"));
    expect(panel().getByText("Home A › 1F › Kitchen")).toBeDefined();
    expect(panel().getByRole("link", { name: /Kitchen AC/ }).getAttribute("href")).toBe("/customer/units/unit-kitchen-a");
    expect(panel().queryByRole("button", { name: "Group control" })).toBeNull();

    fireEvent.click(row("Bedroom"));
    expect(panel().getByText("2 ACs ·")).toBeDefined();
    expect(panel().getAllByRole("link").map((a) => a.getAttribute("href"))).toStrictEqual(["/customer/units/unit-online-rto", "/customer/units/unit-bedroom-2"]);
    fireEvent.click(panel().getByRole("button", { name: "Group control" }));
    expect(panel().getByRole("button", { name: "Group control" }).getAttribute("aria-pressed")).toBe("true");
    expect(panel().getByText("Apply to 2 selected ACs")).toBeDefined();
    expect(panel().queryAllByRole("link")).toHaveLength(0);
    // another location starts in single mode again
    fireEvent.click(row("Meeting room"));
    expect(panel().getByRole("button", { name: "Single AC" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(panel().getByRole("button", { name: "Group control" }));
    expect(panel().getByText("min 24 °C")).toBeDefined();
    fireEvent.click(panel().getByRole("button", { name: "Single AC" }));
    expect(panel().queryByText("Apply to 2 selected ACs")).toBeNull();
  });

  it("the workstations list their eight ACs, the study is offline, and a floor asks for a room", () => {
    render(<PropertiesDemo />);
    fireEvent.click(row("Workstations"));
    expect(panel().getByText("8 ACs ·")).toBeDefined();
    expect(panel().getAllByRole("link")).toHaveLength(8);
    expect(panel().getAllByText(/— °C/)).toHaveLength(2);
    fireEvent.click(panel().getByRole("button", { name: "Group control" }));
    expect(panel().getByText("4 selected")).toBeDefined();
    fireEvent.click(row("Study"));
    expect(panel().getByText("Study AC is offline (unknown).")).toBeDefined();
    fireEvent.click(row("1F"));
    expect(panel().getByText("Select a room with air conditioners to see them here.")).toBeDefined();
    fireEvent.click(row("Unassigned units"));
    expect(panel().getByText("Unassigned units (1)")).toBeDefined();
    expect(panel().queryByRole("button", { name: "Rename" })).toBeNull();
    expect(panel().getByRole("link", { name: /Guest AC/ })).toBeDefined();
  });

  it("renames a location: a name is required and a second “Office A” is refused", () => {
    render(<PropertiesDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Rename Kitchen" }));
    const box = within(screen.getByRole("dialog", { name: "Rename Kitchen" })).getByRole<HTMLInputElement>("textbox");
    expect(box.value).toBe("Kitchen");
    fireEvent.change(box, { target: { value: "  " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByText("Name is required (1–120 characters)")).toBeDefined();
    fireEvent.change(box, { target: { value: " office A " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByText("A location with this name already exists")).toBeDefined();
    fireEvent.change(box, { target: { value: "Pantry" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(toast).toHaveBeenCalledWith("Renamed to “Pantry”");
    expect(screen.queryByRole("dialog")).toBeNull();

    // Office A keeps its own name; the panel's Rename renames the shown location; Cancel closes
    fireEvent.click(screen.getByRole("button", { name: "Rename Office A" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(toast).toHaveBeenLastCalledWith("Renamed to “Office A”");
    fireEvent.click(panel().getByRole("button", { name: "Rename" }));
    expect(screen.getByRole("dialog", { name: "Rename Home A" })).toBeDefined();
    expect(screen.queryByText("Name is required (1–120 characters)")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Rename 2F" }));
    fireEvent.keyDown(screen.getByRole("dialog", { name: "Rename 2F" }), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
