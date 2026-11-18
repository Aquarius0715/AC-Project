// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { Job } from "@ac/web/lib/jobs";

// The Phase 1A demo overview: the offers to answer and the action list built from the shared store's contractor-a
// jobs (job-p09 is one of the fixed rows), then the fixed rows; the other cards are fixed
const store = vi.hoisted((): { jobs: Job[] } => ({ jobs: [] }));
vi.mock("@ac/web/lib/jobs", async (importOriginal) => ({ ...(await importOriginal<Record<string, unknown>>()), useJobs: () => store.jobs }));

import { getJobState } from "@ac/web/lib/jobs";
import { OverviewDemo } from "../app/partner/(overview)/_components/overview-demo";

beforeEach(() => { store.jobs = getJobState().jobs; }); // the seed, as the demo shows it
afterEach(cleanup);

const actionRows = () => [...(screen.getByText(/^Needs your action/).closest("[data-slot=card]")?.querySelectorAll(":scope li a") ?? [])].map((a) => [a.querySelector("b")?.textContent, a.getAttribute("href")]);
const FIXED = [
  ["job-p09 · Lobby AC · customer-b", "/partner/jobs/job-p09"], ["job-p07 · Server room AC", "/partner/schedule?jobId=job-p07"],
  ["job-p05 · Server room AC", "/partner/jobs/job-p05/review"], ["job-p02 · Rooftop unit", "/partner/schedule?jobId=job-p02"],
  ["job-p12 · Lobby AC", "/partner/schedule?jobId=job-p12"],
];
const base = () => {
  const job = getJobState().jobs.find((j) => j.id === "job-a11");
  if (!job) throw new Error("fixture without job-a11");
  return job;
};
const job = (over: Partial<Job>): Job => ({ ...base(), contractor: "contractor-a", partnerProposal: null, techAck: null, ...over });

describe("Overview demo", () => {
  it("lists the store's open offer before the fixed rows, counts it in the offers tile and the card title, and shows the fixed timeline", () => {
    render(<OverviewDemo />);
    expect(actionRows()).toStrictEqual([["job-a11 · Rooftop unit · customer-b", "/partner/jobs/job-a11"], ...FIXED]);
    expect(screen.getByText("Needs your action (6)")).toBeTruthy();
    expect(screen.getByText("Open offers").closest("[data-slot=card]")?.textContent).toMatch(/^Offers to answer2job-p09/);
    expect(screen.getByText("10–12").style.left).toBe("16.6%");
    expect(screen.getAllByText("Free all day")).toHaveLength(1);
  });

  it("turns accepted jobs and a technician who can't make it into assign rows, and skips other companies, job-p09 and offers with a pending proposal", () => {
    store.jobs = [
      job({ id: "job-x1", status: "offered", partnerProposal: { slot: { date: "2026-09-25", win: "09:00–12:00" }, tech: "tech-external-a", reason: "Earlier visit", sentAt: "2026-09-21T01:00:00Z", status: "pending" } }),
      job({ id: "job-x2", status: "accepted" }),
      job({ id: "job-x3", status: "assigned", technician: "tech-external-a", techAck: { status: "cant_make" } }),
      job({ id: "job-x4", status: "assigned", technician: null, techAck: { status: "cant_make" } }),
      job({ id: "job-x5", status: "accepted", contractor: "contractor-b" }),
      job({ id: "job-p09", status: "accepted" }),
    ];
    render(<OverviewDemo />);
    expect(actionRows()).toStrictEqual([
      ["job-x2 · Rooftop unit", "/partner/schedule?jobId=job-x2"], ["job-x3 · Rooftop unit", "/partner/schedule?jobId=job-x3"],
      ["job-x4 · Rooftop unit", "/partner/schedule?jobId=job-x4"], ...FIXED,
    ]);
    expect(screen.getByText(/^Accepted · assign a technician for /)).toBeTruthy();
    expect(screen.getByText(/^tech-external-a can’t make .* — reassign$/)).toBeTruthy();
    expect(screen.getByText(/^— can’t make .* — reassign$/)).toBeTruthy();
    // the pending-proposal offer still counts as an offer to answer
    expect(screen.getByText("Open offers").closest("[data-slot=card]")?.textContent).toMatch(/^Offers to answer2job-p09/);
  });
});
