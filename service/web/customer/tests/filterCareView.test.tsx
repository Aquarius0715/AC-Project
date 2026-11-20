// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import { filterLines, reminderForm, reminderLines, type ApiFilterSettings, type ApiFilterStatus, type FilterUnit } from "@ac/web/lib/customerFilterCare";
import type { MaintenanceLive } from "../app/customer/maintenance/_lib/load";

// Filter care (FR-C18, DD-C18) in API mode: one row per AC, an area of 4 ACs or more folded into “View n” (open when a
// link names one of its ACs), Request cleaning prefilled as a preventive request, the batch request for every overdue
// filter (all sent, some sent, none sent), Mark cleaned, and the owner's reminder settings with their checks.
type Done<T> = { ok: true; value: T } | ({ ok: false } & ActionFailure);
const actions = vi.hoisted(() => ({
  createRequest: vi.fn<(input: unknown) => Promise<Done<{ id: string }>>>(),
  createCleaningRequests: vi.fn<(input: unknown) => Promise<Done<{ ids: string[]; stopped: ActionFailure | null }>>>(),
  markCleaned: vi.fn<(unitId: string) => Promise<Done<null>>>(),
  saveReminders: vi.fn<(input: unknown) => Promise<Done<null>>>(),
  respondProposal: vi.fn(), requestReschedule: vi.fn(), cancelRequest: vi.fn(), addNote: vi.fn(), rateJob: vi.fn(), reportProblem: vi.fn(),
}));
vi.mock("../app/customer/maintenance/actions", () => actions);
const nav = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn(), params: new URLSearchParams() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: nav.push, replace: nav.replace }), useSearchParams: () => nav.params }));
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, <T,>(fn: () => Promise<Done<T>>, _ok: unknown, after?: (v: T) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }] as const,
}));

import { MaintenanceView } from "../app/customer/maintenance/_components/maintenance-view";

const NOW = Date.parse("2026-09-15T01:00:00Z");
const unit = (id: string, name: string, spaceId: string | null): FilterUnit => ({ id, name, spaceId, place: spaceId ? "Office A › Hall" : "Home A", connection: "online" });
/** the bar's tone from its class: a warning, another tone, or the plain bar */
const barTone = (c: string) => {
  if (c.includes("bg-warn")) return "warn";
  return c.includes("bg-") ? "other" : "plain";
};
const units = [unit("u-bed", "Bedroom AC", null), unit("u-liv", "Living AC", null), ...["1", "2", "3", "4"].map((n) => unit(`u-hall-${n}`, `Hall AC ${n}`, "s-hall"))];
const status = (unitId: string, over: Partial<ApiFilterStatus> = {}): ApiFilterStatus => ({
  unitId, runHoursSinceCleaning: 120, thresholdHours: 250, fallbackDays: 90, lastCleanedAt: "2026-08-21T00:00:00Z", lastCleanedBy: "customer", lastCleaningJobId: null, status: "ok", ...over,
});
const items = [
  status("u-bed", { runHoursSinceCleaning: 300, status: "overdue" }), status("u-liv", { lastCleanedBy: "technician", lastCleaningJobId: "job-tech-12345" }),
  status("u-hall-1", { runHoursSinceCleaning: 280, status: "overdue" }), status("u-hall-2"), status("u-hall-3"), status("u-hall-4"),
];
const settings = (over: Partial<ApiFilterSettings> = {}): ApiFilterSettings => ({ id: "fs-1", version: 0, customerId: "org-a", thresholdHours: null, fallbackDays: 90, recipients: "owners", channels: ["inApp"], ...over });
const live = (owner = true, s = settings(), list = items, known = units): MaintenanceLive => ({
  owner, now: new Date(NOW).toISOString(), units: units.map((u) => ({ id: u.id, name: u.name, place: u.place })), display: { locale: "en", timeZone: "Asia/Kuala_Lumpur" }, page: "filters", requestUnitId: null,
  rows: [], rateBanner: null, detail: null, filters: { lines: filterLines(list, known, NOW), settings: s, reminders: reminderLines(s), form: reminderForm(s) },
});
const button = (name: string | RegExp) => screen.getByRole("button", { name });
const dialog = () => screen.getByRole("dialog");
const scrolled = vi.fn();

beforeEach(() => {
  nav.params = new URLSearchParams();
  nav.push.mockReset();
  Element.prototype.scrollIntoView = scrolled;
  scrolled.mockReset();
  actions.createRequest.mockReset().mockResolvedValue({ ok: true, value: { id: "job-new-12345" } });
  actions.createCleaningRequests.mockReset().mockResolvedValue({ ok: true, value: { ids: ["job-c-1", "job-c-2"], stopped: null } });
  actions.markCleaned.mockReset().mockResolvedValue({ ok: true, value: null });
  actions.saveReminders.mockReset().mockResolvedValue({ ok: true, value: null });
});
afterEach(() => { cleanup(); });

describe("Filter care", () => {
  it("folds an area of 4 ACs into View n, links the technician's cleaning job and opens the area of the AC a link named, once in view", () => {
    nav.params = new URLSearchParams("tab=filter-care&unitId=u-hall-3");
    const { rerender } = render(<MaintenanceView live={live()} />);
    expect(screen.getByRole("link", { name: "job-tech" }).getAttribute("href")).toBe("/customer/maintenance?jobId=job-tech-12345");
    const toggle = button("Hide");
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Hall AC 3").closest("tr")?.getAttribute("aria-current")).toBe("true");
    expect(scrolled).toHaveBeenCalledOnce();
    rerender(<MaintenanceView live={live()} />);
    expect(scrolled).toHaveBeenCalledOnce();
    fireEvent.click(toggle);
    expect(screen.queryByText("Hall AC 3")).toBeNull();
    fireEvent.click(button("View 4"));
    expect(screen.getByText("Hall AC 2")).toBeDefined();
    expect(screen.getByText(/reminder at 250 h \(model default\)/)).toBeDefined();
    expect(screen.getByText("Default settings")).toBeDefined();
  });

  it("Request cleaning opens New request for that AC as a preventive request with the run-time line", async () => {
    render(<MaintenanceView live={live()} />);
    fireEvent.click(within(screen.getByText("Bedroom AC").closest("tr") ?? document.body).getByRole("button", { name: "Request cleaning" }));
    expect(within(dialog()).getByRole<HTMLSelectElement>("combobox", { name: /Unit/ }).value).toBe("u-bed");
    expect(within(dialog()).getByRole("button", { name: "Preventive" }).getAttribute("aria-pressed")).toBe("true");
    expect(within(dialog()).getByRole<HTMLTextAreaElement>("textbox", { name: /Symptoms/ }).value).toBe("Filter cleaning: 300 h of running since the last cleaning (reminder at 250 h).");
    fireEvent.click(within(dialog()).getByRole("button", { name: "Submit request" }));
    await screen.findByText(/Request job-new- sent/);
    expect(actions.createRequest).toHaveBeenCalledWith(expect.objectContaining({ unitId: "u-bed", type: "preventive" }));
  });

  it("the batch sends one request per ticked overdue AC, needs one at least, and says how many went when a refusal stopped it", async () => {
    render(<MaintenanceView live={live()} />);
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    expect(within(dialog()).getByText("ACs to clean (2 of 2)")).toBeDefined();
    const boxes = within(dialog()).getAllByRole("checkbox");
    for (const box of boxes) fireEvent.click(box);
    expect(within(dialog()).getByRole("button", { name: "Send 0 requests" }).hasAttribute("disabled")).toBe(true);
    fireEvent.click(boxes[1] ?? dialog());
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Contact window/ }), { target: { value: "me@example.com" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 1 request" }));
    expect(actions.createCleaningRequests).not.toHaveBeenCalled();
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Contact window/ }), { target: { value: "" } });
    fireEvent.click(boxes[0] ?? dialog());
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 2 requests" }));
    await screen.findByText(/2 cleaning requests sent with 3 preferred times/);
    // in the list's order, whatever the order they were ticked in
    expect(actions.createCleaningRequests.mock.lastCall?.[0]).toMatchObject({ units: [{ unitId: "u-bed" }, { unitId: "u-hall-1" }], contactWindow: "" });
    const stopped = { code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: 30 };
    actions.createCleaningRequests.mockResolvedValueOnce({ ok: true, value: { ids: ["job-c-1"], stopped } });
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 2 requests" }));
    await screen.findByText(/^1 of 2 cleaning requests sent; the rest were not sent:/);
    actions.createCleaningRequests.mockResolvedValueOnce({ ok: true, value: { ids: [], stopped: { ...stopped, code: "FORBIDDEN", messageKey: "error.forbidden", retryAfterSeconds: null } } });
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 2 requests" }));
    await screen.findByText(/You don't have permission for this action/);
    expect(screen.queryByText(/cleaning requests sent/)).toBeNull();
  });

  it("the batch for a single AC says 1 request; the batch button needs two overdue filters", async () => {
    actions.createCleaningRequests.mockResolvedValueOnce({ ok: true, value: { ids: ["job-c-1"], stopped: null } });
    const { unmount } = render(<MaintenanceView live={live()} />);
    fireEvent.click(button("Request cleaning for all overdue (2)"));
    fireEvent.click(within(dialog()).getAllByRole("checkbox")[0] ?? dialog());
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send 1 request" }));
    await screen.findByText(/^1 cleaning request sent with 3 preferred times/);
    unmount();
    render(<MaintenanceView live={live(true, settings(), items.slice(1))} />);
    expect(screen.queryByRole("button", { name: /Request cleaning for all overdue/ })).toBeNull();
  });

  it("Mark cleaned restarts the counter and says so; a refusal is shown", async () => {
    actions.markCleaned.mockResolvedValueOnce({ ok: true, value: null }).mockResolvedValueOnce({ ok: false, code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} })
      .mockResolvedValueOnce({ ok: false, code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {} });
    render(<MaintenanceView live={live()} />);
    fireEvent.click(button("Mark cleaned — Living AC"));
    await screen.findByText("Living AC: marked cleaned — the counter restarts at 0 h and your technician sees it.");
    expect(actions.markCleaned).toHaveBeenLastCalledWith("u-liv");
    fireEvent.click(button("Mark cleaned — Bedroom AC"));
    await screen.findByText("Not saved: that AC is no longer in your account.");
    fireEvent.click(button("Mark cleaned — Bedroom AC"));
    await screen.findByText("Not saved: only the account owner can change the reminders.");
  });

  it("only the owner edits the reminders: the hours and days are checked, and the default or a custom threshold is saved", async () => {
    const { unmount } = render(<MaintenanceView live={live(false)} />);
    expect(button("Edit reminders").hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Only the account owner can change the reminders.")).toBeDefined();
    unmount();
    render(<MaintenanceView live={live(true, settings({ version: 3, thresholdHours: 300, channels: ["inApp", "email"] }))} />);
    expect(screen.getByText("Set by the account owner")).toBeDefined();
    expect(screen.getByText(/reminder at 300 h/)).toBeDefined();
    fireEvent.click(button("Edit reminders"));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: "Hours of running" }), { target: { value: "20" } });
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Also remind every/ }), { target: { value: "200" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Save reminders" }));
    expect(within(dialog()).getByText("Whole hours from 50 to 2000.")).toBeDefined();
    expect(within(dialog()).getByText("Whole days from 7 to 180.")).toBeDefined();
    expect(actions.saveReminders).not.toHaveBeenCalled();
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Also remind every/ }), { target: { value: "30" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Model default (250 h)" }));
    expect(within(dialog()).queryByRole("textbox", { name: "Hours of running" })).toBeNull();
    fireEvent.click(within(dialog()).getByRole("button", { name: "All users of the location" }));
    fireEvent.click(within(dialog()).getByRole("checkbox", { name: "E-mail" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Save reminders" }));
    await screen.findByText("Reminder settings saved.");
    expect(actions.saveReminders).toHaveBeenLastCalledWith({ thresholdHours: null, fallbackDays: 30, recipients: "all_users", channels: ["inApp"] });
    actions.saveReminders.mockResolvedValueOnce({ ok: false, code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {} });
    fireEvent.click(button("Edit reminders"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Save reminders" }));
    await screen.findByText("Not saved: only the account owner can change the reminders.");
    fireEvent.click(button("Edit reminders"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("an AC without a place or readings, and the tone of an area due soon or fine", () => {
    const loose: FilterUnit = { id: "u-loose", name: "Loose AC", place: "", spaceId: null, connection: "offline" };
    const area = (space: string, state: ApiFilterStatus["status"]) => ["a", "b", "c", "d"].map((n) => ({ u: unit(`u-${space}-${n}`, `${space} AC ${n}`, space), s: status(`u-${space}-${n}`, { status: state }) }));
    const all = [...area("due", "due_soon"), ...area("fine", "ok")];
    const list = [status("u-loose", { runHoursSinceCleaning: null }), ...all.map((x) => x.s)];
    render(<MaintenanceView live={live(true, settings(), list, [loose, ...all.map((x) => x.u)])} />);
    const row = screen.getByText("Loose AC").closest("tr") ?? document.body;
    expect(within(row).getByText("offline")).toBeDefined();
    expect(within(row).getByText(/^last cleaned/).textContent).toMatch(/^last cleaned \d+ Aug$/);
    // the area rows: due soon draws a warning bar, a fine area the plain one
    const bars = screen.getAllByRole("button", { name: "View 4" }).map((b) => b.closest("tr")?.querySelector("[aria-label]")?.className ?? "");
    expect(bars.map((c) => barTone(c))).toStrictEqual(["warn", "plain"]);
    fireEvent.click(button("Edit reminders"));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("without ACs the table says so", () => {
    render(<MaintenanceView live={live(true, settings(), [])} />);
    expect(screen.getByText("No ACs in your account yet.")).toBeDefined();
  });
});
