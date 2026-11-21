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
    fireEvent.click(screen.getByRole("button", { name: "Open Alert a-1" }));
    expect(actions.readAlert).not.toHaveBeenCalled();
    expect(screen.getByText("Reading an alert does not resolve it. It stays under “Needs attention” until the cause is resolved.")).toBeTruthy();
  });

  it("shows the first five reminders, then all of them on request, and sends a filter change to the URL", () => {
    const info = Array.from({ length: 7 }, (_, i) => row(`i-${String(i)}`, { group: "info", severity: "normal" }));
    render(<AlertsLiveView live={live({ rows: info, total: 7 })} />);
    expect(screen.getAllByRole("button", { name: /^Open Alert i-/ })).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: "Show all 7" }));
    expect(screen.getAllByRole("button", { name: /^Open Alert i-/ })).toHaveLength(7);
    fireEvent.change(screen.getByRole("combobox", { name: "Unit" }), { target: { value: "u-1" } });
    expect(patch).toHaveBeenLastCalledWith({ unitId: "u-1" });
  });
});
