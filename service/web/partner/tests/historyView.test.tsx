// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { HistoryRow, TimelineItem } from "@ac/web/lib/partnerHistory";

// The contractor's job history (FR-P07, FR-P08): the list footer and empty states, the URL filters, the job detail
// (missing job, event filter, job dropdown) and the communication form (closed, note, preview only, refusals)
const actions = vi.hoisted(() => ({ communicate: vi.fn() }));
vi.mock("../app/partner/history/actions", () => actions);
const nav = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav }));
type Result = { ok: true; value: unknown } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, _ok: unknown, after?: (v: unknown) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }] as const,
}));

import { HistoryView } from "../app/partner/history/_components/history-view";
import type { HistoryLive } from "../app/partner/history/_lib/load";

beforeEach(() => {
  actions.communicate.mockReset(); nav.mockReset();
  vi.stubGlobal("ResizeObserver", class { observe = vi.fn(); unobserve = vi.fn(); disconnect = vi.fn(); }); // jsdom has none; the table's scroll box uses it
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const row = (over: Partial<HistoryRow> = {}): HistoryRow => ({
  id: "job-a", short: "job-a", title: "Periodic inspection", origin: null, version: 3, ended: false, status: "assigned", tab: "active",
  sub: "Unit 1", tech: { name: "Tan", sub: "acknowledged" }, last: { title: "Assigned", at: "2026-11-01T00:00:00Z", when: "today" }, lastAt: 0,
  badge: { text: "Assigned", tone: "primary" }, overdue: false, delegation: "active", unit: "Living AC", ...over,
});
const rows = [row(), row({ id: "job-b", short: "job-b", title: "Repair", origin: "periodic_plan", overdue: true, tech: { name: "—", sub: "", tone: "crit" } })];
const tabs = (all: number) => [{ id: "all" as const, label: "All", count: all }, { id: "active" as const, label: "Active", count: all }];
const live = (over: Partial<HistoryLive> = {}): HistoryLive => ({
  now: "2026-11-18T00:00:00Z", company: "Cool Air", sorts: [{ id: "latest", text: "Latest" }, { id: "status", text: "Status" }],
  periods: [{ id: "90d", text: "90 days" }, { id: "all", text: "All time" }], tabs: tabs(2), rows, total: 2,
  options: rows.map((r) => ({ id: r.id, label: `${r.short} · ${r.title}` })), tab: "all", sort: "latest", period: "90d", outside: 0, detail: null, ...over,
});
const items: TimelineItem[] = [
  { id: "e1", title: "Note", detail: "hello", at: "today", badge: "internal" },
  { id: "e2", title: "Assigned", detail: "", at: "yesterday", badge: "customer", tone: "ok" },
];
const recipients = [
  { id: "m-hq", role: "admin", displayLabel: "HQ Desk", allowedChannels: ["inApp", "email"] },
  { id: "m-t1", role: "technician", displayLabel: "Tan", allowedChannels: ["inApp"] },
  { id: "m-t2", role: "technician", displayLabel: "Lee", allowedChannels: ["inApp", "whatsapp"] },
];
type Detail = Extract<NonNullable<HistoryLive["detail"]>, { missing: false }>;
const detail = (over: Partial<Detail> = {}): HistoryLive => live({
  detail: { missing: false, row: row(), items, mode: { kind: "note", button: "Save note & create preview" }, recipients, ...over },
});
const preview = { id: "p1", templateKey: "schedule_change", channel: "inApp", recipientMembershipId: "m-hq", deliveryState: "preview", occurredAt: "", at: "now" };
const message = () => screen.getByRole("textbox");
/** The message text area's current text. */
const value = () => {
  const box = message();
  if (!(box instanceof HTMLTextAreaElement)) throw new TypeError("no text area");
  return box.value;
};
const send = (name = "Save note & create preview") => { fireEvent.click(screen.getByRole("button", { name })); };

describe("History list", () => {
  it("says the API total in the footer once more than 100 jobs exist, the fetched count otherwise", () => {
    const view = render(<HistoryView live={live()} />);
    expect(screen.getByText(/^Showing 2 of 2 · own company only/)).toBeTruthy();
    view.rerender(<HistoryView live={live({ total: 150, tabs: tabs(100) })} />);
    expect(screen.getByText(/^Showing 2 of 150 \(the latest 100\)/)).toBeTruthy();
  });

  it("offers all time when the 90 days are empty but older jobs exist, and says no jobs otherwise", () => {
    const view = render(<HistoryView live={live({ rows: [], outside: 1 })} />);
    expect(screen.getByText("1 job of your company is older than 90 days — it shows under all time.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Show all time" }));
    expect(nav).toHaveBeenCalledWith({ period: "all" });
    view.rerender(<HistoryView live={live({ rows: [], outside: 3 })} />);
    expect(screen.getByText("3 jobs of your company are older than 90 days — they show under all time.")).toBeTruthy();
    view.rerender(<HistoryView live={live({ rows: [], tabs: tabs(0) })} />);
    expect(screen.getByText("No jobs yet")).toBeTruthy();
    view.rerender(<HistoryView live={live({ rows: [], tab: "active" })} />);
    expect(screen.getByText("No jobs in this view")).toBeTruthy();
  });

  it("keeps the filters in the job links and drops the defaults from the URL", () => {
    render(<HistoryView live={live({ tab: "active", sort: "status", period: "all" })} />);
    expect(screen.getByRole("link", { name: "Open the history of job-a" }).getAttribute("href")).toBe("/partner/history?jobId=job-a&tab=active&sort=status&period=all");
    expect(screen.getByText("Work window ended")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Sort" }), { target: { value: "latest" } });
    expect(nav).toHaveBeenLastCalledWith({ sort: null });
    fireEvent.change(screen.getByRole("combobox", { name: "Period" }), { target: { value: "90d" } });
    expect(nav).toHaveBeenLastCalledWith({ period: null });
    fireEvent.change(screen.getByRole("combobox", { name: "Period" }), { target: { value: "all" } });
    expect(nav).toHaveBeenLastCalledWith({ period: "all" });
  });
});

describe("History detail", () => {
  it("says a job outside the company's history is missing, with a way back", () => {
    render(<HistoryView live={live({ detail: { missing: true, id: "0193a1b2-c3d4" } })} />);
    expect(screen.getByText("This job isn’t in your company’s history")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Back to the job history" }).getAttribute("href")).toBe("/partner/history");
  });

  it("filters the events by who sees them and moves to another job through the dropdown", () => {
    render(<HistoryView live={detail({ items: items.slice(0, 1) })} />);
    fireEvent.click(screen.getByRole("button", { name: "Customer-visible (0)" }));
    expect(screen.getByText("No events in this view.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Internal (1)" }));
    expect(screen.getByText("hello · today")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox", { name: "Job" }), { target: { value: "job-b" } });
    expect(nav).toHaveBeenCalledWith({ jobId: "job-b" });
  });

  it("shows only the banner, no form, once the delegation has ended", () => {
    render(<HistoryView live={detail({ row: row({ ended: true }), mode: { kind: "closed", button: "Create preview", text: "The delegation has ended." }, recipients: [] })} />);
    expect(screen.getByText("The delegation has ended.")).toBeTruthy();
    expect(screen.getByText(/^Delegation ended — only your company’s own decisions/)).toBeTruthy();
    expect(screen.queryByRole("textbox")).toBeNull();
  });
});

describe("History communication", () => {
  it("refuses an empty and a too-long message without calling the action", () => {
    render(<HistoryView live={detail()} />);
    send();
    expect(screen.getByText("A message is required — nothing was saved")).toBeTruthy();
    fireEvent.change(message(), { target: { value: "x".repeat(2001) } });
    expect(screen.getByText("At most 2000 characters")).toBeTruthy();
    send();
    expect(actions.communicate).not.toHaveBeenCalled();
  });

  it("saves a customer-visible note with the preview, lists the preview and clears the message", async () => {
    actions.communicate.mockResolvedValue({ ok: true, value: { noteSaved: true, preview } });
    render(<HistoryView live={detail()} />);
    fireEvent.click(screen.getByRole("button", { name: "Customer-visible" }));
    fireEvent.change(screen.getByRole("combobox", { name: "Channel" }), { target: { value: "email" } });
    fireEvent.change(message(), { target: { value: "Moved to 9:00" } });
    send();
    expect(actions.communicate).toHaveBeenCalledWith({ jobId: "job-a", version: 3, saveNote: true, templateKey: "schedule_change", channel: "email", recipientMembershipId: "m-hq", message: "Moved to 9:00", visibility: "customer" });
    expect(await screen.findByText(/^Saved the note \(customer-visible\)/)).toBeTruthy();
    expect(screen.getByText("schedule_change → hq (HQ Desk)")).toBeTruthy();
    expect(value()).toBe("");
  });

  it("picks the technician chosen in the role and recipient lists, and offers only that person's channels", async () => {
    actions.communicate.mockResolvedValue({ ok: true, value: { noteSaved: false, preview } });
    render(<HistoryView live={detail({ mode: { kind: "preview", button: "Create preview", add: true, text: "Notes are closed." } })} />);
    expect(screen.queryByRole("button", { name: "Customer-visible" })).toBeNull();
    expect(screen.getByRole("option", { name: "customer_contact" }).hasAttribute("disabled")).toBe(true);
    fireEvent.change(screen.getByRole("combobox", { name: /^Recipient role/ }), { target: { value: "technician" } });
    fireEvent.change(screen.getByRole("combobox", { name: "Recipient" }), { target: { value: "m-t2" } });
    expect(screen.getAllByRole("option", { name: /preview only$/ }).map((o) => o.textContent)).toStrictEqual(["inApp · preview only", "whatsapp · preview only"]);
    fireEvent.change(message(), { target: { value: "On my way" } });
    send("Create preview");
    expect(actions.communicate).toHaveBeenCalledWith(expect.objectContaining({ saveNote: false, recipientMembershipId: "m-t2", channel: "inApp" }));
    expect(await screen.findByText("Created the in-app preview for Lee. Nothing was sent.")).toBeTruthy();
    expect(value()).toBe("On my way");
  });

  it("says whether the note was kept when the communication fails", async () => {
    const failure = { ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} };
    actions.communicate.mockResolvedValueOnce({ ...failure, stage: "preview", noteSaved: true }).mockResolvedValueOnce({ ...failure, stage: "note", noteSaved: false });
    render(<HistoryView live={detail()} />);
    fireEvent.change(message(), { target: { value: "Hi" } });
    send();
    expect(await screen.findByText(/^The note was saved, but the preview failed: UNAVAILABLE/)).toBeTruthy();
    send();
    expect(await screen.findByText(/^Not saved: UNAVAILABLE .* Your message is kept\.$/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(value()).toBe("");
    expect(screen.queryByText(/^Not saved/)).toBeNull();
  });
});
