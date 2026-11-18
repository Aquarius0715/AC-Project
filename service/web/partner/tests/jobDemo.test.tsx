// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Job } from "@ac/web/lib/jobs";

// The Phase 1A demo job detail: contractor-a's store jobs (offer, proposal, assignment banners and actions), the seeded
// offer job-p09 and its states, the other fixed jobs as a summary with their next step, and "not available" for any
// other id
const store = vi.hoisted((): { jobs: Job[] } => ({ jobs: [] }));
const toast = vi.hoisted(() => vi.fn());
vi.mock("@ac/web/lib/jobs", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useJobs: () => store.jobs }));
vi.mock("@ac/web/components/ui", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useToast: () => toast }));

import { getJobState, jobActions } from "@ac/web/lib/jobs";
import { JobDemo } from "../app/partner/jobs/[id]/(detail)/_components/job-demo";

const base = () => {
  const job = getJobState().jobs.find((j) => j.id === "job-a11");
  if (!job) throw new Error("fixture without job-a11");
  return job;
};
const job = (over: Partial<Job>): Job => ({ ...base(), id: "job-x1", contractor: "contractor-a", status: "offered", partnerProposal: null, techAck: null, ...over });
const proposal = (status: "pending" | "sent_to_client" | "rejected" | "approved"): Job["partnerProposal"] => ({ slot: { date: "2026-09-25", win: "09:00–11:00" }, tech: "tech-external-a", reason: "r", sentAt: "09-21 10:00", status });

beforeEach(() => { store.jobs = []; toast.mockClear(); });
afterEach(() => { cleanup(); vi.restoreAllMocks(); jobActions.reset(); }); // the spied store actions run for real

const click = (name: string) => { fireEvent.click(screen.getByRole("button", { name })); };
/** The banners' texts, without their action links. */
const banners = () => [...document.querySelectorAll("[role=alert] > div, [role=status] > div")].map((b) => b.textContent);
const assignHref = () => screen.getByRole("link", { name: "Assign technician" }).getAttribute("href");
const notAvailable = () => screen.queryByText("This page isn’t available");

describe("Job detail demo: which view an id gets", () => {
  it("shows not available for another company's job, an unknown id and the cross-company fixture", () => {
    store.jobs = [job({ id: "job-b1", contractor: "contractor-b" })];
    for (const id of ["job-b1", "job-zz", "job-b-offer"]) {
      render(<JobDemo id={id} />);
      expect(notAvailable(), id).not.toBeNull();
      expect(screen.queryByRole("button", { name: "Accept job" }), id).toBeNull();
      cleanup();
    }
  });

  it("shows a fixed job past the offer as its summary, without the offer's actions", () => {
    render(<JobDemo id="job-p02" />);
    expect(notAvailable()).toBeNull();
    expect(screen.queryByRole("button", { name: "Accept job" })).toBeNull();
    expect(screen.queryByText("job-p02 · Offer from HQ")).toBeNull();
    expect(document.querySelector("[data-slot=card-title]")?.textContent).toBe("job-p02 · Rooftop unit");
    expect(screen.getByText("Accepted 09-14 10:02 · Reactive maintenance · refrigerant cert required")).toBeDefined();
    expect(screen.getByText("— · 2 qualified free")).toBeDefined();
    expect(screen.getByText("Delegation ends 09-22 00:00 · Assign before 09-21")).toBeDefined();
    expect(assignHref()).toBe("/partner/schedule?jobId=job-p02");
  });

  it("leads each fixed job to its next step, and a completed one to none", () => {
    const back = ["Jobs", "/partner/jobs"];
    const cases: [string, string[][]][] = [
      ["job-p07", [back, ["Reassign technician", "/partner/schedule?jobId=job-p07"]]],
      ["job-p05", [back, ["Review report", "/partner/jobs/job-p05/review"]]],
      ["job-p12", [back, ["Assign technician", "/partner/schedule?jobId=job-p12"]]],
      ["job-p03", [back]],
      ["job-p04", [back]],
      ["job-contractor-a", [back]],
    ];
    for (const [id, links] of cases) {
      render(<JobDemo id={id} />);
      expect(screen.getAllByRole("link").map((a) => [a.textContent.trim(), a.getAttribute("href")]), id).toStrictEqual(links);
      cleanup();
    }
  });

  it("prefers contractor-a's store job to a fixed job, except the seeded offer job-p09", () => {
    store.jobs = [job({ id: "job-p02" }), job({ id: "job-p09", status: "submitted" })];
    render(<JobDemo id="job-p02" />);
    expect(screen.getByText("job-p02 · Offer from HQ")).toBeDefined();
    expect(screen.getByText("Expires 2026-09-25 12:00")).toBeDefined();
    cleanup();
    render(<JobDemo id="job-p09" />);
    expect(screen.getByText("2026-09-16 10:00–12:00 · from the periodic plan")).toBeDefined();
    expect(screen.queryByText("Expires 2026-09-25 12:00")).toBeNull();
  });
});

describe("Job detail demo: the seeded offer job-p09", () => {
  it("accepts and then leads to the schedule", () => {
    render(<JobDemo id="job-p09" />);
    click("Accept job");
    expect(banners()).toContain("Accepted — receipt: job-p09 · access window 2026-09-14 01:00 – 2026-09-22 00:00. Assign a technician in Schedule.");
    expect(assignHref()).toBe("/partner/schedule?jobId=job-p02");
    expect(screen.queryByRole("button", { name: "Accept job" })).toBeNull();
    expect(toast).toHaveBeenCalledWith("Offer accepted");
  });

  it("declines only with a reason", () => {
    render(<JobDemo id="job-p09" />);
    click("Decline…");
    click("Confirm decline");
    expect(screen.getByText("A reason is required (1–1000 characters)")).toBeDefined();
    expect(toast).not.toHaveBeenCalled();
    fireEvent.change(screen.getByPlaceholderText("No qualified technician available at this time."), { target: { value: "  no one  " } });
    click("Confirm decline");
    expect(banners()).toContain("Declined with reason. HQ was notified.");
    expect(toast).toHaveBeenCalledWith("Offer declined", "warn");
  });

  it("proposes another time only with a reason, and cancels the dialogs", () => {
    render(<JobDemo id="job-p09" />);
    click("Decline…");
    click("Cancel");
    expect(screen.queryByRole("button", { name: "Confirm decline" })).toBeNull();
    click("Propose another time…");
    click("Cancel");
    expect(screen.queryByRole("button", { name: "Send to HQ" })).toBeNull();
    click("Propose another time…");
    click("Send to HQ");
    expect(screen.getByText("A reason is required")).toBeDefined();
    fireEvent.change(screen.getByPlaceholderText("tech-external-a has a certificate renewal course on 10-01 morning."), { target: { value: "course" } });
    click("Send to HQ");
    expect(banners()).toContain("Time change sent to HQ — the client must approve it. The offer stays reserved for you.");
    expect(toast).toHaveBeenCalledWith("Sent to HQ — waiting for the client");
    expect(screen.queryByRole("button", { name: "Send to HQ" })).toBeNull();
  });

  it("previews the expired state without actions, and colours the free days", () => {
    render(<JobDemo id="job-p09" />);
    expect(screen.getAllByText("4 h").at(0)?.parentElement?.className).toContain("text-ok");
    expect(screen.getAllByText("—").at(0)?.parentElement?.className).toContain("text-muted-foreground");
    click("Preview: expired / cancelled state");
    expect(banners()).toContain("This offer expired or was cancelled by HQ. No action is possible.");
    expect(screen.queryByRole("button", { name: "Accept job" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Assign technician" })).toBeNull();
  });
});

describe("Job detail demo: a store job", () => {
  it("accepts an offer through the store", () => {
    const accept = vi.spyOn(jobActions, "partnerAccept");
    store.jobs = [job({ history: [1, 2, 3, 4, 5, 6].map((n) => ({ at: `09-2${String(n)}`, text: `step ${String(n)}` })) })];
    render(<JobDemo id="job-x1" />);
    expect(screen.getByText("Expires 2026-09-25 12:00")).toBeDefined();
    expect(screen.getByText("Reactive maintenance")).toBeDefined();
    expect([...document.querySelectorAll("li")].map((li) => li.textContent)).toStrictEqual(["09-22 · step 2", "09-23 · step 3", "09-24 · step 4", "09-25 · step 5", "09-26 · step 6"]);
    click("Accept job");
    expect(accept).toHaveBeenCalledWith("job-x1");
    expect(toast).toHaveBeenCalledWith("Offer accepted");
  });

  it("declines and proposes through the store with the trimmed reason and the picked slot", () => {
    const decline = vi.spyOn(jobActions, "partnerDecline");
    const propose = vi.spyOn(jobActions, "partnerPropose");
    store.jobs = [job({ origin: "plan" })];
    render(<JobDemo id="job-x1" />);
    expect(screen.getByText("Periodic inspection")).toBeDefined();
    click("Decline…");
    click("Cancel");
    click("Propose another time…");
    click("Cancel");
    expect(screen.queryByRole("dialog")).toBeNull();
    click("Decline…");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: " busy " } });
    click("Confirm decline");
    expect(decline).toHaveBeenCalledWith("job-x1", "busy");
    expect(toast).toHaveBeenCalledWith("Offer declined", "warn");
    click("Propose another time…");
    fireEvent.change(screen.getByLabelText("Date"), { target: { value: "2026-10-03" } });
    fireEvent.change(screen.getByLabelText("Window"), { target: { value: "16:00–18:00" } });
    fireEvent.change(screen.getByLabelText("Technician"), { target: { value: "tech-external-a2" } });
    fireEvent.change(screen.getByLabelText("Reason (sent to HQ and the client) · required"), { target: { value: " course " } });
    click("Send to HQ");
    expect(propose).toHaveBeenCalledWith("job-x1", { date: "2026-10-03", win: "16:00–18:00" }, "tech-external-a2", "course");
    expect(screen.queryByRole("button", { name: "Send to HQ" })).toBeNull();
  });

  it("holds Accept while its proposal is open, and withdraws it", () => {
    const withdraw = vi.spyOn(jobActions, "partnerWithdraw");
    for (const status of ["pending", "sent_to_client"] as const) {
      store.jobs = [job({ partnerProposal: proposal(status) })];
      render(<JobDemo id="job-x1" />);
      expect(screen.getByText("Time change proposed · waiting for client")).toBeDefined();
      expect(screen.getByText("Fri 09-25 · 09:00–11:00 · sent 09-21 10:00")).toBeDefined();
      expect(screen.getByText("09-24 · 09:00–12:00", { selector: "s" })).toBeDefined();
      expect(screen.getByRole("button", { name: "Accept job" }).hasAttribute("disabled")).toBe(true);
      click("Withdraw my proposal");
      cleanup();
    }
    expect(withdraw).toHaveBeenCalledTimes(2);
    expect(toast).toHaveBeenCalledWith("Proposal withdrawn");
  });

  it("shows the client's answer to its proposal on an offer", () => {
    store.jobs = [job({ partnerProposal: proposal("approved") })];
    render(<JobDemo id="job-x1" />);
    expect(banners()).toContain("The client approved your time (09-24 · 09:00–12:00). Accept the offer to continue.");
    cleanup();
    store.jobs = [job({ partnerProposal: proposal("rejected") })];
    render(<JobDemo id="job-x1" />);
    expect(banners()).toContain("Your time change was not approved — the agreed time stays 09-24 · 09:00–12:00. Accept or decline.");
    expect(screen.getByRole("button", { name: "Accept job" }).hasAttribute("disabled")).toBe(false);
  });

  it("shows the stage after the offer, without the offer's actions", () => {
    const cases: [Partial<Job>, string][] = [
      [{ status: "accepted" }, "Accepted — assign your technician for exactly 09-24 · 09:00–12:00."],
      [{ status: "assigned", technician: "tech-external-a", techAck: { status: "accepted" } }, "Assigned to tech-external-a · accepted by the technician."],
      [{ status: "in_progress", technician: "tech-external-a", techAck: { status: "cant_make" } }, "Assigned to tech-external-a · technician can’t make it — reassign in Schedule."],
      [{ status: "assigned", technician: "tech-external-a", techAck: null }, "Assigned to tech-external-a · waiting for the technician to accept."],
      [{ status: "time_proposed", partnerProposal: proposal("approved") }, "HQ has asked the client about your proposed time. You will be notified of the answer."],
    ];
    for (const [over, text] of cases) {
      store.jobs = [job(over)];
      render(<JobDemo id="job-x1" />);
      expect(banners(), text).toContain(text);
      expect(banners().some((b) => b.startsWith("The client approved")), text).toBe(false);
      expect(screen.queryByRole("button", { name: "Accept job" }), text).toBeNull();
      cleanup();
    }
    store.jobs = [job({ status: "accepted" })];
    render(<JobDemo id="job-x1" />);
    expect(assignHref()).toBe("/partner/schedule?jobId=job-x1");
  });

  it("shows a dash for a job without a visit time", () => {
    store.jobs = [job({ scheduled: null })];
    render(<JobDemo id="job-x1" />);
    expect(screen.getByText("— · agreed with the client", { selector: "b" })).toBeDefined();
  });
});
