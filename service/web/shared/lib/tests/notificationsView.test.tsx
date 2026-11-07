// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { ToastProvider } from "@ac/web/components/ui";
import type { InboxRow } from "@ac/web/lib/notifications";

const actions = vi.hoisted(() => ({ markAllRead: vi.fn(), markRead: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/navigation", () => ({ usePathname: () => "/customer/alerts", useSearchParams: () => new URLSearchParams() }));
vi.mock("@ac/web/screens/shell/notifications/actions", () => actions);

import { NotificationsView, type ApiInbox } from "@ac/web/screens/shell/notifications/_components/notifications-view";

afterEach(() => { cleanup(); actions.markAllRead.mockReset(); });

const row = (id: string, read: boolean): InboxRow => ({ id, read, version: 1, t: `Notice ${id}`, d: "detail", w: "09:12", href: "" });
const view = (inbox: ApiInbox) => render(<ToastProvider><NotificationsView inbox={inbox} /></ToastProvider>);

// IR637: the page lists the newest 100 of each list, but the tabs count every notification, as the sidebar badge does
describe("the inbox in API mode", () => {
  it("counts the totals, offers Mark all as read, and says the list is cut", () => {
    view({ rows: [row("a", false), row("b", true)], total: 1400, unreadRows: [row("a", false)], unreadTotal: 130 });
    expect(screen.getByRole("tab", { name: /^All\s*1400$/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /^Unread\s*130$/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Mark all as read" })).toBeTruthy();
    expect(screen.getByText("Showing the newest 2 notifications.")).toBeTruthy();
  });
  it("still offers Mark all as read when only older notifications are unread", () => {
    // the newest ones are all read: the unread ones are past the page, so they come from the unread list
    view({ rows: [row("a", true)], total: 150, unreadRows: [row("z", false)], unreadTotal: 1 });
    expect(screen.getByRole("button", { name: "Mark all as read" })).toBeTruthy();
  });
  it("names no IDs when Mark all as read is clicked", () => {
    actions.markAllRead.mockResolvedValue({ marked: 3, skipped: 0, failure: null });
    view({ rows: [row("a", false)], total: 1, unreadRows: [row("a", false)], unreadTotal: 1 });
    act(() => { screen.getByRole("button", { name: "Mark all as read" }).click(); });
    expect(actions.markAllRead).toHaveBeenCalledExactlyOnceWith();
  });
  it("shows no button and no note when nothing is unread and the whole inbox is listed", () => {
    view({ rows: [row("a", true)], total: 1, unreadRows: [], unreadTotal: 0 });
    expect(screen.queryByRole("button", { name: "Mark all as read" })).toBeNull();
    expect(screen.queryByText(/Showing the newest/)).toBeNull();
  });
});
