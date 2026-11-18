// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Job } from "@ac/web/lib/jobs";

// The Phase 1A schedule demo: contractor-a's accepted and assigned store jobs before the fixed jobs, the assign form's
// refusals (reason, start before end, each job's own delegation window, the 09:00 overlap) and its receipt, and the
// store job's locked visit time with the technician's answer
const store = vi.hoisted((): { jobs: Job[] } => ({ jobs: [] }));
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/jobs", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useJobs: () => store.jobs }));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { getJobState, jobActions } from "@ac/web/lib/jobs";
import { ScheduleDemo } from "../app/partner/schedule/_components/schedule-demo";

const base = () => {
  const job = getJobState().jobs.find((j) => j.id === "job-a11");
  if (!job) throw new Error("fixture without job-a11");
  return job;
};
const job = (over: Partial<Job>): Job => ({ ...base(), contractor: "contractor-a", status: "assigned", technician: "tech-external-a", techAck: null, scheduled: { date: "2026-09-26", win: "09:00–12:00" }, ...over });

beforeEach(() => { store.jobs = []; toast.mockClear(); });
afterEach(() => { cleanup(); vi.restoreAllMocks(); jobActions.reset(); }); // partnerAssign runs for real and writes the demo store

const rowTitles = () => [...document.querySelectorAll("button b, [role=option] b, li b")].map((b) => b.textContent);
const pick = (id: string) => { fireEvent.click(screen.getByText(id, { exact: false, selector: "b.text-\\[13px\\]" })); };
const save = (name = "Assign technician") => { fireEvent.click(screen.getByRole("button", { name })); };
/** The banners (a refusal is an alert, the others a status). */
const banners = () => [...document.querySelectorAll("[role=alert], [role=status]")];
const alertText = () => banners().map((a) => a.textContent).join(" | ");
/** The control with the label in the work start (0) or the work end (1) picker. */
const control = (label: string, i: number) => {
  const el = screen.getAllByLabelText(label).at(i);
  if (!el) throw new Error(`no ${label} control ${String(i)}`);
  return el;
};
/** The classes of the first team-week cell with the text. */
const cell = (text: string) => screen.getAllByText(text, { selector: "td div" }).at(0)?.className ?? "";
const setDate = (i: number, v: string) => { fireEvent.change(control("Date", i), { target: { value: v } }); };
const setStartHour = (v: string) => { fireEvent.change(control("Hour", 0), { target: { value: v } }); };

describe("Schedule demo", () => {
  it("lists contractor-a's accepted and assigned store jobs first, skipping other companies, other states and the fixed ids", () => {
    store.jobs = [
      job({ id: "job-x1", status: "accepted", technician: null, origin: "plan" }),
      job({ id: "job-x2", techAck: null, scheduled: null }),
      job({ id: "job-x3", contractor: "contractor-b" }),
      job({ id: "job-x4", status: "in_progress" }),
      job({ id: "job-p02", status: "accepted" }),
    ];
    render(<ScheduleDemo />);
    expect(rowTitles()).toStrictEqual(["job-x1 · Rooftop unit", "job-x2 · Rooftop unit", "job-p02 · Rooftop unit", "job-p12 · Lobby AC", "job-contractor-a · Bedroom AC", "job-p07 · Server room AC"]);
    expect(screen.getAllByText("Accepted · awaiting assignment")).toHaveLength(3);
    expect(screen.getByText("Assigned · tech-external-a · awaiting acceptance")).toBeTruthy();
    expect(screen.getByText("09-26 00:00 → 09-26 23:59")).toBeTruthy();
    expect(screen.getByText("— 00:00 → — 23:59")).toBeTruthy();
    // only the store jobs carry their origin; the fixed jobs have none
    expect(screen.getByText("Periodic plan")).toBeTruthy();
    expect(screen.getAllByText("Client request")).toHaveLength(1);
    // nothing picked: the first fixed job's form; the ended window is red, the others muted
    expect(screen.getByText("Assign — job-p02 · Rooftop unit")).toBeTruthy();
    expect(screen.getByText("Ended").className).toContain("crit");
    expect(screen.getByText("14 h left").className).not.toContain("crit");
  });

  it("assigns a fixed job inside its window and marks its row; an unknown jobId falls back to the first fixed job", () => {
    render(<ScheduleDemo jobId="job-gone" />);
    expect(screen.getByText("Assign — job-p02 · Rooftop unit")).toBeTruthy();
    expect(screen.getByText("Delegation period: 09-14 01:00 – 09-22 00:00 — work must fit within this window.")).toBeTruthy();
    save();
    expect(alertText()).toBe("Assigned tech-external-a to job-p02. Notifications sent (preview).");
    expect(toast).toHaveBeenCalledWith("Technician assigned");
    expect(screen.getByText("Assigned · awaiting technician acceptance")).toBeTruthy();
  });

  it("refuses a start at or after the end, and an end past the job's own window", () => {
    render(<ScheduleDemo jobId="job-p02" />);
    setStartHour("12");
    save();
    expect(alertText()).toBe("Start must be before end.");
    setStartHour("10");
    setDate(1, "2026-09-22");
    save();
    expect(alertText()).toBe("Slot is outside the delegation period (VALIDATION).");
    expect(toast).not.toHaveBeenCalled();
  });

  it("checks job-contractor-a's end against its own window (09-23 00:00) and needs a reason to reassign it", () => {
    render(<ScheduleDemo jobId="job-contractor-a" />);
    save("Reassign");
    expect(alertText()).toBe("Reason required when reassigning a job already in progress (VALIDATION).");
    fireEvent.change(screen.getByLabelText(/^Reassign reason/), { target: { value: "Sick" } });
    setDate(0, "2026-09-23");
    setDate(1, "2026-09-23");
    save("Reassign");
    expect(alertText()).toBe("Slot is outside the delegation period (VALIDATION).");
    setDate(0, "2026-09-22");
    setDate(1, "2026-09-22");
    save("Reassign");
    expect(alertText()).toBe("Assigned tech-external-a to job-contractor-a. Notifications sent (preview).");
  });

  it("lets a job whose work window ended be reassigned past it, which extends the window", () => {
    render(<ScheduleDemo jobId="job-p07" />);
    fireEvent.change(screen.getByLabelText(/^Reassign reason/), { target: { value: " Extend " } });
    setDate(1, "2026-09-30");
    save("Reassign");
    expect(alertText()).toBe("Assigned tech-external-a to job-p07. Notifications sent (preview).");
  });

  it("shows the 09:00 overlap for tech-external-a only; picking another row clears the message", () => {
    render(<ScheduleDemo jobId="job-p12" />);
    setStartHour("09");
    save();
    expect(alertText()).toBe("Overlaps a confirmed schedule for tech-external-a (CONFLICT).");
    fireEvent.change(screen.getByLabelText(/^Technician/), { target: { value: "tech-external-a2" } });
    expect(screen.getByText("No overlapping confirmed schedule for tech-external-a2 in this window.")).toBeTruthy();
    save();
    expect(alertText()).toBe("Assigned tech-external-a2 to job-p12. Notifications sent (preview).");
    pick("job-p02");
    expect(banners()).toHaveLength(0);
    expect(screen.getByText("Assign — job-p02 · Rooftop unit")).toBeTruthy();
  });

  it("locks a store job's agreed time, shows the technician's answer and assigns through the job store", () => {
    const assign = vi.spyOn(jobActions, "partnerAssign");
    store.jobs = [
      job({ id: "job-x1", techAck: { status: "accepted", at: "09-21 08:30" } }),
      job({ id: "job-x2", techAck: { status: "cant_make", reason: "Away", alt: "09-27 morning" } }),
      job({ id: "job-x3", techAck: { status: "cant_make" }, technician: null }),
      job({ id: "job-x4", techAck: { status: "pending" }, scheduled: null }),
      job({ id: "job-x5", status: "accepted", technician: null }),
    ];
    render(<ScheduleDemo jobId="job-x1" />);
    expect(screen.queryByLabelText("Hour")).toBeNull();
    expect(screen.getByText("tech-external-a accepted the assignment (09-21 08:30)")).toBeTruthy();
    expect(screen.getByText("Assigned · tech-external-a · accepted")).toBeTruthy();
    save("Reassign");
    expect(assign).toHaveBeenCalledWith("job-x1", "tech-external-a");
    expect(alertText()).toContain("Assigned tech-external-a to job-x1 for ");
    expect(toast).toHaveBeenCalledWith("Technician assigned — waiting for acceptance");

    pick("job-x2");
    expect(alertText()).toBe("tech-external-a can’t make this time: “Away” · could do 09-27 morning. Reassign another technician below, or ask HQ to propose a new time to the client.");
    expect(screen.getByText("Assigned · tech-external-a · can’t make it")).toBeTruthy();
    pick("job-x3");
    expect(alertText()).toBe("— can’t make this time: “—”. Reassign another technician below, or ask HQ to propose a new time to the client.");
    pick("job-x4");
    expect(alertText()).toBe("Waiting for tech-external-a to accept the assignment.");
    expect(screen.getByText("—", { selector: "div.font-semibold" })).toBeTruthy();
    // an accepted store job has no answer yet and keeps the first-assignment wording
    pick("job-x5");
    expect(banners()).toHaveLength(0);
    expect(screen.getByRole("button", { name: "Assign technician" })).toBeTruthy();
  });

  it("colours the team week: booked, free, leave and no-hours cells", () => {
    render(<ScheduleDemo />);
    expect(cell("10–12 job-p02")).toContain("bg-primary-soft");
    expect(cell("free")).toContain("bg-ok-soft");
    expect(cell("no hours")).toContain("bg-surface2");
    expect(cell("09–17 Leave")).toContain("bg-warn-soft");
  });
});
