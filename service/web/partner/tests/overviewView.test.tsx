// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionRow, CapacityRow, TimelineRow } from "@ac/web/lib/partnerOverview";

// The contractor overview (FR-P01): the period choice through the URL, the progress bar and legend, the action list,
// the capacity list with its cut and warn tone, today's timeline (blocks, away, free, the now line, the cut) and the
// activity, each with its empty state
const nav = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav }));

import { OverviewView } from "../app/partner/(overview)/_components/overview-view";
import type { OverviewLive } from "../app/partner/(overview)/_lib/load";

beforeEach(() => {
  nav.mockReset();
  vi.stubGlobal("ResizeObserver", class { observe = vi.fn(); unobserve = vi.fn(); disconnect = vi.fn(); }); // jsdom has none; the timeline's scroll box uses it
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const counts = { offered: 1, unassigned: 2, active: 0, overdue: 0, review: 1, completed: 4 };
const tech = (i: number, over: Partial<TimelineRow> = {}): TimelineRow => ({ id: `m-${String(i)}`, name: `Tech ${String(i)}`, sub: "Indoor", blocks: [], off: null, ...over });
const cap = (i: number, pct: number | null): CapacityRow => ({ id: `m-${String(i)}`, name: `Tech ${String(i)}`, text: pct === null ? "—" : `${String(pct)}%`, pct });
const action = (kind: ActionRow["kind"], href: string): ActionRow => ({ kind, href, badge: `${kind} badge`, title: `${kind} title`, detail: `${kind} detail`, button: `${kind} button` });
const live = (over: Partial<OverviewLive> = {}): OverviewLive => ({
  updated: "09:30", otherZone: false,
  period: { value: "2026-09-21|2026-09-27", custom: false, text: "this week", label: "21 Sep – 27 Sep" },
  periods: [{ value: "2026-09-14|2026-09-20", label: "Last week" }, { value: "2026-09-21|2026-09-27", label: "This week" }],
  today: { title: "Today · Mon 21 Sep", nowPct: 15, now: "09:30" },
  kpis: [{ label: "Offers to answer", value: 1, sub: "job-1", href: "/partner/jobs?tab=offered", link: "Open offers" }, { label: "Overdue", value: 0, sub: "", href: "/partner/jobs?tab=overdue", link: "View", tone: "crit" }],
  progress: { counts, total: 8, completedPct: 50 },
  actions: [action("offer", "/partner/jobs/job-1"), action("overdue", "/partner/schedule?jobId=job-2"), action("unassigned", "/partner/schedule?jobId=job-3"), action("unassigned", "/partner/schedule?jobId=job-4")],
  timeline: [tech(1, { blocks: [{ left: 20, width: 20, text: "10:00–12:00" }] }), tech(2, { off: "annual leave" }), tech(3)],
  capacity: [cap(1, 95), cap(2, 40), cap(3, null)],
  activity: [{ id: "e1", time: "today 09:05", text: "Tech 1 checked in — job-3", href: "/partner/history?jobId=job-3" }],
  jobCount: 8,
  ...over,
});
const card = (title: string) => {
  const box = screen.getByText(title).closest("[data-slot=card]");
  if (!(box instanceof HTMLElement)) throw new TypeError(`no card ${title}`);
  return within(box);
};
const nowLines = () => [...document.querySelectorAll<HTMLElement>(".bg-crit.w-px")];

describe("Overview view", () => {
  it("moves the period through the URL, offering the custom range only when it is the current one", () => {
    const view = render(<OverviewView live={live()} />);
    const period = () => screen.getByRole("combobox", { name: "Period" });
    expect(within(period()).getAllByRole("option").map((o) => o.textContent)).toStrictEqual(["Last week", "This week"]);
    fireEvent.change(period(), { target: { value: "2026-09-14|2026-09-20" } });
    expect(nav).toHaveBeenLastCalledWith({ from: "2026-09-14", to: "2026-09-20" });
    // a custom value without its end reads as an open end
    view.rerender(<OverviewView live={live({ period: { value: "2026-09-01", custom: true, text: "1 Sep", label: "1 Sep –" } })} />);
    expect(within(period()).getAllByRole("option").map((o) => o.textContent)).toStrictEqual(["1 Sep –", "Last week", "This week"]);
    fireEvent.change(period(), { target: { value: "2026-09-14|2026-09-20" } }); // away from the custom value, so choosing it again fires a change
    fireEvent.change(period(), { target: { value: "2026-09-01" } });
    expect(nav).toHaveBeenLastCalledWith({ from: "2026-09-01", to: "" });
    expect(screen.getByText("Updated 09:30 · your company’s jobs only")).toBeTruthy();
  });

  it("draws only the non-empty statuses, sized by their share, counts every status in the legend, and links the KPI tiles", () => {
    render(<OverviewView live={live()} />);
    expect(screen.getByText("8 jobs · 4 completed (50%) · counts use the same period as the Jobs list")).toBeTruthy();
    const bar = screen.getByRole("img", { name: "Jobs by status" });
    expect([...bar.querySelectorAll<HTMLElement>(":scope > div")].map((c) => [c.getAttribute("title"), c.style.width])).toStrictEqual([
      ["Offered: 1", "12.5%"], ["Accepted · unassigned: 2", "25%"], ["Submitted · in review: 1", "12.5%"], ["Completed: 4", "50%"],
    ]);
    expect(screen.getByText("Assigned / in progress").querySelector("b")?.textContent).toBe("0");
    expect(screen.getByText("Open offers").closest("a")?.getAttribute("href")).toBe("/partner/jobs?tab=offered");
  });

  it("names one job, and no jobs with an empty bar", () => {
    const view = render(<OverviewView live={live({ progress: { counts: { ...counts, offered: 0, unassigned: 0, review: 0, completed: 1 }, total: 1, completedPct: 100 } })} />);
    expect(screen.getByText("1 job · 1 completed (100%) · counts use the same period as the Jobs list")).toBeTruthy();
    view.rerender(<OverviewView live={live({ progress: { counts: { ...counts, offered: 0, unassigned: 0, review: 0, completed: 0 }, total: 0, completedPct: 0 } })} />);
    expect(screen.getByText("No jobs yet — offers from HQ to your company will appear here and in Jobs.")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Jobs by status" }).children).toHaveLength(0);
  });

  it("lists the actions with their badge tone and button, and warns about the accepted jobs without a technician", () => {
    render(<OverviewView live={live()} />);
    const actions = card("Needs your action (4)");
    expect(actions.getByRole("link", { name: "offer button" }).getAttribute("href")).toBe("/partner/jobs/job-1");
    expect(actions.getByRole("link", { name: "overdue button" }).getAttribute("href")).toBe("/partner/schedule?jobId=job-2");
    expect(actions.getByText("overdue badge").className).toMatch(/crit/);
    expect(actions.getByText("offer badge").className).not.toMatch(/crit|warn/);
    expect(actions.getAllByText("unassigned badge").map((b) => /crit|warn/.test(b.className))).toStrictEqual([false, false]);
    expect(actions.getByText("offer detail")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain("2 accepted jobs still have no technician — assign them before the delegation period ends.");
  });

  it("warns in the singular about one unassigned job, and shows no banner without one", () => {
    const view = render(<OverviewView live={live({ actions: [action("unassigned", "/partner/schedule?jobId=job-3")] })} />);
    expect(screen.getByRole("status").textContent).toContain("1 accepted job still has no technician — assign it before the delegation period ends.");
    view.rerender(<OverviewView live={live({ actions: [action("review", "/partner/jobs/job-5/review")] })} />);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("marks a technician at 90 % or more in warn, and cuts the capacity and the timeline after 8 with a link to the rest", () => {
    const view = render(<OverviewView live={live()} />);
    const bars = screen.getAllByRole("progressbar");
    expect(bars.map((b) => b.getAttribute("aria-label"))).toStrictEqual(["Tech 1: 95%", "Tech 2: 40%", "Tech 3: —"]);
    const tone = (label: string) => {
      const bar = screen.getByRole("progressbar", { name: label }).closest("[data-slot=progress]");
      if (!bar) throw new TypeError(`no progress element for ${label}`);
      return bar.className;
    };
    expect(tone("Tech 1: 95%")).toMatch(/bg-warn/);
    expect(tone("Tech 2: 40%")).not.toMatch(/bg-warn/);
    expect(tone("Tech 3: —")).not.toMatch(/bg-warn/);
    expect(screen.queryByText(/more technicians/)).toBeNull();
    const many = Array.from({ length: 10 }, (_, i) => i + 1);
    view.rerender(<OverviewView live={live({ capacity: many.map((i) => cap(i, 90)), timeline: many.slice(0, 9).map((i) => tech(i)) })} />);
    expect(screen.getAllByRole("progressbar")).toHaveLength(8);
    expect(tone("Tech 1: 90%")).toMatch(/bg-warn/);
    expect(screen.getByRole("link", { name: "2 more technicians" }).getAttribute("href")).toBe("/partner/team");
    expect(screen.getByRole("link", { name: "1 more technicians" }).getAttribute("href")).toBe("/partner/schedule");
    expect(screen.getAllByText("Free all day")).toHaveLength(8);
  });

  it("places today's work blocks, names the away reason and the free day, and draws the now line on every row", () => {
    render(<OverviewView live={live()} />);
    const block = screen.getByText("10:00–12:00");
    expect([block.style.left, block.style.width]).toStrictEqual(["20%", "20%"]);
    expect(screen.getByText("Unavailable · annual leave")).toBeTruthy();
    expect(screen.getByText("Free all day")).toBeTruthy();
    expect(nowLines()).toHaveLength(3);
    expect(nowLines().map((l) => l.style.left)).toStrictEqual(["15%", "15%", "15%"]);
    expect(screen.getByText(/Red line = now \(09:30\)\. Blue = assigned work windows\./)).toBeTruthy();
    expect(screen.queryByText(/Kuala Lumpur time/)).toBeNull();
    expect(screen.getByRole("link", { name: "Tech 1 checked in — job-3" }).getAttribute("href")).toBe("/partner/history?jobId=job-3");
  });

  it("hides the now line outside 08:00–18:00, names the KL zone for another display zone, and says there is no work today", () => {
    const view = render(<OverviewView live={live({ otherZone: true, timeline: [tech(1)], today: { title: "Today", nowPct: 0, now: "07:00" } })} />);
    expect(nowLines()).toHaveLength(0);
    expect(screen.getByText(/No assignments today\./).textContent).toContain("The timeline is in Kuala Lumpur time (Asia/Kuala_Lumpur).");
    view.rerender(<OverviewView live={live({ timeline: [tech(1)], today: { title: "Today", nowPct: 100, now: "19:00" } })} />);
    expect(nowLines()).toHaveLength(0);
  });

  it("shows each section's empty state", () => {
    render(<OverviewView live={live({ actions: [], capacity: [], timeline: [], activity: [] })} />);
    expect(screen.getByText("Nothing needs your action")).toBeTruthy();
    expect(screen.getAllByText("No technicians in your company yet.")).toHaveLength(2);
    expect(screen.getByText("No job activity in this period.")).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
  });
});
