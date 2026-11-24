// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ActionFailure } from "@ac/web/lib/actionMessage";
import type { ApiCapacity, ApiMember, Slot } from "@ac/web/lib/partnerOverview";
import { assignRefusal, type Candidate, type FormMode, type ScheduleRow, type ScheduleSort } from "@ac/web/lib/partnerSchedule";

// The contractor's schedule & assignments (FR-P03): the empty list, the sort and the delegation windows, the assign,
// reassign and extend forms with their checks before saving, the refusals, a blocked job, and the team's week with
// "This proposal"
const actions = vi.hoisted(() => ({ assignTechnician: vi.fn() }));
vi.mock("../app/partner/schedule/actions", () => actions);
const nav = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/useUrlPatch", () => ({ useUrlPatch: () => nav }));
type Result = { ok: true; value: { slot: string } } | ({ ok: false } & ActionFailure);
vi.mock("@ac/web/lib/useAction", () => ({
  useAction: () => [false, (fn: () => Promise<Result>, _ok: unknown, after?: (v: { slot: string }) => void, failed?: (f: ActionFailure) => void) => {
    void fn().then((r) => { if (r.ok) after?.(r.value); else failed?.(r); });
  }] as const,
}));
// the date-and-time picker as one text box: the view only passes its value on
vi.mock(import("@ac/web/components/TimeSelect"), async (importOriginal) => ({
  ...(await importOriginal()),
  DateTimeSelect: ({ value, onChange }: Readonly<{ value: string; onChange: (v: string) => void }>) => <input aria-label="Work end" value={value} onChange={(e) => { onChange(e.target.value); }} />,
}));

import { ScheduleView } from "../app/partner/schedule/_components/schedule-view";
import type { ScheduleLive } from "../app/partner/schedule/_lib/load";

beforeEach(() => {
  actions.assignTechnician.mockReset();
  nav.mockReset();
  vi.stubGlobal("ResizeObserver", class { observe = vi.fn(); unobserve = vi.fn(); disconnect = vi.fn(); }); // jsdom has none; the week's scroll box uses it
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

type Selected = NonNullable<ScheduleLive["selected"]>;
type Week = NonNullable<ScheduleLive["week"]>;
const slot: Slot = { startAt: "2026-09-21T02:00:00Z", endAt: "2026-09-21T04:00:00Z" }; // 10:00–12:00 in Kuala Lumpur
const other: Slot = { startAt: "2026-09-23T02:00:00Z", endAt: "2026-09-23T04:00:00Z" };
const row = (over: Partial<ScheduleRow> = {}): ScheduleRow => ({
  id: "job-1", short: "job-1", title: "Rooftop unit", line: "Accepted · awaiting assignment", window: { from: "2026-09-14T00:00:00Z", until: "2026-09-25T00:00:00Z" },
  windowText: "14 Sep – 25 Sep", left: "3 d left", ended: false, leftTone: "primary", elapsedPct: 40, ...over,
});
const member = (id: string, name: string): ApiMember => ({ id, userId: `u-${id}`, displayName: name, role: "technician" });
const members = [member("m-1", "Aina"), member("m-2", "Badrul")];
const cand = (id: string, name: string, eligible = true, badge = "Qualified"): Candidate => ({ id, name, eligible, sub: "Refrigerant · 8 h free", badge: { text: badge, tone: eligible ? "ok" : "warn" } });
const dates = ["2026-09-21", "2026-09-22", "2026-09-23", "2026-09-24", "2026-09-25", "2026-09-26", "2026-09-27"];
const capacity = (id: string): ApiCapacity => ({ membershipId: id, date: "", availableSlots: [], assignedSlots: [], availableMinutes: 480, assignedMinutes: 0, unavailability: null });
const week = (over: Partial<Week> = {}): Week => ({
  from: dates[0] ?? "", jobsOf: {}, title: "Team schedule — week of 21 Sep", today: "2026-09-22", days: dates.map((d) => `Day ${d.slice(8)}`), dates,
  capacity: dates.map(() => [capacity("m-1"), capacity("m-2")]), ...over,
});
const assignMode: FormMode = { slot, kind: "assign", title: "Assign — job-1 · Rooftop unit", choices: [slot, other], endEditable: false, reasonRequired: false, button: "Confirm assignment", currentTech: null };
const selected = (over: Partial<Selected> = {}): Selected => ({
  id: "job-1", short: "job-1", version: 3, ack: null, moved: false, times: [{ text: "21 Sep 10:00–12:00", on: true }, { text: "23 Sep 10:00–12:00", on: false }],
  window: row().window, candidates: [cand("m-1", "Aina"), cand("m-2", "Badrul")],
  text: { window: "14 Sep – 25 Sep", slot: "21 Sep 10:00–12:00", start: "21 Sep 10:00", end: "2026-09-21T12:00", alternative: null },
  label: "job-1 · Rooftop unit", mode: assignMode, currentId: null, required: [], ...over,
});
// the loader returns a selected job with its week, or neither
interface Over extends Partial<Omit<ScheduleLive, "selected" | "week">> { selected?: Selected | null; week?: Week }
const live = (over: Over = {}): ScheduleLive => {
  const { selected: sel = selected(), week: grid = week(), ...rest } = over;
  const sort: ScheduleSort = "status";
  const sorts: ScheduleLive["sorts"] = [{ id: "status", text: "Status" }, { id: "dueAt", text: "Deadline" }];
  const base = {
    members, sort, sorts, now: "2026-09-22T00:00:00Z", zone: "Asia/Kuala_Lumpur", otherZone: false,
    rows: [row(), row({ id: "job-2", short: "job-2", title: "Lobby AC", tone: "crit", line: "Overdue", ended: true, leftTone: "crit", left: "Ended", elapsedPct: 120 })], ...rest,
  };
  return sel ? { ...base, selected: sel, week: grid } : { ...base, selected: null, week: null };
};
const save = (name: RegExp | string) => { fireEvent.click(screen.getByRole("button", { name })); };
const banner = (text: RegExp | string) => screen.getByText(text);

describe("ScheduleView list", () => {
  it("shows the empty state with a link to the offers", () => {
    render(<ScheduleView live={live({ rows: [], selected: null })} />);
    expect(screen.getByText("No accepted jobs")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Open offers/u }).getAttribute("href")).toBe("/partner/jobs?tab=offered");
  });

  it("links each row with the sort kept, patches the sort, fills an ended window, and tells when the job is no longer delegated", () => {
    const { rerender } = render(<ScheduleView live={live()} />);
    const links = screen.getAllByRole("link").filter((a) => a.getAttribute("href")?.startsWith("/partner/schedule"));
    expect(links.map((a) => a.getAttribute("href"))).toStrictEqual(["/partner/schedule?jobId=job-1", "/partner/schedule?jobId=job-2"]);
    expect(screen.getByText("job-1 · Rooftop unit").className).toContain("text-primary");
    expect(screen.getByText("Overdue").className).toContain("text-crit");
    expect(screen.getByText("Accepted · awaiting assignment").className).toContain("text-muted-foreground");
    const bars = [...document.querySelectorAll<HTMLElement>(".h-full.rounded-full")].map((b) => b.style.width);
    expect(bars).toStrictEqual(["40%", "100%"]);
    fireEvent.change(screen.getByRole("combobox", { name: "Sort" }), { target: { value: "dueAt" } });
    fireEvent.change(screen.getByRole("combobox", { name: "Sort" }), { target: { value: "status" } });
    expect(nav.mock.calls).toStrictEqual([[{ sort: "dueAt" }], [{ sort: null }]]);
    rerender(<ScheduleView live={live({ sort: "dueAt", selected: null })} />);
    expect(screen.getAllByRole("link").map((a) => a.getAttribute("href"))).toContain("/partner/schedule?jobId=job-2&sort=dueAt");
    expect(screen.getByText("The job is no longer delegated to your company — pick another one.")).toBeTruthy();
  });
});

describe("ScheduleView assign", () => {
  it("preselects the first eligible technician, picks a visit time through the URL, saves and shows the result", async () => {
    actions.assignTechnician.mockResolvedValue({ ok: true, value: { slot: "21 Sep 10:00–12:00" } });
    const { rerender } = render(<ScheduleView live={live()} />);
    expect(screen.getByRole("radio", { name: /Aina/u }).getAttribute("aria-checked")).toBe("true");
    expect(banner("No overlapping confirmed schedule for Aina in this window.")).toBeTruthy();
    fireEvent.click(screen.getByRole("radio", { name: "23 Sep 10:00–12:00" }));
    expect(nav).toHaveBeenCalledExactlyOnceWith({ slot: "1" });
    // the week shows the proposal in Aina's Monday cell
    const aina = screen.getByRole("row", { name: /Aina/u });
    expect(within(aina).getByText("10:00–12:00").parentElement?.className).toContain("border-2");
    fireEvent.click(screen.getByRole("radio", { name: /Badrul/u }));
    save("Confirm assignment");
    expect(await screen.findByText("Assigned Badrul for 21 Sep 10:00–12:00. The technician must accept the assignment; HQ and the technician are notified.")).toBeTruthy();
    expect(actions.assignTechnician).toHaveBeenCalledExactlyOnceWith("job-1", 3, "m-2", slot, "");
    expect(screen.queryByRole("textbox")).toBeNull(); // no reason on a first assignment
    // a re-read leaves one agreed time and no window, and Badrul has left the list: the time is fixed, the name generic
    rerender(<ScheduleView live={live({ selected: selected({ times: [{ text: "21 Sep 10:00–12:00", on: true }], candidates: [cand("m-1", "Aina")], text: { ...selected().text, window: null } }) })} />);
    expect(screen.getByText("Visit time (agreed with the client)")).toBeTruthy();
    expect(screen.queryByRole("radiogroup", { name: "Visit time" })).toBeNull();
    expect(screen.queryByText(/Delegation period:/u)).toBeNull();
    save("Confirm assignment");
    expect(await screen.findByText(/^Assigned technician for 21 Sep 10:00–12:00\./u)).toBeTruthy();
    expect(actions.assignTechnician).toHaveBeenLastCalledWith("job-1", 3, "m-2", slot, "");
  });

  it("shows each refusal in its tone, refuses saving without eligible technicians, and asks for a technician that came back", async () => {
    actions.assignTechnician.mockResolvedValueOnce({ ok: false, code: "CONFLICT", messageKey: "errors.assignment_overlap", fieldErrors: {}, retryAfterSeconds: null });
    actions.assignTechnician.mockResolvedValueOnce({ ok: false, code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {}, retryAfterSeconds: null });
    render(<ScheduleView live={live({ otherZone: true })} />);
    expect(screen.getByText(/Free hours are in Kuala Lumpur time/u)).toBeTruthy();
    save("Confirm assignment");
    const overlap = await screen.findByText("Overlaps a confirmed schedule for Aina — choose another technician.");
    expect(overlap.closest("[role='status']")).toBeTruthy(); // a conflict is a warning, not an alert
    save("Confirm assignment");
    const forbidden = assignRefusal({ code: "FORBIDDEN", messageKey: "error.forbidden", fieldErrors: {}, retryAfterSeconds: null }, "Aina");
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(forbidden);
    expect(screen.queryByText("Overlaps a confirmed schedule for Aina — choose another technician.")).toBeNull();
    // nobody can take the time: the button is off and the reason shows
    cleanup();
    const busy = selected({ id: "job-2", candidates: [cand("m-1", "Aina", false, "Busy at this time")] });
    const { rerender } = render(<ScheduleView live={live({ selected: busy })} />);
    expect(screen.getByText(/No technician of your company can take this time/u)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Confirm assignment" })).toHaveProperty("disabled", true);
    // after a re-read Aina is free again, but no technician is chosen yet
    rerender(<ScheduleView live={live({ selected: { ...busy, candidates: [cand("m-1", "Aina")] } })} />);
    save("Confirm assignment");
    expect(screen.getByText("Choose a technician")).toBeTruthy();
    expect(actions.assignTechnician).toHaveBeenCalledTimes(2);
    // Aina chosen, then booked elsewhere by the next re-read: the choice stays and the refusal shows
    fireEvent.click(screen.getByRole("radio", { name: /Aina/u }));
    rerender(<ScheduleView live={live({ selected: busy })} />);
    expect(screen.getByText("Aina: Busy at this time — saving is refused.")).toBeTruthy();
    cleanup();
    render(<ScheduleView live={live({ selected: selected({ candidates: [] }), week: week({ capacity: [] }), members: [] })} />);
    expect(screen.getByText("No technicians in your company yet — add them in Team & capacity.")).toBeTruthy();
    expect(screen.getByText("No technicians in your company yet.")).toBeTruthy();
  });
});

const reassign: FormMode = { ...assignMode, kind: "reassign", title: "Reassign — job-1", button: "Confirm reassignment", currentTech: "Aina", banner: { tone: "warn", text: "Aina can’t make this time." } };
const extend: FormMode = { ...assignMode, kind: "extend", title: "Reassign — job-1 (in progress)", endEditable: true, reasonRequired: true, button: "Confirm reassignment", currentTech: "Aina", choices: [] };

describe("ScheduleView reassign and extend", () => {
  it("reassigns: names the current technician, refuses the same technician at the same time, and keeps the acknowledgement", async () => {
    actions.assignTechnician.mockResolvedValue({ ok: true, value: { slot: "21 Sep 10:00–12:00" } });
    const sel = selected({ mode: reassign, currentId: "m-1", candidates: [cand("m-1", "Aina", true, "Current")], ack: { status: "accepted", reason: null, alternative: null }, text: { ...selected().text, alternative: "22 Sep 14:00–16:00" } });
    render(<ScheduleView live={live({ selected: sel })} />);
    expect(screen.getByText("New technician (current: Aina)")).toBeTruthy();
    expect(banner("Aina can’t make this time.")).toBeTruthy();
    expect(banner("The current technician accepted the assignment")).toBeTruthy();
    expect(screen.getByText(/The technician could do 22 Sep 14:00–16:00 instead/u)).toBeTruthy();
    expect(document.querySelector("textarea")?.getAttribute("placeholder")).toBe("");
    save("Confirm reassignment");
    expect(screen.getByText("Choose another technician or another time")).toBeTruthy();
    expect(document.querySelector(".border-2")).toBeNull(); // the booking as it is: no proposal in the week
    expect(actions.assignTechnician).not.toHaveBeenCalled();
    cleanup();
    // another agreed time with the same technician is a change, and the week shows it as the proposal
    render(<ScheduleView live={live({ selected: { ...sel, moved: true, mode: { ...reassign, slot: other }, ack: { status: "pending", reason: null, alternative: null } } })} />);
    expect(banner("Waiting for the current technician to accept the assignment.")).toBeTruthy();
    const aina = screen.getByRole("row", { name: /Aina/u });
    expect(within(aina).getByText("10:00–12:00").parentElement?.className).toContain("border-2");
    save("Confirm reassignment");
    expect(await screen.findByText(/^Changed to Aina for 21 Sep 10:00–12:00/u)).toBeTruthy();
    expect(actions.assignTechnician).toHaveBeenCalledExactlyOnceWith("job-1", 3, "m-1", other, "");
  });

  it("extends work under way: keeps the start, needs a later end and a reason, and warns when the technician changes", async () => {
    actions.assignTechnician.mockResolvedValue({ ok: true, value: { slot: "21 Sep 10:00–14:00" } });
    const sel = selected({ mode: extend, currentId: "m-1", candidates: [cand("m-1", "Aina", true, "Current"), cand("m-2", "Badrul")], ack: { status: "cant_make", reason: "sick", alternative: null } });
    render(<ScheduleView live={live({ selected: sel })} />);
    // the current technician is the default only when nobody else is eligible: Badrul is chosen first
    expect(banner(/On save, Aina loses action access immediately/u)).toBeTruthy();
    fireEvent.click(screen.getByRole("radio", { name: /Aina/u }));
    expect(screen.getByRole("button", { name: "Save new slot" })).toBeTruthy();
    expect(screen.queryByText(/accept the assignment/u)).toBeNull(); // no acknowledgement banner while extending
    save("Save new slot");
    expect(screen.getByText("A reason is required while the work is under way")).toBeTruthy();
    expect(screen.getByText("Choose another technician or a later end")).toBeTruthy();
    fireEvent.change(screen.getByPlaceholderText("tech-external-a is on sick leave from today."), { target: { value: "Parts arrive late" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Work end" }), { target: { value: "2026-09-21T11:00" } });
    save("Save new slot");
    expect(screen.getByText("The end must be at or after the current end")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Work end" }), { target: { value: "" } });
    save("Save new slot");
    expect(actions.assignTechnician).not.toHaveBeenCalled();
    fireEvent.change(screen.getByRole("textbox", { name: "Work end" }), { target: { value: "2026-09-21T14:00" } });
    // the week shows the longer block for Aina
    expect(within(screen.getByRole("row", { name: /Aina/u })).getByText("10:00–14:00")).toBeTruthy();
    save("Save new slot");
    expect(await screen.findByText(/^Changed to Aina for 21 Sep 10:00–14:00/u)).toBeTruthy();
    expect(actions.assignTechnician).toHaveBeenCalledExactlyOnceWith("job-1", 3, "m-1", { startAt: slot.startAt, endAt: "2026-09-21T06:00:00.000Z" }, "Parts arrive late");
    cleanup();
    // the current technician is no longer listed: the form still says whom the change replaces
    render(<ScheduleView live={live({ selected: selected({ mode: { ...extend, banner: { tone: "muted", text: "Work paused" } }, currentId: "m-9", candidates: [cand("m-2", "Badrul")] }) })} />);
    expect(screen.getByText("New technician (current: —)")).toBeTruthy();
    expect(banner(/^On save, the current technician loses action access immediately/u)).toBeTruthy();
    expect(banner("Work paused").closest("[role='status']")).toBeTruthy();
  });
});

describe("ScheduleView blocked", () => {
  it("shows why the job cannot be booked, the window and the links, with no proposal in the week", () => {
    const blocked: FormMode = { slot, kind: "blocked", title: "Assign — job-1", banner: { tone: "muted", text: "The job is completed — nothing to schedule." } };
    render(<ScheduleView live={live({ selected: selected({ mode: blocked, times: [], candidates: [] }) })} />);
    expect(banner("The job is completed — nothing to schedule.")).toBeTruthy();
    expect(banner("Delegation period: 14 Sep – 25 Sep — work must fit within this window.")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Open the job/u }).getAttribute("href")).toBe("/partner/jobs/job-1");
    expect(screen.getByRole("link", { name: /Job history/u }).getAttribute("href")).toBe("/partner/history?jobId=job-1");
    expect(screen.queryByRole("button")).toBeNull();
    expect(document.querySelector(".border-2")).toBeNull();
    // the week marks today's column
    expect(screen.getByRole("columnheader", { name: "Day 22" }).className).toContain("text-primary");
    expect(screen.getByRole("columnheader", { name: "Day 21" }).className).toContain("text-muted-foreground");
    cleanup();
    // a passed time is an alert, and a job with no window says nothing about one
    render(<ScheduleView live={live({ selected: selected({ mode: { ...blocked, banner: { tone: "crit", text: "Every time the client agreed has passed." } }, text: { ...selected().text, window: null } }) })} />);
    expect(screen.getByRole("alert").textContent).toBe("Every time the client agreed has passed.");
    expect(screen.queryByText(/Delegation period:/u)).toBeNull();
  });
});
