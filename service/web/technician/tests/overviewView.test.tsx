// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { Job } from "@ac/web/lib/jobs";

// the technician overview (FR-T01, review round 1081): the live view from the loader's rows (banners, tabs and sort in the
// URL, empty states, timeline, alerts and reports) and the Phase 1A demo from the fixtures and the shared job store
const patch = vi.hoisted(() => vi.fn<(p: Record<string, string | null>) => void>());
const store = vi.hoisted((): { jobs: Job[] } => ({ jobs: [] }));
// the English text with its {placeholders} filled, as the real translator does for the English display language
function fill(s: string, values: Record<string, string | number> = {}) {
  let text = s;
  for (const [k, x] of Object.entries(values)) text = text.split(`{${k}}`).join(String(x));
  return text;
}
vi.mock("@ac/web/components/I18n", () => ({ useT: () => fill }));
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => patch }));
vi.mock(import("@ac/web/lib/jobs"), async (importOriginal) => ({ ...(await importOriginal()), useJobs: () => store.jobs }));

import { getJobState } from "@ac/web/lib/jobs";
import { OverviewView } from "../app/technician/(overview)/_components/overview-view";
import { OverviewDemo } from "../app/technician/(overview)/_components/overview-demo";
import type { OverviewLive } from "../app/technician/(overview)/_lib/load";

type Row = OverviewLive["rows"][number];
const SLOT = { startAt: "2026-09-21T02:00:00Z", endAt: "2026-09-21T04:00:00Z" };
const row = (id: string, over: Partial<Row> = {}): Row => ({
  id, short: id.slice(0, 8), title: "Bedroom AC", origin: null, line: `${id} line`, badge: { text: "Work not started", tone: "primary" }, today: true,
  slot: SLOT, slotText: "10:00–12:00", ackPending: false, opensAt: null, opens: null, status: "assigned", severity: "normal", ...over,
});
const alert = (n: number): OverviewLive["alerts"][number] => ({ id: `al-${String(n)}`, title: `Alert ${String(n)}`, sub: `unit · open · ${String(n)}`, severity: "warning", href: `/technician/units/u${String(n)}/alerts?alertId=al-${String(n)}` });
function live(over: Partial<OverviewLive> = {}): OverviewLive {
  const rows = [row("job-aaaa-1111", { origin: "periodic_plan" }), row("job-bbbb-2222", { origin: "hq_request" }), row("job-cccc-3333", { origin: "client_request" }), row("job-dddd-4444")];
  return {
    now: "2026-09-21T03:45:00Z",
    sorts: [{ id: "severity", text: "Sort: severity" }, { id: "dueAt", text: "Sort: deadline" }],
    title: "Today — sorted by severity, deadline, progress",
    tabs: [{ id: "today", label: "Today", count: 4 }, { id: "all", label: "All assigned", count: 6 }],
    kpis: [{ label: "Assigned units", value: 3, sub: "in your assignments" }, { label: "Overdue", value: 1, sub: "past the due date", tone: "crit" }],
    pending: [], rows, readOnly: [], todayRows: rows,
    timeline: { hours: ["08:00", "18:00"], blocks: [{ id: "b1", short: "job-aaaa", left: 20, width: 20, text: "10:00–12:00 · job-aaaa", current: true }, { id: "b2", short: "job-bbbb", left: 60, width: 10, text: "14:00–15:00 · job-bbbb", current: false }], nowPct: 37.5, now: "11:45" },
    day: "Today · Mon 21 Sep", otherZone: false, alerts: [], reports: [], tab: "today", sort: "severity",
    ...over,
  };
}
const link = (name: string | RegExp | ((accessibleName: string) => boolean)) => screen.getByRole("link", { name });
afterEach(() => { cleanup(); patch.mockClear(); store.jobs = []; });

describe("OverviewView", () => {
  it("lists the jobs with their links and badges each origin: plan, HQ request, client request, none (IR859)", () => {
    render(<OverviewView live={live()} />);
    expect(screen.getByText("Assigned units")).toBeTruthy();
    expect(screen.getByText("past the due date")).toBeTruthy();
    const badge = (id: string) => within(link((name) => name.startsWith(id))).queryByText(/Periodic plan|HQ request|Client request/)?.textContent ?? null;
    expect(link(/^job-aaaa · Bedroom AC/).getAttribute("href")).toBe("/technician/jobs/job-aaaa-1111");
    expect(badge("job-aaaa")).toBe("Periodic plan");
    expect(badge("job-bbbb")).toBe("HQ request");
    expect(badge("job-cccc")).toBe("Client request");
    expect(badge("job-dddd")).toBeNull();
    expect(screen.queryByText(/new assignment/i)).toBeNull();
  });

  it("asks to accept one new assignment, or names each of several and links the first", () => {
    const one = row("job-pend-0001", { ackPending: true, slotText: "21 Sep 10:00–12:00" });
    const view = render(<OverviewView live={live({ pending: [one] })} />);
    expect(screen.getByText("New assignment to accept — job-pend · Bedroom AC · 21 Sep 10:00–12:00")).toBeTruthy();
    expect(link("Review & accept").getAttribute("href")).toBe("/technician/jobs/job-pend-0001");
    view.rerender(<OverviewView live={live({ pending: [one, row("job-pend-0002", { slotText: "22 Sep 08:00–10:00" })] })} />);
    expect(screen.getByText("2 new assignments to accept")).toBeTruthy();
    expect(link("Review the first").getAttribute("href")).toBe("/technician/jobs/job-pend-0001");
    expect(link("job-pend · 22 Sep 08:00–10:00").getAttribute("href")).toBe("/technician/jobs/job-pend-0002");
    expect(screen.queryByRole("link", { name: "Review & accept" })).toBeNull();
  });

  it("puts a tab and a sort other than the defaults in the URL and drops the defaults", () => {
    const view = render(<OverviewView live={live()} />);
    fireEvent.click(screen.getByRole("tab", { name: /^All assigned/ }));
    expect(patch).toHaveBeenLastCalledWith({ tab: "all" });
    fireEvent.change(screen.getByRole("combobox", { name: "Sort" }), { target: { value: "dueAt" } });
    expect(patch).toHaveBeenLastCalledWith({ sort: "dueAt" });
    view.rerender(<OverviewView live={live({ tab: "all", sort: "dueAt" })} />);
    fireEvent.click(screen.getByRole("tab", { name: /^Today/ }));
    expect(patch).toHaveBeenLastCalledWith({ tab: null });
    fireEvent.change(screen.getByRole("combobox", { name: "Sort" }), { target: { value: "severity" } });
    expect(patch).toHaveBeenLastCalledWith({ sort: null });
  });

  it("offers every assignment from an empty Today, but not from an empty All or with none at all", () => {
    const view = render(<OverviewView live={live({ rows: [], todayRows: [] })} />);
    expect(screen.getByText("No assigned jobs today")).toBeTruthy();
    expect(screen.getByText("Nothing scheduled today.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "All assigned (6)" }));
    expect(patch).toHaveBeenLastCalledWith({ tab: "all" });
    view.rerender(<OverviewView live={live({ rows: [], tab: "all" })} />);
    expect(screen.getByText("No assigned jobs")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /All assigned/ })).toBeNull();
    view.rerender(<OverviewView live={live({ rows: [], tabs: [{ id: "today", label: "Today", count: 0 }, { id: "all", label: "All assigned", count: 0 }] })} />);
    expect(screen.getByText("No assigned jobs today")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /All assigned/ })).toBeNull();
    // a loader without an All tab counts nothing to offer
    view.rerender(<OverviewView live={live({ rows: [], tabs: [{ id: "today", label: "Today", count: 0 }] })} />);
    expect(screen.queryByRole("button", { name: /All assigned/ })).toBeNull();
  });

  it("says which of today's jobs are read-only, on Today only", () => {
    const ro = [row("job-ro-00001", { opensAt: "2026-09-21T06:00:00Z", opens: "in 2 h" }), row("job-ro-00002", { opensAt: "2026-09-21T07:00:00Z", opens: null })];
    const view = render(<OverviewView live={live({ readOnly: ro })} />);
    expect(screen.getByText("job-ro-0 shows read-only until its work window opens in 2 h — actions are disabled until then.")).toBeTruthy();
    expect(screen.getAllByText(/shows read-only until its work window opens/)).toHaveLength(2);
    view.rerender(<OverviewView live={live({ readOnly: ro, tab: "all" })} />);
    expect(screen.queryByText(/shows read-only/)).toBeNull();
  });

  it("draws today's timeline with the current job solid and now as a red line, and names the time zone when it differs", () => {
    const view = render(<OverviewView live={live()} />);
    const current = screen.getByText("job-aaaa", { selector: "div" }), later = screen.getByText("job-bbbb", { selector: "div" });
    expect(current.className).toContain("bg-primary text-white");
    expect(current.style.left).toBe("20%");
    expect(later.className).toContain("border-dashed");
    expect(later.style.width).toBe("10%");
    expect(document.querySelector(".bg-crit")?.getAttribute("style")).toContain("left: 37.5%");
    expect(screen.getByText(/Red line = now \(11:45\)/).textContent).not.toContain("Kuala Lumpur");
    expect(link(/^10:00–12:00 · job-aaaa · Bedroom AC/).getAttribute("href")).toBe("/technician/jobs/job-aaaa-1111");
    view.rerender(<OverviewView live={live({ otherZone: true, timeline: { ...live().timeline, nowPct: null } })} />);
    expect(document.querySelector(".bg-crit")).toBeNull();
    expect(screen.getByText(/Red line = now/).textContent).toContain("The timeline is in Kuala Lumpur time (Asia/Kuala_Lumpur).");
  });

  it("shows up to four alerts with All on the first, and the reports; says when there are none", () => {
    const reports: OverviewLive["reports"] = [{ id: "job-rep-1", title: "job-rep- · Lobby AC", sub: "draft in progress", badge: { text: "Draft", tone: "warn" } }];
    const view = render(<OverviewView live={live({ alerts: [1, 2, 3, 4, 5].map((n) => alert(n)), reports })} />);
    expect(screen.getByText("Alerts on my units (5)")).toBeTruthy();
    expect(screen.getByText("Alert 4")).toBeTruthy();
    expect(screen.queryByText("Alert 5")).toBeNull();
    expect(link(/^All/).getAttribute("href")).toBe("/technician/units/u1/alerts?alertId=al-1");
    expect(link(/^Alert 1/).getAttribute("href")).toBe("/technician/units/u1/alerts?alertId=al-1");
    expect(link(/^job-rep- · Lobby AC/).getAttribute("href")).toBe("/technician/jobs/job-rep-1");
    expect(screen.getByText("Draft")).toBeTruthy();
    view.rerender(<OverviewView live={live()} />);
    expect(screen.getByText("No open alerts on your units.")).toBeTruthy();
    expect(screen.queryByRole("link", { name: /^All/ })).toBeNull();
    expect(screen.getByText("No drafts, returned or submitted reports.")).toBeTruthy();
  });
});

describe("OverviewDemo", () => {
  it("shows today's fixture jobs with the read-only note, and every assigned job on All", () => {
    render(<OverviewDemo />);
    expect(screen.getByText("Today — sorted by severity, deadline, progress")).toBeTruthy();
    expect(screen.getByRole("link", { name: /^job-contractor-a/ })).toBeTruthy();
    expect(screen.queryByRole("link", { name: /^job-p05/ })).toBeNull();
    expect(screen.getByText(/job-t07 shows read-only/)).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: /^All assigned/ }));
    expect(screen.getByText("All assigned jobs")).toBeTruthy();
    expect(screen.getByRole("link", { name: /^job-p05/ })).toBeTruthy();
    expect(screen.queryByText(/job-t07 shows read-only/)).toBeNull();
    expect(link(/^Bedroom too hot/).getAttribute("href")).toBe("/technician/units/unit-online-rto/alerts");
  });

  it("opens on All from the URL and asks tech-external-a to accept the store's pending assignments", () => {
    const seed = getJobState().jobs.find((j) => j.id === "job-p09");
    if (!seed) throw new Error("seed job-p09 is missing");
    store.jobs = [
      { ...seed, id: "job-live-1", status: "assigned", techAck: { status: "pending" }, contractor: "contractor-a" },
      { ...seed, id: "job-live-2", status: "assigned", techAck: { status: "pending" }, contractor: null },
      { ...seed, id: "job-live-3", status: "assigned", techAck: { status: "accepted" } },
      { ...seed, id: "job-other", status: "assigned", techAck: { status: "pending" }, technician: "tech-external-a2" },
    ];
    render(<OverviewDemo tab="all" />);
    expect(screen.getByText("All assigned jobs")).toBeTruthy();
    expect(screen.getAllByRole("link", { name: "Review & accept" }).map((a) => a.getAttribute("href"))).toStrictEqual(["/technician/jobs/job-live-1", "/technician/jobs/job-live-2"]);
    expect(screen.getByText(/· assigned by contractor-a\./)).toBeTruthy();
    expect(screen.getByText(/· assigned by HQ\./)).toBeTruthy();
  });
});
