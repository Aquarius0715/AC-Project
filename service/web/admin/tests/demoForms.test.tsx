// @vitest-environment jsdom
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demos of the Admin app (R843): a check in the click handler reads the rule, not the message of the last
// render; uncontrolled fields start again from each selection; scheduled rows have their own ids; a dialog opens
// without the errors of an earlier try.
vi.mock("next/link", () => ({ default: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a> }));
vi.mock("@ac/web/lib/useUrlTab", () => ({
  useUrlTab: <T extends string>(_ids: Record<string, T>, first: T) => {
    const [tab, setTab] = useState<T>(first);
    return [tab, setTab] as const;
  },
}));

import { ExceptionDemo } from "../app/admin/restrictions/[id]/_components/exception-demo";
import { RestrictionsDemo } from "../app/admin/restrictions/_components/restrictions-demo";
import { AutomationDemo } from "../app/admin/settings/automation/_components/automation-demo";
import { AccessDemo } from "../app/admin/settings/access/_components/access-demo";
import { DevicesDemo } from "../app/admin/devices/_components/devices-demo";

afterEach(cleanup);

const fieldValue = (el: HTMLElement) => (el instanceof HTMLInputElement || el instanceof HTMLSelectElement ? el.value : null);

describe("Admin demo forms", () => {
  it("refuses a past exception date on the first Apply", () => {
    render(<ExceptionDemo id="restriction-limited-a" />);
    fireEvent.change(screen.getByLabelText(/Exception until/), { target: { value: "2020-01-01" } });
    fireEvent.click(screen.getByRole("button", { name: /^Apply / }));
    expect(screen.getByText("Date must be in the future")).toBeTruthy();
    expect(screen.queryByText(/a remove command was sent/)).toBeNull();
  });

  it("refuses an Execute-after under 24 h on the first Schedule, and numbers each scheduled row", () => {
    render(<RestrictionsDemo />);
    const schedule = (date: string) => {
      fireEvent.click(screen.getByRole("button", { name: /Schedule restriction/ }));
      const dialog = screen.getByRole("dialog");
      const [contract] = within(dialog).getAllByRole("combobox");
      if (!contract) throw new Error("no contract select");
      fireEvent.change(contract, { target: { value: "contract-rto-a" } });
      fireEvent.change(within(dialog).getByLabelText("Date"), { target: { value: date } });
      fireEvent.click(within(dialog).getByRole("button", { name: "Schedule" }));
    };
    schedule("2020-01-01");
    expect(screen.getByText("Execute-after must be at least 24 h after the notice")).toBeTruthy();
    expect(screen.queryByText("restriction-new-1")).toBeNull();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: /Schedule restriction/ }));
    expect(screen.queryByText("Execute-after must be at least 24 h after the notice")).toBeNull(); // Cancel cleared the try
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    schedule("2099-01-01");
    schedule("2099-01-02");
    expect(screen.getByText("restriction-new-1")).toBeTruthy();
    expect(screen.getByText("restriction-new-2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Schedule restriction/ }));
    const [contract] = within(screen.getByRole("dialog")).getAllByRole("combobox");
    if (!contract) throw new Error("no contract select");
    expect(fieldValue(contract)).toBe(""); // the form starts empty again
  });

  it("shows the picked policy's name and priority", () => {
    render(<AutomationDemo />);
    fireEvent.click(screen.getByText("Occupancy stop"));
    expect(fieldValue(screen.getByDisplayValue("Occupancy stop"))).toBe("Occupancy stop");
    expect(screen.getByDisplayValue("50")).toBeTruthy();
  });

  it("shows the picked person's email", () => {
    render(<AccessDemo />);
    fireEvent.click(screen.getByText("aziz.contractor-a"));
    expect(screen.getByDisplayValue("aziz@contractor-a.example")).toBeTruthy();
    expect(screen.queryByDisplayValue("r.lim@hq.example")).toBeNull();
  });

  it("shows the picked model, counts every device, and opens Register without the errors of New model", () => {
    render(<DevicesDemo />);
    fireEvent.click(screen.getByText("DemoAir SPL-100"));
    expect(screen.getByDisplayValue("SPL-100")).toBeTruthy();
    expect(screen.getByRole("tab", { name: /IoT devices/ }).textContent).toContain("6");
    fireEvent.click(screen.getByRole("button", { name: /New$/ }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Create" }));
    expect(within(screen.getByRole("dialog")).getAllByText("Required")).toHaveLength(2);
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("tab", { name: /IoT devices/ }));
    fireEvent.click(screen.getByRole("button", { name: /Register device/ }));
    expect(within(screen.getByRole("dialog")).queryByText("Serial is required")).toBeNull();
  });
});
