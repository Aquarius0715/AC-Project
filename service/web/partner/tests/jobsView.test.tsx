// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { JobRow } from "@ac/web/lib/partnerJobs";

// The contractor job list (FR-P01) from the Core API: the tab, sort and period move through the URL and drop the
// cursor, the two empty states, the restarted-list banner, the row's tones and origin badge, and the paging links
const nav = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav }));

import { JobsView } from "../app/partner/jobs/(list)/_components/jobs-view";
import type { JobsLive } from "../app/partner/jobs/(list)/_lib/load";

beforeEach(() => { nav.mockReset(); });
afterEach(cleanup);

const row = (over: Partial<JobRow>): JobRow => ({
  id: "job-1", title: "job-1 · Lobby AC", origin: null, line: "Assigned", href: "/partner/jobs/job-1",
  tech: { name: "Tech 1", sub: "accepted" }, slot: { main: "21 Sep 10:00", sub: "" }, badge: { text: "Assigned", tone: "primary" }, ...over,
});
const RESET = { cursor: null, page: null };
const live = (over: Partial<JobsLive> = {}): JobsLive => ({
  tab: "all", sort: "status:asc", sortText: "status, then deadline", restarted: false,
  tabs: [{ id: "all", label: "All", count: 3 }, { id: "offered", label: "Offered", count: 1 }, { id: "active", label: "Active", count: 2 }, { id: "review", label: "Review", count: 0 }, { id: "completed", label: "Completed", count: 0 }],
  sorts: [{ id: "status:asc", label: "Sort: status" }, { id: "dueAt:asc", label: "Sort: deadline" }],
  period: { value: "2026-09-14|2026-09-20", custom: false, text: "this week", label: "14 Sep – 20 Sep" },
  periods: [{ value: "2026-09-07|2026-09-13", label: "Last week" }, { value: "2026-09-14|2026-09-20", label: "This week" }],
  rows: [
    row({}),
    row({ id: "job-2", title: "job-2 · Rooftop", origin: "periodic_plan", href: "/partner/schedule?jobId=job-2", tech: { name: "Unassigned", sub: "can’t make it — reassign", tone: "warn", subTone: "warn" }, slot: { main: "Overdue", sub: "ended", tone: "crit" }, badge: { text: "Overdue", tone: "crit" } }),
    row({ id: "job-3", title: "job-3 · Office", origin: "client_request" }),
  ],
  total: 3, nextCursor: null, paging: { page: 1, of: 1 }, ...over,
});
const link = (title: string) => {
  const a = screen.getByText(title).closest("a");
  if (!a) throw new TypeError(`no row ${title}`);
  return a;
};

describe("Jobs view", () => {
  it("moves the tab, sort and period through the URL, dropping the cursor, and keeps the default tab and sort out of it", () => {
    render(<JobsView live={live()} />);
    fireEvent.click(screen.getByRole("tab", { name: /Offered/ }));
    expect(nav).toHaveBeenLastCalledWith({ tab: "offered", ...RESET });
    const sort = screen.getByRole("combobox", { name: "Sort" });
    fireEvent.change(sort, { target: { value: "dueAt:asc" } });
    expect(nav).toHaveBeenLastCalledWith({ sort: "dueAt:asc", ...RESET });
    const period = screen.getByRole("combobox", { name: "Period" });
    expect(within(period).getAllByRole("option").map((o) => o.textContent)).toStrictEqual(["Period: Last week", "Period: This week"]);
    fireEvent.change(period, { target: { value: "2026-09-07|2026-09-13" } });
    expect(nav).toHaveBeenLastCalledWith({ from: "2026-09-07", to: "2026-09-13", ...RESET });
    expect(screen.getByRole("heading", { name: "Jobs — your company only · 3 this week" })).toBeTruthy();
    expect(screen.getByText("Showing 3 of 3 · Sorted by status, then deadline · KPI counts on Overview use the same period (your company only)")).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
    nav.mockReset();
    // back to the defaults: All and status:asc leave the URL
    const view = render(<JobsView live={live({ tab: "offered", sort: "dueAt:asc" })} />);
    fireEvent.click(within(view.container).getByRole("tab", { name: /All/ }));
    expect(nav).toHaveBeenLastCalledWith({ tab: null, ...RESET });
    fireEvent.change(within(view.container).getByRole("combobox", { name: "Sort" }), { target: { value: "status:asc" } });
    expect(nav).toHaveBeenLastCalledWith({ sort: null, ...RESET });
  });

  it("offers the custom period only while it is the current one, and reads a value without an end as an open end", () => {
    render(<JobsView live={live({ period: { value: "2026-09-01", custom: true, text: "1 Sep –", label: "1 Sep –" } })} />);
    const period = screen.getByRole("combobox", { name: "Period" });
    expect(within(period).getAllByRole("option").map((o) => o.textContent)).toStrictEqual(["Period: 1 Sep –", "Period: Last week", "Period: This week"]);
    // away from the custom value first: a change to the value the select already shows fires no event
    fireEvent.change(period, { target: { value: "2026-09-07|2026-09-13" } });
    fireEvent.change(period, { target: { value: "2026-09-01" } });
    expect(nav).toHaveBeenLastCalledWith({ from: "2026-09-01", to: "", ...RESET });
  });

  it("links each row, colours the technician, the slot and the badge by tone, and badges the origin", () => {
    render(<JobsView live={live()} />);
    expect(link("job-1 · Lobby AC").getAttribute("href")).toBe("/partner/jobs/job-1");
    const late = within(link("job-2 · Rooftop"));
    expect(link("job-2 · Rooftop").getAttribute("href")).toBe("/partner/schedule?jobId=job-2");
    expect(late.getByText("Unassigned").className).toBe("text-warn");
    expect(late.getByText("can’t make it — reassign").className).toBe("text-warn"); // the tone replaces the muted colour (cx merges Tailwind classes)
    expect(late.getByText("Overdue", { selector: "b" }).className).toBe("text-crit");
    expect(late.getByText("Overdue", { selector: "[data-slot=badge]" }).className).toMatch(/crit/);
    expect(late.getByText("Periodic plan")).toBeTruthy();
    const plain = within(link("job-1 · Lobby AC"));
    expect(plain.getByText("Tech 1").className).toBe("");
    expect(plain.getByText("accepted").className).toBe("text-muted-foreground");
    expect(plain.queryByText(/Periodic plan|Client request/)).toBeNull();
    expect(within(link("job-3 · Office")).getByText("Client request")).toBeTruthy();
  });

  it("tells a company without jobs from an empty tab", () => {
    const view = render(<JobsView live={live({ rows: [], total: 0, tabs: [{ id: "all", label: "All", count: 0 }] })} />);
    expect(screen.getByText("No jobs yet")).toBeTruthy();
    expect(screen.getByText("Offers from HQ to your company will appear here.")).toBeTruthy();
    view.rerender(<JobsView live={live({ tab: "review", rows: [], total: 0 })} />);
    expect(screen.getByText("No jobs in this tab")).toBeTruthy();
    expect(screen.getByText("Choose another tab or period.")).toBeTruthy();
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    // without any tab the heading counts nothing
    view.rerender(<JobsView live={live({ rows: [], total: 0, tabs: [] })} />);
    expect(screen.getByRole("heading", { name: "Jobs — your company only · 0 this week" })).toBeTruthy();
  });

  it("warns when the list restarted, and pages with the cursor forward and back to the first page", () => {
    const view = render(<JobsView live={live({ restarted: true, nextCursor: "c2", paging: { page: 1, of: 3 } })} />);
    expect(screen.getByText("The list changed since that page was loaded — showing the first page again.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /First page/ })).toBeNull();
    expect(screen.getByText("Page 1 of 3")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Next/ }));
    expect(nav).toHaveBeenLastCalledWith({ cursor: "c2", page: "2" });
    view.rerender(<JobsView live={live({ nextCursor: null, paging: { page: 3, of: 3 } })} />);
    expect(screen.queryByRole("button", { name: /Next/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /First page/ }));
    expect(nav).toHaveBeenLastCalledWith(RESET);
  });
});
