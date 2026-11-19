// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { Job } from "@ac/web/lib/jobs";

// the job workspace's Phase 1A demo (review round 1080): the shared store's live assignment (accept or can't make) and
// the fixture workspace (checklist, submit rules, tabs, readings, parts, sign-off and the demo status switches)
const store = vi.hoisted((): { jobs: Job[]; tab: string | null } => ({ jobs: [], tab: null }));
const toast = vi.hoisted(() => vi.fn<(notice: string, tone?: string) => void>());
const techAccept = vi.hoisted(() => vi.fn<(id: string) => void>());
const techCantMake = vi.hoisted(() => vi.fn<(id: string, reason: string, alt: string) => void>());
vi.mock("next/navigation", () => ({ useSearchParams: () => new URLSearchParams(store.tab === null ? "" : `tab=${store.tab}`) }));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));
vi.mock(import("@ac/web/lib/jobs"), async (importOriginal) => {
  const real = await importOriginal();
  return { ...real, useJobs: () => store.jobs, jobActions: { ...real.jobActions, techAccept, techCantMake } };
});

import { getJobState } from "@ac/web/lib/jobs";
import { WorkspaceDemo } from "../app/technician/jobs/[id]/_components/workspace-demo";

// a new contractor assignment of tech-external-a (`over` for HQ delivery, the other answers, no time)
function assignment(over: Partial<Job> = {}): Job {
  const seed = getJobState().jobs.find((j) => j.id === "job-p09");
  if (!seed) throw new Error("seed job-p09 is missing");
  return { ...seed, id: "job-live", status: "assigned", techAck: { status: "pending" }, ...over };
}
const button = (name: string | RegExp, root: HTMLElement = document.body) => within(root).getByRole<HTMLButtonElement>("button", { name });
const dialog = () => screen.getByRole("dialog");
const errors = () => within(screen.getByText("Can’t submit:").closest("div") ?? document.body).getAllByRole("listitem").map((li) => li.textContent);
// every checklist item of the open tab answered Normal
const answerAllNormal = () => {
  const normals = screen.getAllByRole("button", { name: "Normal" });
  for (const b of normals) fireEvent.click(b);
};

beforeEach(() => {
  store.jobs = []; store.tab = null;
  toast.mockClear(); techAccept.mockClear(); techCantMake.mockClear();
  history.replaceState(null, "", "/technician/jobs/job-x");
});
afterEach(() => { cleanup(); });

describe("WorkspaceDemo live assignment", () => {
  it("shows the store's assignment of tech-external-a, but the fixture workspace for another technician or a fixture job", () => {
    const fixtures = ["job-contractor-a", "job-t07", "job-p05", "job-p09"];
    store.jobs = [assignment(), assignment({ id: "job-other", technician: "tech-external-a2" }), ...fixtures.map((id) => assignment({ id }))];
    const { unmount } = render(<WorkspaceDemo id="job-live" />);
    expect(screen.getByText("New assignment — please accept")).toBeDefined();
    unmount();
    for (const id of ["job-other", ...fixtures]) {
      const view = render(<WorkspaceDemo id={id} />);
      expect(screen.queryByText("New assignment — please accept")).toBeNull();
      expect(screen.getByText("Inspection checklist")).toBeDefined();
      view.unmount();
    }
  });

  it("accepts the assignment through the store", () => {
    store.jobs = [assignment()];
    render(<WorkspaceDemo id="job-live" />);
    expect(screen.getByText(/new assignment$/)).toBeDefined();
    fireEvent.click(button(/Accept assignment/));
    expect(techAccept).toHaveBeenCalledWith("job-live");
    expect(toast).toHaveBeenCalledWith("Assignment accepted");
  });

  it("needs a comment before it sends can't make to the contractor, with the reason and the other time", () => {
    store.jobs = [assignment()];
    render(<WorkspaceDemo id="job-live" />);
    fireEvent.click(button("Can’t make this time…"));
    fireEvent.click(button("Send to contractor-a", dialog()));
    expect(within(dialog()).getByText("Tell your coordinator why")).toBeDefined();
    expect(techCantMake).not.toHaveBeenCalled();
    fireEvent.click(within(dialog()).getByRole("button", { name: "Sick" }));
    fireEvent.change(within(dialog()).getByPlaceholderText("Medical appointment on 10-01 morning."), { target: { value: "  Clinic visit " } });
    fireEvent.change(within(dialog()).getByRole("combobox", { name: /A time you could do instead/ }), { target: { value: "Mon 10-05 10:00–12:00" } });
    fireEvent.click(button("Send to contractor-a", dialog()));
    expect(techCantMake).toHaveBeenCalledWith("job-live", "Sick · Clinic visit", "Mon 10-05 10:00–12:00");
    expect(toast).toHaveBeenCalledWith("Sent to contractor-a", "warn");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("shows an accepted job's locked Start, and HQ as the coordinator of a can't-make answer without a time", () => {
    store.jobs = [assignment({ techAck: { status: "accepted" } }), assignment({ id: "job-hq", delivery: "internal", contractor: null, scheduled: null, origin: "request", techAck: { status: "cant_make", reason: "Sick · Clinic" } })];
    const { unmount } = render(<WorkspaceDemo id="job-live" />);
    expect(screen.getByText(/You accepted this assignment/).textContent).toContain("on 09-16");
    expect(button("Start job (opens check-in)").disabled).toBe(true);
    expect(screen.queryByRole("button", { name: /Accept assignment/ })).toBeNull();
    unmount();
    render(<WorkspaceDemo id="job-hq" />);
    expect(screen.getByText(/You told HQ you can’t make it/).textContent).toContain("“Sick · Clinic”");
    expect(screen.getByText("Job — assigned to you")).toBeDefined();
    expect(screen.getByText("the client (via HQ)")).toBeDefined();
    const visit = screen.getByText("Visit time").parentElement;
    expect(visit?.textContent).toBe("Visit time—"); // no visit time
  });

  it("reads a job without an answer as new, and names HQ everywhere when a contractor job has no contractor yet", () => {
    store.jobs = [assignment({ contractor: null, techAck: null })];
    render(<WorkspaceDemo id="job-live" />);
    expect(screen.getByText(/new assignment$/)).toBeDefined();
    expect(screen.getByText("Assigned by").parentElement?.textContent).toBe("Assigned byHQ");
    expect(screen.getByText(/^Accepting tells HQ that you will attend/)).toBeDefined(); // not "HQ and HQ"
    fireEvent.click(button("Can’t make this time…"));
    fireEvent.change(within(dialog()).getByPlaceholderText("Medical appointment on 10-01 morning."), { target: { value: "Clinic" } });
    fireEvent.click(button("Send to HQ", dialog()));
    expect(toast).toHaveBeenCalledWith("Sent to HQ", "warn");
  });
});

describe("WorkspaceDemo fixture workspace", () => {
  it("is not available for another technician's job", () => {
    render(<WorkspaceDemo id="job-internal-a" />);
    expect(screen.getByText("This page isn’t available")).toBeDefined();
    expect(screen.queryByText("Inspection checklist")).toBeNull();
  });

  it("refuses to submit with empty items, a short report or a missing reason, and submits once all are fixed", () => {
    render(<WorkspaceDemo id="job-x" />);
    expect(screen.getByText("7 / 18")).toBeDefined();
    fireEvent.change(screen.getByRole("textbox", { name: /^Work performed/ }), { target: { value: "short" } });
    fireEvent.click(within(screen.getAllByRole("listitem").find((li) => li.textContent.startsWith("Louver")) ?? document.body).getByRole("button", { name: "Not inspected" }));
    fireEvent.click(button("Submit report"));
    expect(errors()).toStrictEqual([
      "10 items still null — each needs a result, or not_inspected / not_applicable with a reason.",
      "Work performed must be 10–4000 characters.",
      "Louver: reason required.",
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.change(screen.getAllByPlaceholderText("Reason…").at(-1) ?? document.body, { target: { value: "behind a cabinet" } });
    fireEvent.click(screen.getByRole("tab", { name: "Outdoor (5)" }));
    answerAllNormal();
    fireEvent.click(screen.getByRole("tab", { name: "Electrical (5)" }));
    answerAllNormal();
    fireEvent.change(screen.getByRole("textbox", { name: /^Work performed/ }), { target: { value: "Cleaned everything" } });
    expect(screen.getByText("18 / 18")).toBeDefined();
    fireEvent.click(button("Submit report"));
    expect(screen.queryByText("Can’t submit:")).toBeNull();
    expect(within(dialog()).getByText(/No customer sign-off yet/)).toBeDefined();
    fireEvent.click(button("Submit", dialog()));
    expect(toast).toHaveBeenCalledWith("Report v2 submitted");
    expect(screen.getByText("Submitted v1 — read-only, awaiting quality review.")).toBeDefined();
    expect(screen.queryByRole("button", { name: "Submit report" })).toBeNull();
    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: /^Work performed/ }).disabled).toBe(true);
  });

  it("starts on the URL's tab and keeps the chosen tab in the URL", () => {
    store.tab = "electrical";
    render(<WorkspaceDemo id="job-x" />);
    expect(screen.getByText("Capacitor")).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: "Outdoor (5)" }));
    expect(screen.getByText("Compressor")).toBeDefined();
    expect(new URL(location.href).searchParams.get("tab")).toBe("outdoor");
    cleanup();
    store.tab = "bogus";
    render(<WorkspaceDemo id="job-x" />);
    expect(screen.getByText("Evaporator coil")).toBeDefined(); // an unknown tab falls back to indoor
  });

  it("adds and edits readings, adds and removes parts, shows the time on site, saves and signs the draft", () => {
    render(<WorkspaceDemo id="job-x" />);
    fireEvent.click(button(/Add reading/));
    const cells = screen.getAllByDisplayValue("New reading");
    fireEvent.change(cells[0] ?? document.body, { target: { value: "Outdoor temperature" } });
    expect(screen.getByDisplayValue("Outdoor temperature")).toBeDefined();
    fireEvent.click(screen.getByText("(see Parts & refrigerant)"));
    expect(screen.getByText("Refrigerant (R32) — cylinder CYL-R32-0182")).toBeDefined();
    fireEvent.click(button(/Add part/));
    fireEvent.click(button("Add", dialog()));
    expect(toast).toHaveBeenCalledWith("Part added — van stock is deducted when the report is accepted");
    expect(screen.getByText("Capacitor 35 µF")).toBeDefined();
    expect(screen.getByText(/Replacement parts: 3 lines/)).toBeDefined();
    fireEvent.click(screen.getAllByRole("button", { name: "Remove" })[0] ?? document.body);
    expect(screen.queryByText("Air filter (standard)")).toBeNull();
    expect(screen.getByText(/Replacement parts: 2 lines/)).toBeDefined();
    fireEvent.click(screen.getByRole("tab", { name: "Time on site" }));
    expect(screen.getByText("1 h 33 min")).toBeDefined();
    expect(screen.queryByRole("button", { name: /Add (reading|part)/ })).toBeNull();
    fireEvent.click(button("Save draft"));
    expect(toast).toHaveBeenCalledWith("Draft autosaved 11:44");
    fireEvent.click(button("Get signature"));
    fireEvent.click(button("Save signature", dialog()));
    expect(screen.getByText("Signed · Ms Tan · 11:52")).toBeDefined();
    expect(button("Sign again")).toBeDefined();
  });

  it("closes every dialog with Cancel or Escape without its change, and adds a demo photo", () => {
    store.jobs = [assignment()];
    const live = render(<WorkspaceDemo id="job-live" />);
    fireEvent.click(button("Can’t make this time…"));
    fireEvent.click(button("Cancel", dialog()));
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(button("Can’t make this time…"));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(techCantMake).not.toHaveBeenCalled();
    live.unmount();
    render(<WorkspaceDemo id="job-x" />);
    fireEvent.click(button(/Add photo/));
    expect(toast).toHaveBeenCalledWith("Photo added (demo)");
    for (const [open, onPartsTab] of [[/Add part/, true], ["Get signature", false]] as const) {
      if (onPartsTab) fireEvent.click(screen.getByText("(see Parts & refrigerant)"));
      fireEvent.click(button(open));
      fireEvent.click(button("Cancel", dialog()));
      expect(screen.queryByRole("dialog")).toBeNull();
      fireEvent.click(button(open));
      fireEvent.keyDown(dialog(), { key: "Escape" });
      expect(screen.queryByRole("dialog")).toBeNull();
    }
    expect(screen.getByText(/Replacement parts: 2 lines/)).toBeDefined();
    expect(screen.getByText("Not signed")).toBeDefined();
    for (const tab of ["Indoor (8)", "Outdoor (5)", "Electrical (5)"]) { fireEvent.click(screen.getByRole("tab", { name: tab })); answerAllNormal(); }
    fireEvent.click(button("Submit report"));
    fireEvent.click(button("Cancel", dialog()));
    fireEvent.click(button("Submit report"));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText(/Work window ends in 15 min/)).toBeDefined(); // still in progress
    cleanup();
    render(<WorkspaceDemo id="job-t07" />);
    fireEvent.click(button("Preview check-in"));
    fireEvent.click(button("Cancel", dialog()));
    fireEvent.click(button("Preview check-in"));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toast).not.toHaveBeenCalledWith("Checked in — job started (demo)");
  });

  it("puts the job on hold and back, and previews the check-in of an assigned job; a submitted job is read-only", () => {
    const { unmount } = render(<WorkspaceDemo id="job-x" />);
    fireEvent.click(button("Demo: HQ puts the job on hold"));
    expect(screen.getByText(/On hold by HQ/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Submit report" })).toBeNull();
    expect(screen.getAllByRole<HTMLButtonElement>("button", { name: "Normal" }).every((b) => b.disabled)).toBe(true);
    fireEvent.click(button("Demo: HQ resumes"));
    expect(screen.getByText(/Work window ends in 15 min/)).toBeDefined();
    unmount();
    const assigned = render(<WorkspaceDemo id="job-t07" />);
    expect(button("Start job").disabled).toBe(true);
    fireEvent.click(button("Preview check-in"));
    expect(within(dialog()).getByText("job-t07 · Bedroom AC · Now 14:02")).toBeDefined();
    fireEvent.click(button("Check in & start", dialog()));
    expect(toast).toHaveBeenCalledWith("Checked in — job started (demo)");
    assigned.unmount();
    render(<WorkspaceDemo id="job-p09" />);
    expect(screen.getByText("Submitted v1 — read-only, awaiting quality review.")).toBeDefined();
    expect(button(/Add reading/).disabled).toBe(true);
  });
});
