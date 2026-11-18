// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { Job } from "@ac/web/lib/jobs";

// The Phase 1A demo job list: contractor-a's store jobs (not the fixed ones, not cancelled) before the fixed rows,
// with their stage, technician answer and link; the status tabs and the origin filter; the demo's empty and stale states
const store = vi.hoisted((): { jobs: Job[] } => ({ jobs: [] }));
vi.mock("@ac/web/lib/jobs", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useJobs: () => store.jobs }));

import { getJobState } from "@ac/web/lib/jobs";
import { JobsDemo } from "../app/partner/jobs/(list)/_components/jobs-demo";

beforeEach(() => { store.jobs = getJobState().jobs; }); // the seed, as the demo shows it
afterEach(cleanup);

const base = () => {
  const job = getJobState().jobs.find((j) => j.id === "job-a11");
  if (!job) throw new Error("fixture without job-a11");
  return job;
};
const job = (over: Partial<Job>): Job => ({ ...base(), contractor: "contractor-a", partnerProposal: null, techAck: null, ...over });
const proposal = (status: "pending" | "sent_to_client" | "rejected"): Job["partnerProposal"] => ({ slot: { date: "2026-09-25", win: "09:00–12:00" }, tech: "tech-external-a", reason: "r", sentAt: "09-21", status });
/** Each row's title, its line, the technician column, its badge and where it leads. */
const rows = () => screen.getAllByRole("link").map((a) => {
  const [title = "", tech = "", , status = ""] = [...a.children].map((c) => c.textContent);
  return { tech, status, job: title, href: a.getAttribute("href") };
});
const idOf = (title: string) => title.split(" · ", 1)[0];
const ids = () => rows().map((r) => idOf(r.job));
/** The badge classes of the row whose title starts with the id. */
const badge = (id: string) => screen.getByText(id, { exact: false, selector: "b" }).closest("a")?.querySelector("[data-slot=badge]")?.className;

describe("Jobs demo", () => {
  it("lists the store's contractor-a jobs before the fixed rows, with their stage, technician answer and link", () => {
    render(<JobsDemo />);
    expect(ids()).toStrictEqual(["job-c05", "job-a11", "job-p09", "job-contractor-a", "job-p05", "job-p02", "job-p12", "job-p07", "job-p03", "job-p04"]);
    expect(rows().slice(0, 2)).toStrictEqual([
      { job: "job-c05 · Kitchen AC · customer-aClient requestAssigned tech-external-a2 · 09-25 · 09:00–11:00", tech: "tech-external-a2accepted", status: "Assigned", href: "/partner/jobs/job-c05" },
      { job: "job-a11 · Rooftop unit · customer-bClient requestOffered by HQ · visit 09-24 · 09:00–12:00 (fixed)", tech: "—not assignable before acceptance", status: "Offered", href: "/partner/jobs/job-a11" },
    ]);
    expect(screen.getByText("Answer by 09-25 12:00")).toBeTruthy();
    // a fixed row in review opens the review, a fixed unassigned row stays on the job
    expect(rows().find((r) => r.job.startsWith("job-p05"))?.href).toBe("/partner/jobs/job-p05/review");
    expect(rows().find((r) => r.job.startsWith("job-p02"))?.href).toBe("/partner/jobs/job-p02");
    expect(screen.getByRole("heading", { name: "Jobs" })).toBeTruthy();
    expect(screen.getByText("contractor-a only · 10 this week")).toBeTruthy();
    expect(screen.getByText(/^Showing 10 of 10/)).toBeTruthy();
  });

  it("names a proposed time, an accepted job waiting for a technician, and the technician's answer; skips other companies, the fixed ids and cancelled jobs", () => {
    store.jobs = [
      job({ id: "job-x1", status: "time_proposed", partnerProposal: proposal("sent_to_client") }),
      job({ id: "job-x2", status: "offered", partnerProposal: proposal("pending") }),
      job({ id: "job-x3", status: "offered", partnerProposal: proposal("rejected") }),
      job({ id: "job-x4", status: "accepted", technician: null }),
      job({ id: "job-x5", status: "assigned", technician: "tech-external-a", techAck: { status: "cant_make" } }),
      job({ id: "job-x6", status: "assigned", technician: "tech-external-a", techAck: { status: "pending" }, scheduled: null }),
      job({ id: "job-x7", status: "assigned", contractor: "contractor-b" }),
      job({ id: "job-p09", status: "offered" }),
      job({ id: "job-x8", status: "cancelled" }),
    ];
    render(<JobsDemo />);
    const live = rows().slice(0, 6);
    expect(live.map((r) => [idOf(r.job), r.status, r.tech, r.href])).toStrictEqual([
      ["job-x1", "Time proposed", "—not assignable before acceptance", "/partner/jobs/job-x1"],
      ["job-x2", "Time proposed", "—not assignable before acceptance", "/partner/jobs/job-x2"],
      ["job-x3", "Offered", "—not assignable before acceptance", "/partner/jobs/job-x3"],
      ["job-x4", "Unassigned", "—not assignable before acceptance", "/partner/schedule?jobId=job-x4"],
      ["job-x5", "Assigned", "tech-external-acan’t make it", "/partner/jobs/job-x5"],
      ["job-x6", "Assigned", "tech-external-aawaiting acceptance", "/partner/jobs/job-x6"],
    ]);
    expect(live[0]?.job).toContain("Time change proposed · waiting for the client");
    expect(live[3]?.job).toContain("Accepted · assign a technician for the agreed time");
    expect(live[5]?.job).toContain("Assigned tech-external-a · —");
    expect([badge("job-x1"), badge("job-x2")].map((c) => c?.includes("text-warn"))).toStrictEqual([true, true]);
    expect(badge("job-x3")).not.toMatch(/warn/); // a rejected proposal is not pending
    expect(ids()).not.toContain("job-x7");
    expect(ids()).not.toContain("job-x8");
    expect(ids().filter((id) => id === "job-p09")).toHaveLength(1);
    // an unassigned fixed row with the same label still opens the job: only a store job goes to the schedule
    expect(rows().find((r) => r.job.startsWith("job-p12"))?.href).toBe("/partner/jobs/job-p12");
  });

  it("filters by tab (Active counts Overdue) and origin, opens on Offered from the overview's link, and says when a filter is empty", () => {
    const view = render(<JobsDemo status="offered" />);
    expect(ids()).toStrictEqual(["job-a11", "job-p09"]);
    const tabs = screen.getAllByRole("tab").map((t) => t.textContent);
    expect(tabs).toStrictEqual(["All10", "Offered2", "Active5", "Review1", "Completed2"]);
    fireEvent.click(screen.getByRole("tab", { name: /Active/ }));
    expect(ids()).toStrictEqual(["job-c05", "job-contractor-a", "job-p02", "job-p12", "job-p07"]);
    const origin = screen.getByRole("combobox", { name: "Origin" });
    fireEvent.change(origin, { target: { value: "plan" } });
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    expect(screen.getByText("No jobs in this filter.")).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: /All/ }));
    expect(ids()).toStrictEqual(["job-p09"]);
    fireEvent.change(origin, { target: { value: "request" } });
    expect(ids()).not.toContain("job-p09");
    fireEvent.change(origin, { target: { value: "bogus" } }); // not an option: the filter stays
    expect(within(view.container).getByRole("combobox", { name: "Origin" })).toHaveProperty("value", "request");
  });

  it("switches to the empty and stale demo states and back", () => {
    render(<JobsDemo />);
    fireEvent.click(screen.getByRole("button", { name: "empty state" }));
    expect(screen.getByText("No jobs yet")).toBeTruthy();
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "stale state" }));
    expect(screen.getByText("Couldn’t refresh jobs (stale)")).toBeTruthy();
    expect(screen.getAllByRole("link")).toHaveLength(10);
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(screen.queryByText("Couldn’t refresh jobs (stale)")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "stale state" }));
    fireEvent.click(screen.getByRole("button", { name: "normal" }));
    expect(screen.queryByText("Couldn’t refresh jobs (stale)")).toBeNull();
    expect(screen.getAllByRole("link")).toHaveLength(10);
  });
});
