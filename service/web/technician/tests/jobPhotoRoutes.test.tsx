// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const { attachmentResponse, jobPhotoResponse } = vi.hoisted(() => ({
  attachmentResponse: vi.fn<(input: { jobId: string; reportId: string; reportVersion: number; attachmentId: string }) => Promise<Response>>(),
  jobPhotoResponse: vi.fn<(jobId: string, attachmentId: string) => Promise<Response>>(),
}));
vi.mock("@ac/web/lib/attachmentResponse", () => ({ attachmentResponse }));
vi.mock("@ac/web/lib/jobPhotoResponse", () => ({ jobPhotoResponse }));

import { GET as reportPhoto } from "../app/technician/jobs/[id]/photos/[attachmentId]/route";
import { GET as problemPhoto } from "../app/technician/jobs/[id]/problem-photos/[attachmentId]/route";
import { JobHistory } from "../app/technician/jobs/[id]/_components/job-history";

// the two photo routes of the job workspace and the ended-job card: a report photo needs its report id and a positive
// whole version before the DAL is asked; a problem photo goes straight to the Core API, which decides the access
const params = Promise.resolve({ id: "job-1", attachmentId: "att-1" });
const image = new Response("png", { status: 200 });

afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe("report photo route", () => {
  it("streams the photo of the given report version", async () => {
    attachmentResponse.mockResolvedValue(image);
    const res = await reportPhoto(new Request("https://t/technician/jobs/job-1/photos/att-1?reportId=r-1&reportVersion=3"), { params });
    expect(res).toBe(image);
    expect(attachmentResponse).toHaveBeenCalledWith({ jobId: "job-1", reportId: "r-1", reportVersion: 3, attachmentId: "att-1" });
  });

  it.each([
    ["no report id", "reportVersion=3"],
    ["an empty report id", "reportId=&reportVersion=3"],
    ["no version", "reportId=r-1"],
    ["version 0", "reportId=r-1&reportVersion=0"],
    ["a negative version", "reportId=r-1&reportVersion=-1"],
    ["a fractional version", "reportId=r-1&reportVersion=1.5"],
    ["a non-number version", "reportId=r-1&reportVersion=abc"],
    ["a version past the safe range", "reportId=r-1&reportVersion=9007199254740993"],
  ])("refuses %s with 400 and asks nothing", async (_name, query) => {
    const res = await reportPhoto(new Request(`https://t/technician/jobs/job-1/photos/att-1?${query}`), { params });
    expect(res.status).toBe(400);
    await expect(res.text()).resolves.toBe("reportId and reportVersion are required");
    expect(attachmentResponse).not.toHaveBeenCalled();
  });
});

describe("problem photo route", () => {
  it("passes the job and attachment to the Core API", async () => {
    jobPhotoResponse.mockResolvedValue(image);
    const res = await problemPhoto(new Request("https://t/technician/jobs/job-1/problem-photos/att-1"), { params });
    expect(res).toBe(image);
    expect(jobPhotoResponse).toHaveBeenCalledWith("job-1", "att-1");
  });
});

describe("JobHistory", () => {
  it("shows the frozen snapshot with its rows, note and the way back", () => {
    render(<JobHistory card={{ title: "job-1 · Repair", sub: "Your assignment on this job has ended", rows: [["Status", "Cancelled when your assignment ended"], ["Report", "None"]], note: "Read only", back: "Back to jobs" }} />);
    expect(screen.getByText("job-1 · Repair")).toBeTruthy();
    expect(screen.getByText("Your assignment on this job has ended")).toBeTruthy();
    expect(screen.getByText("Cancelled when your assignment ended")).toBeTruthy();
    expect(screen.getByText("Report")).toBeTruthy();
    expect(screen.getByText("Read only")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Back to jobs" }).getAttribute("href")).toBe("/technician");
  });
});
