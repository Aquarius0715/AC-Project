// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { InboxAlert } from "@ac/web/lib/alerts";
import type { AlertsLive } from "../app/customer/alerts/_lib/load";

// The customer alert inbox in API mode (FR-C08, DD-C08): the empty text says when the list is filtered, by severity,
// unread only or one unit; opening an unread alert marks it read and a refusal shows why; the reminders show the first
// five, the rest on request; the filters go to the URL.
const actions = vi.hoisted(() => ({
  readAlert: vi.fn<(ids: string[]) => Promise<ActionFailure | null>>(), savePolicy: vi.fn(), deletePolicy: vi.fn(), setDefaultRule: vi.fn(),
}));
vi.mock("../app/customer/alerts/actions", () => actions);
const patch = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => patch }));
const nav = vi.hoisted(() => ({ refresh: vi.fn(), push: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => nav, useSearchParams: () => new URLSearchParams() }));
// the policies tab has its own tests (policiesLive.test.tsx); here it only reports a save or a refusal to the page
vi.mock("../app/customer/alerts/_components/policies-live", () => ({
  PoliciesLive: ({ onDone, onFail }: Readonly<ComponentProps<typeof PoliciesLive>>) => (
    <div>
      <button type="button" onClick={() => { onDone("Policy saved"); }}>done</button>
      <button type="button" onClick={() => { onFail({ code: "CONFLICT", messageKey: "error.CONFLICT", fieldErrors: {}, retryAfterSeconds: null }); }}>conflict</button>
      <button type="button" onClick={() => { onFail({ code: "VALIDATION", messageKey: "error.VALIDATION", fieldErrors: {}, retryAfterSeconds: null }); }}>invalid</button>
    </div>
  ),
}));

import type { ComponentProps } from "react";
import { policyForm } from "@ac/web/lib/customerPolicies";
import type { PoliciesLive } from "../app/customer/alerts/_components/policies-live";
import { AlertsLiveView } from "../app/customer/alerts/_components/alerts-live";

const row = (id: string, over: Partial<InboxAlert> = {}): InboxAlert => ({
  id, unitId: "u-1", type: "sensor", title: `Alert ${id}`, severity: "warning", cause: "fault", kind: "Fault", kindIcon: "error", where: "Bedroom AC · detected now",
  evidence: "Evidence: hot", group: "attn", status: { state: "open", text: "Unresolved", tone: "warn", detail: "Open — not resolved yet" }, unread: [], notes: 0, next: null, ...over,
});
const live = (over: Partial<AlertsLive> = {}): AlertsLive => ({
  now: "2026-09-15T01:12:30.000Z", unreadOnly: false, unitId: null, units: [{ id: "u-1", name: "Bedroom AC", place: "Home" }], alertCount: 0, total: 0, unread: 0, rows: [],
  policies: { count: 0, owner: true, customerId: "c-1", membershipId: "m-1", timezone: "Asia/Kuala_Lumpur", defaults: null, cards: [], list: [], editor: null },
  display: { locale: "en", timeZone: "Asia/Kuala_Lumpur" }, tab: "alerts", severity: null, ...over,
});

beforeEach(() => {
  patch.mockReset();
  nav.refresh.mockReset();
  actions.readAlert.mockReset().mockResolvedValue(null);
});
afterEach(() => { cleanup(); });

describe("AlertsLiveView", () => {
  it("says the list is filtered when a unit, severity or unread-only filter hides everything", () => {
    render(<AlertsLiveView live={live()} />);
    expect(screen.getByText("Nothing needs your attention.")).toBeTruthy();
    cleanup();
    render(<AlertsLiveView live={live({ unitId: "u-1" })} />);
    expect(screen.getByText("Nothing in this filter.")).toBeTruthy();
    cleanup();
    render(<AlertsLiveView live={live({ severity: "critical" })} />);
    expect(screen.getByText("Nothing in this filter.")).toBeTruthy();
    cleanup();
    render(<AlertsLiveView live={live({ unreadOnly: true })} />);
    expect(screen.getByText("Nothing in this filter.")).toBeTruthy();
  });

  it("opening an unread alert marks its notifications read; a refusal says why it was not marked", async () => {
    actions.readAlert.mockResolvedValueOnce({ code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {}, retryAfterSeconds: null });
    render(<AlertsLiveView live={live({ rows: [row("a-1", { unread: [{ id: "n-1", version: 1 }, { id: "n-2", version: 2 }] })], total: 1, unread: 1 })} />);
    fireEvent.click(screen.getByRole("button", { name: "Open Alert a-1" }));
    await waitFor(() => { expect(screen.getByText(/^Not marked as read:/)).toBeTruthy(); });
    expect(actions.readAlert).toHaveBeenCalledWith(["n-1", "n-2"]);
    expect(screen.getByRole("dialog", { name: "Alert a-1" })).toBeTruthy();
  });

  it("opening an alert that is already read sends no write", () => {
    render(<AlertsLiveView live={live({ rows: [row("a-1")], total: 1 })} />);
    fireEvent.click(screen.getByText("Alert a-1"));
    expect(actions.readAlert).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: "Alert a-1" })).toBeTruthy();
    expect(screen.getByText("Reading an alert does not resolve it. It stays under “Needs attention” until the cause is resolved.")).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("shows the first five reminders, then all of them on request, and sends a filter change to the URL", () => {
    const info = Array.from({ length: 7 }, (_, i) => row(`i-${String(i)}`, { group: "info", severity: "normal" }));
    render(<AlertsLiveView live={live({ rows: info, total: 7 })} />);
    expect(screen.getAllByRole("button", { name: /^Open Alert i-/ })).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: "Show all 7" }));
    expect(screen.getAllByRole("button", { name: /^Open Alert i-/ })).toHaveLength(7);
    fireEvent.change(screen.getByRole("combobox", { name: "Unit" }), { target: { value: "u-1" } });
    expect(patch).toHaveBeenLastCalledWith({ unitId: "u-1" });
    fireEvent.click(screen.getByRole("button", { name: "Show the first 5" }));
    expect(screen.getAllByRole("button", { name: /^Open Alert i-/ })).toHaveLength(5);
  });

  it("sends the severity, unit, unread-only and tab changes to the URL, clearing a filter with null", () => {
    render(<AlertsLiveView live={live({ severity: "critical", unitId: "u-1", unreadOnly: true })} />);
    expect(screen.getByRole("option", { name: "Unit: Bedroom AC · Home" })).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Severity" }), { target: { value: "warning" } });
    expect(patch).toHaveBeenLastCalledWith({ severity: "warning" });
    fireEvent.change(screen.getByRole("combobox", { name: "Severity" }), { target: { value: "" } });
    expect(patch).toHaveBeenLastCalledWith({ severity: null });
    fireEvent.change(screen.getByRole("combobox", { name: "Unit" }), { target: { value: "" } });
    expect(patch).toHaveBeenLastCalledWith({ unitId: null });
    fireEvent.click(screen.getByRole("checkbox", { name: "Unread only" }));
    expect(patch).toHaveBeenLastCalledWith({ unreadOnly: null });
    fireEvent.click(screen.getByRole("tab", { name: /Alert policies/ }));
    expect(patch).toHaveBeenLastCalledWith({ tab: "policies", policyId: null });
    cleanup();
    render(<AlertsLiveView live={live({ units: [{ id: "u-9", name: "Loose AC", place: "" }] })} />);
    expect(screen.getByRole("option", { name: "Unit: Loose AC" })).toBeTruthy();
    fireEvent.click(screen.getByRole("checkbox", { name: "Unread only" }));
    expect(patch).toHaveBeenLastCalledWith({ unreadOnly: "true" });
    cleanup();
    render(<AlertsLiveView live={live({ tab: "policies" })} />);
    fireEvent.click(screen.getByRole("tab", { name: /^Alerts/ }));
    expect(patch).toHaveBeenLastCalledWith({ tab: null, policyId: null });
  });

  it("words the unresolved count in the singular for one warning and the plural otherwise", () => {
    render(<AlertsLiveView live={live({ alertCount: 1, unread: 0 })} />);
    expect(screen.getByText(/^1 unresolved warning · Updated .* · 0 unread$/)).toBeTruthy();
    cleanup();
    render(<AlertsLiveView live={live({ alertCount: 3, unread: 2 })} />);
    expect(screen.getByText(/^3 unresolved warnings · Updated .* · 2 unread$/)).toBeTruthy();
  });

  it("a read that goes through re-reads the page and clears an earlier refusal", async () => {
    actions.readAlert.mockResolvedValueOnce({ code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {}, retryAfterSeconds: null }).mockResolvedValueOnce(null);
    render(<AlertsLiveView live={live({ rows: [row("a-1", { unread: [{ id: "n-1", version: 1 }] })], total: 1, unread: 1 })} />);
    fireEvent.click(screen.getByRole("button", { name: "Open Alert a-1" }));
    await waitFor(() => { expect(screen.getByText(/^Not marked as read:/)).toBeTruthy(); });
    fireEvent.click(screen.getByRole("button", { name: "Back to list" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Open Alert a-1" }));
    await waitFor(() => { expect(screen.queryByText(/^Not marked as read:/)).toBeNull(); });
    expect(nav.refresh).toHaveBeenCalledOnce();
    expect(actions.readAlert).toHaveBeenCalledTimes(2);
  });

  it("offers the next step of each alert: cleaning, air quality or repair while it is open, none once resolved", () => {
    const resolved = { state: "resolved", text: "Resolved", tone: "ok", detail: "Resolved" } as const;
    render(<AlertsLiveView live={live({ rows: [
      row("m-1", { type: "maintenance", cause: "maintenance", group: "info", unitId: "u-7" }),
      row("q-1", { cause: "quality", next: "Open a window" }),
      row("f-1", { unitId: "u-8" }),
      row("m-2", { type: "maintenance", cause: "maintenance", group: "info", status: resolved }),
      row("q-2", { cause: "quality", group: "info", status: resolved }),
      row("f-2", { group: "info", status: resolved }),
    ], total: 6 })} />);
    expect(screen.getAllByRole("link", { name: "Book cleaning" }).map((a) => a.getAttribute("href"))).toStrictEqual(["/customer/maintenance?tab=filter-care&unitId=u-7"]);
    expect(screen.getAllByRole("link", { name: "Air quality" }).map((a) => a.getAttribute("href"))).toStrictEqual(["/customer/air-quality"]);
    expect(screen.getAllByRole("link", { name: "Request repair" }).map((a) => a.getAttribute("href"))).toStrictEqual(["/customer/maintenance?new=u-8"]);
    expect(screen.getAllByRole("link", { name: "View unit" })).toHaveLength(6);
    expect(screen.getByText("What to do: Open a window")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Open Alert q-1" }));
    const dialog = screen.getByRole("dialog", { name: "Alert q-1" });
    expect(dialog.textContent).toContain("Open a window");
    expect(dialog.querySelector('a[href="/customer/air-quality"]')).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Back to list" }));
    fireEvent.click(screen.getByRole("button", { name: "Open Alert f-2" }));
    expect(screen.getByText("This alert is resolved. It stays in the list as a record.")).toBeTruthy();
  });

  it("shows the policies tab's save and refusal on the page, as a warning for a conflict, and drops it when the editor opens", () => {
    const view = render(<AlertsLiveView live={live({ tab: "policies" })} />);
    fireEvent.click(screen.getByRole("button", { name: "done" }));
    expect(screen.getByRole("status").textContent).toContain("Policy saved");
    fireEvent.click(screen.getByRole("button", { name: "conflict" }));
    expect(screen.getByRole("status").textContent).toContain("this policy changed meanwhile");
    fireEvent.click(screen.getByRole("button", { name: "invalid" }));
    expect(screen.getByRole("alert").textContent).toContain("Not saved (VALIDATION).");
    view.rerender(<AlertsLiveView live={live({ tab: "policies", policies: { ...live().policies, editor: { policy: null, form: policyForm(null), attached: [] } } })} />);
    expect(screen.queryByRole("alert")).toBeNull();
    // back from the editor to the list: the editor's own message (here a save) stays
    fireEvent.click(screen.getByRole("button", { name: "done" }));
    view.rerender(<AlertsLiveView live={live({ tab: "policies" })} />);
    expect(screen.getByRole("status").textContent).toContain("Policy saved");
  });
});
