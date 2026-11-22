// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { Alert } from "@ac/web/lib/alerts";
import { AlertsView } from "../app/customer/alerts/_components/alerts-view";

// The Phase 1A demo of /customer/alerts (FR-C08, FR-C15): opening an alert marks it read but leaves it unresolved; the
// unread-only filter hides read rows; the load-error state is not "0 alerts"; the policies tab switches default rules
// (the "AC offline" warning), creates a policy only with a name and a recovery below the limit, and deletes one.
beforeEach(() => { history.replaceState(null, "", "/customer/alerts"); });
afterEach(() => { cleanup(); });

const unreadCount = () => screen.getByText(/^Updated 09:13 · \d+ unread$/).textContent;
const card = (title: string) => {
  const heading = screen.getByText(title);
  const section = heading.closest("[data-slot=card]") ?? heading.parentElement?.parentElement;
  if (!(section instanceof HTMLElement)) throw new TypeError(`no card ${title}`);
  return section;
};

describe("AlertsView (demo)", () => {
  it("opening an unread alert marks it read, and the unread-only filter then hides it", () => {
    render(<AlertsView />);
    expect(unreadCount()).toBe("Updated 09:13 · 1 unread");
    fireEvent.click(screen.getByText("Unread only"));
    expect(screen.queryByText("Compressor short-cycling")).toBeNull();
    expect(screen.getByText("Possible open window")).toBeTruthy();
    expect(within(card("Reminders & information")).getByText("Nothing here.")).toBeTruthy();
    fireEvent.click(screen.getByText("Possible open window"));
    const dialog = screen.getByRole("dialog", { name: "Possible open window" });
    expect(dialog.textContent).toContain("Opened → auto Read, still Unresolved");
    fireEvent.click(within(dialog).getByRole("button", { name: "Back to list" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(unreadCount()).toBe("Updated 09:13 · 0 unread");
    expect(within(card("Needs attention")).getByText("Nothing here.")).toBeTruthy();
  });

  it("the load error is not an empty list, and Retry brings the rows back; new rows replace the local read flags", () => {
    const rows: Alert[] = [{ id: "x-1", title: "Fresh alert", sev: "warning", kind: "Fault", kindIcon: "error", icon: "warning", where: "Hall AC", ev: "Evidence", group: "attn", read: false }];
    const view = render(<AlertsView rows={rows} />);
    fireEvent.click(screen.getByText("show load error"));
    expect(screen.getByText("Couldn’t load alerts")).toBeTruthy();
    expect(screen.queryByText("Fresh alert")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(screen.getByText("Fresh alert")).toBeTruthy();
    fireEvent.click(screen.getByText("Fresh alert"));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(unreadCount()).toBe("Updated 09:13 · 0 unread");
    view.rerender(<AlertsView rows={[...rows]} />);
    expect(unreadCount()).toBe("Updated 09:13 · 1 unread");
  });

  it("the policies tab switches a default rule, deletes a policy, and saves a new one only with a name and a recovery below the limit", () => {
    render(<AlertsView />);
    fireEvent.click(screen.getByRole("tab", { name: /Alert policies/ }));
    expect(location.search).toBe("?tab=policies");
    expect(screen.getByText(/Turned off: “AC offline”/)).toBeTruthy();
    fireEvent.click(screen.getByRole("switch", { name: "AC offline" }));
    expect(screen.queryByText(/Turned off: “AC offline”/)).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "Night humidity" }));
    expect(screen.getByRole("switch", { name: "Night humidity" }).getAttribute("aria-checked")).toBe("true");

    fireEvent.click(within(card("Stuffy office")).getByRole("button", { name: "Delete" }));
    expect(screen.queryByText("Stuffy office")).toBeNull();

    fireEvent.click(within(card("Bedroom too hot")).getByRole("button", { name: "Edit" }));
    let dialog = screen.getByRole("dialog", { name: "Edit alert policy" });
    expect(within(dialog).getByRole("textbox")).toHaveProperty("value", "Bedroom too hot");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    // saving an edit keeps the list as it is (the demo does not rewrite the card)
    fireEvent.click(within(card("Bedroom too hot")).getByRole("button", { name: "Edit" }));
    fireEvent.click(screen.getByRole("button", { name: "Save policy" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getAllByRole("button", { name: "Edit" })).toHaveLength(2);

    fireEvent.click(screen.getByRole("button", { name: /Create policy/ }));
    dialog = screen.getByRole("dialog", { name: "Create alert policy" });
    const save = within(dialog).getByRole("button", { name: "Save policy" });
    expect(within(dialog).getByText("Name is required")).toBeTruthy();
    expect(save).toHaveProperty("disabled", true);
    fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: "Hot lab" } });
    const [limit, , recover] = within(dialog).getAllByRole("spinbutton");
    if (!limit || !recover) throw new TypeError("no number fields");
    fireEvent.change(recover, { target: { value: "1300" } });
    expect(within(dialog).getByText("Recovery must be below the 1200 limit")).toBeTruthy();
    expect(save).toHaveProperty("disabled", true);
    fireEvent.change(limit, { target: { value: "1400" } });
    expect(within(dialog).getByText("Recovery 1300 is below the 1400 limit")).toBeTruthy();
    const [metric, op] = within(dialog).getAllByRole("combobox");
    if (!metric || !op) throw new TypeError("no selects");
    fireEvent.change(metric, { target: { value: "temp" } });
    fireEvent.change(op, { target: { value: "≤" } });
    fireEvent.change(within(dialog).getAllByRole("spinbutton")[1] ?? limit, { target: { value: "20" } });
    fireEvent.click(within(dialog).getByText("Critical"));
    fireEvent.click(save);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(card("Hot lab")).getByText("TEMP ≤ 1400 for 20 min · recover below 1300")).toBeTruthy();
    expect(within(card("Hot lab")).getByText("critical · notify in-app")).toBeTruthy();
  });
});
