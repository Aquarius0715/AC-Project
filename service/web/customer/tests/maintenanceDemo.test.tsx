// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";

// The Phase 1A demo of /customer/maintenance (fixture store): the status tabs and Origin filter, a request's detail with
// its note and cancel, New request with its checks, a proposed time to accept or decline, another plan time, Confirm &
// rate and Report a problem on the finished plan visit, and the Filter care fixtures (Mark cleaned, Request cleaning).
const toast = vi.hoisted(() => vi.fn<(text: string, tone?: string) => void>());
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { MaintenanceDemo } from "../app/customer/maintenance/_components/maintenance-demo";
import { jobActions } from "@ac/web/lib/jobs";

const button = (name: string | RegExp) => screen.getByRole("button", { name });
const dialog = () => screen.getByRole("dialog");
/** a lookup that must have found its element: the test stops at the step that missed it */
const missing = (): never => { throw new Error("element not found"); };
const c07Row = () => screen.getByRole("button", { name: /job-c07/ });

beforeEach(() => {
  toast.mockReset();
  localStorage.clear();
  jobActions.reset();
});
afterEach(() => { cleanup(); });

describe("MaintenanceDemo", () => {
  it("filters by status and origin, and a requested job takes a note and is cancelled with a reason", () => {
    render(<MaintenanceDemo />);
    expect(screen.getByText(/requests · sorted by status/)).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: /Cancelled/ }));
    expect(screen.getAllByRole("button", { name: /job-c/ }).map((b) => b.textContent.slice(0, 7))).toStrictEqual(["job-c01"]);
    fireEvent.change(screen.getByRole("combobox", { name: "Origin" }), { target: { value: "plan" } });
    expect(screen.getByText("No requests in this filter.")).toBeDefined();
    fireEvent.change(screen.getByRole("combobox", { name: "Origin" }), { target: { value: "all" } });
    fireEvent.click(screen.getByRole("tab", { name: /^All/ }));
    fireEvent.click(c07Row());
    expect(screen.getByText("Your preferred times")).toBeDefined();
    fireEvent.click(button(/Add note/));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send note" }));
    expect(within(dialog()).getByText("Write 1–2000 characters")).toBeDefined();
    fireEvent.change(within(dialog()).getByRole("textbox"), { target: { value: "Gate code 4821" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send note" }));
    expect(toast).toHaveBeenLastCalledWith("Note sent — HQ, the service partner and the technician can read it");
    expect(screen.getByText("Notes to coordinator").nextElementSibling?.textContent).toMatch(/^You · .+ — “Gate code 4821”$/);
    fireEvent.click(button("Cancel request"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel request" }));
    expect(within(dialog()).getByText("A reason is required")).toBeDefined();
    fireEvent.change(within(dialog()).getByRole("textbox"), { target: { value: "Resolved itself" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel request" }));
    expect(toast).toHaveBeenLastCalledWith("Request cancelled", "warn");
    expect(within(c07Row()).getAllByText("Cancelled")).toHaveLength(2); // the line and the status badge
    fireEvent.click(button("Close"));
    expect(screen.queryByText("Notes to coordinator")).toBeNull();
  });

  it("New request checks the symptoms, then records the request and opens it", () => {
    render(<MaintenanceDemo />);
    fireEvent.click(button("New maintenance request"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Submit request" }));
    expect(within(dialog()).getByText("Symptoms must be 10–2000 characters")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Preventive" }));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Symptoms/ }), { target: { value: "Weak cold air since this morning" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Submit request" }));
    const [text] = toast.mock.lastCall ?? [];
    expect(text).toMatch(/^Request job-\S+ sent with 3 preferred times$/);
    expect(screen.getByText("“Weak cold air since this morning”")).toBeDefined();
  });

  it("a proposed time needs a reply: Review opens it, Accept books it, Decline sends new times or none", () => {
    act(() => { jobActions.propose("job-c07", { date: "2026-09-30", win: "14:00–16:00" }, "contractor-a tech", "Earlier slot", "Sep 26"); });
    render(<MaintenanceDemo />);
    expect(screen.getByText(/A new visit time needs your reply \(1\)/)).toBeDefined();
    fireEvent.click(button("Review"));
    expect(screen.getByText("Reply by Sep 26")).toBeDefined();
    fireEvent.click(button("Decline…"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Too late for me" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Decline & send new times" }));
    expect(toast).toHaveBeenLastCalledWith("Declined — HQ will look again", "warn");
    expect(screen.getByText(/You declined the proposed time \(Too late for me\)/)).toBeDefined();
    act(() => { jobActions.propose("job-c07", { date: "2026-09-30", win: "14:00–16:00" }, "hq", "Again", "Sep 27"); });
    fireEvent.click(button("Decline…"));
    fireEvent.click(within(dialog()).getByRole("checkbox"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Decline" }));
    expect(toast).toHaveBeenLastCalledWith("Declined — HQ will look again", "warn");
    act(() => { jobActions.propose("job-c07", { date: "2026-09-30", win: "14:00–16:00" }, "hq", "Again", "Sep 28"); });
    fireEvent.click(button("Accept this time"));
    expect(toast).toHaveBeenLastCalledWith("Time accepted — booking the technician");
    expect(screen.queryByRole("button", { name: "Accept this time" })).toBeNull();
  });

  it("a plan visit asks HQ for another time; the finished visit is rated or reported once", () => {
    render(<MaintenanceDemo jobId="job-c08" />);
    fireEvent.click(button("Request another time…"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send to HQ" }));
    expect(toast).toHaveBeenLastCalledWith("Asked HQ for another time");
    // the banner's Confirm & rate opens the finished visit's rating; Not now leaves it open to rate
    fireEvent.click(button("Confirm & rate"));
    expect(within(dialog()).getByText("Confirm & rate job-c02")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Not now" }));
    expect(screen.getByText("Completed report")).toBeDefined();
    fireEvent.click(within(screen.getByText("Completed report").closest("div") ?? missing()).getAllByRole("button", { name: "Confirm & rate" }).at(-1) ?? missing());
    fireEvent.click(within(dialog()).getByRole("button", { name: "1 stars" }));
    expect(within(dialog()).getByText(/What went wrong\?/)).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Polite" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "On time" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Confirm & submit" }));
    expect(toast).toHaveBeenLastCalledWith("Thanks — job confirmed and rated");
    expect(screen.queryByText(/Work finished — job-c02/)).toBeNull();
  });

  it("dialogs close without changes, and bad times or a changed job are refused in the dialog", () => {
    act(() => { jobActions.propose("job-c07", { date: "2026-09-30", win: "14:00–16:00" }, "hq", "Again", "Sep 27"); });
    render(<MaintenanceDemo jobId="job-c07" />);
    const closeBy = (open: string | RegExp, close: string) => {
      fireEvent.click(button(open));
      fireEvent.click(within(dialog()).getByRole("button", { name: close }));
      expect(screen.queryByRole("dialog")).toBeNull();
    };
    closeBy("Decline…", "Back");
    closeBy(/Add note/, "Cancel");
    closeBy("Cancel request", "Keep request");
    fireEvent.click(button("Decline…"));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Comment to HQ/ }), { target: { value: "Away" } });
    const [firstDate] = within(dialog()).getAllByLabelText(/date/i);
    fireEvent.change(firstDate ?? missing(), { target: { value: "2026-09-20" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Decline & send new times" }));
    expect(within(dialog()).getByText("Each time must be at least 1 day ahead")).toBeDefined();
    expect(toast).not.toHaveBeenCalled();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Back" }));
    // the job is cancelled elsewhere while the note is open: the note is refused in the dialog
    fireEvent.click(button(/Add note/));
    act(() => { jobActions.cancel("job-c07", "elsewhere"); });
    fireEvent.change(within(dialog()).getByRole("textbox"), { target: { value: "Gate code 4821" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send note" }));
    expect(within(dialog()).getByText("CONFLICT — notes are only possible on open jobs")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("New maintenance request"));
    fireEvent.change(within(dialog()).getByRole("combobox", { name: /Unit/ }), { target: { value: "unit-kitchen-a" } });
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Symptoms/ }), { target: { value: "Kitchen AC drips water" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Submit request" }));
    expect(screen.getByText("“Kitchen AC drips water”")).toBeDefined();
    expect(screen.getByText(/^Kitchen AC · /)).toBeDefined();
  });

  it("a plan visit's other time checks the times, and Cancel keeps the visit", () => {
    render(<MaintenanceDemo jobId="job-c08" />);
    fireEvent.click(button("Request another time…"));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    fireEvent.click(button("Request another time…"));
    fireEvent.change(within(dialog()).getByRole("textbox", { name: /Comment/ }), { target: { value: "Away" } });
    const [, second] = within(dialog()).getAllByLabelText(/date/i);
    fireEvent.change(second ?? missing(), { target: { value: "" } });
    fireEvent.click(within(dialog()).getByRole("button", { name: "Send to HQ" }));
    expect(within(dialog()).getByText("Enter 3 preferred times")).toBeDefined();
    expect(toast).not.toHaveBeenCalled();
  });

  it("a booked partner, its technician, a partner's proposed time and the follow-up classes", () => {
    act(() => { jobActions.book("job-c07", { date: "2026-09-28", win: "14:00–16:00" }, "contractor", "contractor-a"); });
    const { unmount } = render(<MaintenanceDemo jobId="job-c07" />);
    expect(within(c07Row()).getByText(/· booking the technician$/)).toBeDefined();
    expect(screen.getByText("booked")).toBeDefined();
    act(() => { jobActions.partnerAssign("job-c07", "tech-partner-a"); jobActions.techAccept("job-c07"); });
    expect(screen.getByText("tech-partner-a (contractor-a)")).toBeDefined();
    fireEvent.click(button(/Add note/));
    expect(within(dialog()).getByText(/Visible to: HQ coordinator · contractor-a · assigned technician \(tech-partner-a\)/)).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    unmount();
    act(() => {
      jobActions.reset();
      jobActions.book("job-c07", { date: "2026-09-28", win: "14:00–16:00" }, "contractor", "contractor-a");
      jobActions.partnerPropose("job-c07", { date: "2026-10-01", win: "09:00–11:00" }, "tech-partner-a", "Earlier");
      jobActions.forwardPartner("job-c07", "Sep 27");
    });
    render(<MaintenanceDemo jobId="job-c07" />);
    expect(screen.getByText("Your service partner asks for another time (via HQ)")).toBeDefined();
    expect(within(c07Row()).getByText(/^Service partner asks for /)).toBeDefined();
    act(() => {
      const made = jobActions.reportProblem("job-c02", "Other", "Water on the floor", "").id;
      const second = jobActions.reportProblem("job-c02", "Other", "Noise again", "").id;
      if (made === undefined || second === undefined) throw new Error("job-c02 is completed in the seed");
      jobActions.classifyFollowUp(made, "rework", "same fault");
      jobActions.classifyFollowUp(second, "new_request", "new fault");
    });
    expect(screen.getAllByText("Rework (free)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("New request").length).toBeGreaterThan(0);
  });

  it("Filter care marks a filter cleaned and sends an overdue one to New request", () => {
    render(<MaintenanceDemo initialTab="filter-care" />);
    fireEvent.click(within(screen.getByText("Bedroom AC").closest("tr") ?? missing()).getByRole("button", { name: "Mark cleaned" }));
    expect(toast).toHaveBeenLastCalledWith("Marked cleaned — counter reset");
    expect(screen.getByText("Home A › 1F · cleaned by you just now")).toBeDefined();
    fireEvent.click(button("Edit reminders"));
    expect(toast).toHaveBeenLastCalledWith("Reminder settings saved");
    fireEvent.click(button("Request cleaning"));
    expect(within(dialog()).getByText("New maintenance request")).toBeDefined();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
    expect(screen.getByText(/requests · sorted by status/)).toBeDefined();
  });
});
