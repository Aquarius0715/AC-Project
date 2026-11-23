// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo of automations (FR-C04, FR-C05, fixtures only): the editor reviews before it saves and refuses a
// missing name, a temperature outside 16–30 °C and a location rule without consent; withdrawing consent pauses the
// location rules; switching, editing and deleting change the list, down to its empty state.
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { AutomationsDemo } from "../app/customer/automations/_components/automations-demo";

const dialog = () => screen.getByRole("dialog");
const press = (name: string) => { fireEvent.click(within(dialog()).getByRole("button", { name })); };
const typeIn = (name: RegExp, value: string) => { fireEvent.change(within(dialog()).getByRole("textbox", { name }), { target: { value } }); };

beforeEach(() => { toast.mockReset(); });
afterEach(() => { cleanup(); });

describe("AutomationsDemo", () => {
  it("refuses a rule without a name or with a temperature out of range, then reviews and saves it", () => {
    render(<AutomationsDemo />);
    expect(screen.getByText("6 automations · 4 on · checked again when it runs")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create automation" }));
    fireEvent.change(within(dialog()).getByRole("spinbutton", { name: /^Set temperature/ }), { target: { value: "40" } });
    press("Review");
    expect(within(dialog()).getByText("Name is required")).toBeTruthy();
    expect(within(dialog()).getByText("16–30°C only (device capability)")).toBeTruthy();
    typeIn(/^Name/, "Evening cool");
    fireEvent.change(within(dialog()).getByRole("spinbutton", { name: /^Set temperature/ }), { target: { value: "26" } });
    press("Review");
    expect(within(dialog()).getByText("Simulation passed — no commands were sent.")).toBeTruthy();
    press("Back");
    expect(within(dialog()).getByRole("textbox", { name: "Name" })).toHaveProperty("value", "Evening cool");
    press("Review");
    press("Save automation");
    expect(toast).toHaveBeenCalledWith("Automation saved");
    expect(screen.getByText("7 automations · 5 on · checked again when it runs")).toBeTruthy();
    expect(screen.getByText("Evening cool")).toBeTruthy();
    expect(screen.getAllByText("Every day at 18:00")).toHaveLength(1);
    expect(screen.getByText("Set temperature 26°C")).toBeTruthy();
  });

  it("adds a presence rule with the trigger's own when-line and one Then row", () => {
    render(<AutomationsDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Create automation" }));
    typeIn(/^Name/, "Empty office");
    press("Presence");
    expect(within(dialog()).queryByRole("group", { name: "Time" })).toBeNull();
    press("Review");
    press("Save automation");
    expect(screen.getAllByText("No one detected for 30 min (demo occupancy)")).toHaveLength(2);
    expect(screen.getAllByText("Then")).toHaveLength(5);
  });

  it("withdrawing consent pauses the location rules and refuses a new location rule", () => {
    render(<AutomationsDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Withdraw consent" }));
    expect(toast).toHaveBeenLastCalledWith("Location consent withdrawn", "warn");
    expect(screen.getAllByText("Paused — consent withdrawn")).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Create automation" }));
    typeIn(/^Name/, "Arrive");
    press("Location");
    expect(within(dialog()).getByText("Location consent withdrawn.")).toBeTruthy();
    press("Review");
    expect(within(dialog()).getByText("Location consent is withdrawn — grant it to use location triggers")).toBeTruthy();
    press("Cancel");
    expect(screen.getByText("6 automations · 4 on · checked again when it runs")).toBeTruthy(); // nothing added
    expect(screen.queryByText("Arrive")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Grant consent" }));
    expect(toast).toHaveBeenLastCalledWith("Location consent granted", "ok");
    expect(screen.queryByText("Paused — consent withdrawn")).toBeNull();
  });

  it("edits a rule's temperature and AC, and switches a rule on", () => {
    render(<AutomationsDemo />);
    fireEvent.click(screen.getAllByRole("button", { name: "Edit" })[0] ?? document.body);
    expect(within(dialog()).getByText("Edit automation")).toBeTruthy();
    expect(within(dialog()).getByRole("textbox", { name: "Name" })).toHaveProperty("value", "Weekday pre-cool");
    expect(within(dialog()).getByRole("combobox", { name: "Air conditioner" })).toHaveProperty("value", "Bedroom AC");
    fireEvent.change(within(dialog()).getByRole("combobox", { name: "Air conditioner" }), { target: { value: "Kitchen AC" } });
    fireEvent.change(within(dialog()).getByRole("spinbutton", { name: /^Set temperature/ }), { target: { value: "27" } });
    press("Review");
    press("Save automation");
    expect(screen.getByText("Set temperature 27°C")).toBeTruthy();
    expect(screen.getByText("Kitchen AC")).toBeTruthy();
    expect(screen.getByText("Weekday pre-cool")).toBeTruthy(); // the name kept
    expect(screen.getByText("Set temperature 25°C")).toBeTruthy(); // Vacation mode unchanged
    fireEvent.click(screen.getByRole("switch", { name: "Toggle Hot afternoon pre-cool" }));
    expect(screen.getByText("6 automations · 5 on · checked again when it runs")).toBeTruthy();
  });

  it("deletes every rule from its menu and then shows the empty state", async () => {
    render(<AutomationsDemo />);
    for (let n = 6; n > 0; n--) {
      fireEvent.click(screen.getAllByRole("button", { name: "More" })[0] ?? document.body);
      fireEvent.click(await screen.findByRole("menuitem", { name: "Delete…" }));
      fireEvent.click(within(dialog()).getByRole("button", { name: "Delete" }));
    }
    expect(toast).toHaveBeenCalledWith("“Weekday pre-cool” removed");
    expect(screen.getByText("No automations yet")).toBeTruthy();
    // the empty state's own Create opens a new rule, and saving it leaves the empty state
    const creates = screen.getAllByRole("button", { name: "Create automation" });
    expect(creates).toHaveLength(2); // the page head's and the empty state's
    fireEvent.click(creates[1] ?? document.body);
    expect(within(dialog()).getByText("Create automation")).toBeTruthy();
    typeIn(/^Name/, "First rule");
    press("Review");
    press("Save automation");
    expect(screen.queryByText("No automations yet")).toBeNull();
    expect(screen.getByText("1 automation · 1 on · checked again when it runs")).toBeTruthy();
  });

  it("a schedule rule runs at the hour picked", () => {
    render(<AutomationsDemo />);
    fireEvent.click(screen.getByRole("button", { name: "Create automation" }));
    typeIn(/^Name/, "Morning cool");
    fireEvent.change(within(dialog()).getByRole("combobox", { name: "Hour" }), { target: { value: "07" } });
    press("Review");
    press("Save automation");
    expect(screen.getByText("Every day at 07:00")).toBeTruthy();
    expect(screen.getByText("07:00")).toBeTruthy();
  });
});
