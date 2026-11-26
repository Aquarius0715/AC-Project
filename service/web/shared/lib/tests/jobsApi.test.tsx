// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

// The maintenance jobs as the screens read them (lib/jobs.ts): jobs.list's summary, offer and history projections
// turned into the screens' jobs in Kuala Lumpur time, and the demo actions' other paths — a contractor's proposal
// accepted or declined, a note on a contractor's job, a problem report without a visit day, a follow-up classified as
// a new request, and a contractor technician's answers.
const api = vi.hoisted(() => ({ items: [] as unknown[] }));
vi.mock("@ac/web/lib/useOp", () => ({
  useOp: (_op: string, _input: unknown, _mock: unknown, map: (data: { items: unknown[] }) => unknown) => ({ data: map({ items: api.items }), loading: false, error: null, source: "api" }),
}));

import { getJobState, jobActions, useJobs, type Job, type Slot } from "@ac/web/lib/jobs";

const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const job = (id: string): Job | undefined => getJobState().jobs.find((j) => j.id === id);
const notes = (role: string) => getJobState().notes.filter((n) => n.role === role);
const visit: Slot = { date: "2026-10-05", win: "14:00–16:00" };

beforeEach(() => {
  sessionStorage.clear();
  jobActions.reset();
});

describe("jobs.list as the screens read it", () => {
  it("reads a summary in Kuala Lumpur time, an offer at its requested time, and a history row by its job ID", () => {
    api.items = [
      { projection: "summary", id: "job-1", unitId: "unit-online-rto", type: "preventive", status: "assigned", origin: "periodic_plan", scheduledSlot: slot("2026-10-05T06:00:00Z", "2026-10-05T08:00:00Z"),
        preferredSlots: [slot("2026-10-06T02:00:00Z", "2026-10-06T04:00:00Z")] },
      { projection: "offer", id: "job-2", type: "periodic", status: "offered", requestedSlot: slot("2026-10-07T16:30:00Z", "2026-10-07T18:00:00Z") },
      { projection: "history", jobId: "job-3", type: "installation", status: "teleported" },
      { projection: "summary", type: "reactive", status: "requested", displayStatus: "time_proposed" },
    ];
    const { result } = renderHook(() => useJobs());
    const [summary, offer, history, proposed] = result.current;
    expect([summary?.id, summary?.unit, summary?.unitId, summary?.type, summary?.status, summary?.origin, summary?.scheduled, summary?.preferred]).toStrictEqual([
      "job-1", "Unit unit-onl", "unit-online-rto", "Preventive", "assigned", "plan", { date: "2026-10-05", win: "14:00–16:00" }, [{ date: "2026-10-06", win: "10:00–12:00" }],
    ]);
    expect([offer?.scheduled, offer?.type, offer?.unit, offer?.preferred]).toStrictEqual([{ date: "2026-10-08", win: "00:30–02:00" }, "Periodic", "Unit", []]); // past midnight in Kuala Lumpur: the next day
    expect([history?.id, history?.status, history?.type, history?.scheduled]).toStrictEqual(["job-3", "requested", "Reactive", null]); // an unknown status reads as a new request, an unknown type as reactive
    expect([proposed?.id, proposed?.status]).toStrictEqual(["", "time_proposed"]);
  });
});

describe("the demo actions' other paths", () => {
  it("offers a job when the client accepts a contractor's proposed time, and keeps it offered when they decline one", () => {
    const id = getJobState().jobs.find((j) => j.status === "requested")?.id ?? "";
    jobActions.book(id, visit, "contractor", "contractor-a");
    jobActions.partnerPropose(id, { date: "2026-10-09", win: "09:00–11:00" }, "tech-external-a", "Parts arrive Friday");
    jobActions.forwardPartner(id, "2026-10-04");
    expect(job(id)?.status).toBe("time_proposed");
    jobActions.accept(id);
    expect([job(id)?.status, job(id)?.scheduled, job(id)?.partnerProposal?.status]).toStrictEqual(["offered", { date: "2026-10-09", win: "09:00–11:00" }, "approved"]);
    expect(notes("contractor")[0]?.title).toBe(`Client approved your time for ${id}`);
    jobActions.partnerPropose(id, visit, "tech-external-a", "Again");
    jobActions.forwardPartner(id, "2026-10-04");
    jobActions.decline(id, "Too late", "", []);
    expect([job(id)?.status, job(id)?.partnerProposal?.status, job(id)?.round]).toStrictEqual(["offered", "rejected", 1]); // no new times: the round stays
    expect(notes("admin")[0]?.detail).toBe("Too late");
  });

  it("offers a job to the contractor HQ held when the client accepts HQ's proposal", () => {
    const id = getJobState().jobs.find((j) => j.status === "requested")?.id ?? "";
    jobActions.propose(id, visit, "contractor-a · tech-external-a", "Earliest free", "2026-10-04");
    jobActions.accept(id);
    expect([job(id)?.status, job(id)?.delivery, job(id)?.contractor]).toStrictEqual(["offered", "contractor", "contractor-a"]);
    jobActions.accept("job-none"); // nothing proposed: nothing changes
    jobActions.forwardPartner(id, "2026-10-04");
    expect(job(id)?.status).toBe("offered");
  });

  it("tells the contractor about a note on its job, and its technician's answers go to the contractor", () => {
    const id = getJobState().jobs.find((j) => j.status === "requested")?.id ?? "";
    jobActions.book(id, visit, "contractor", "contractor-a");
    jobActions.partnerAccept(id);
    jobActions.partnerAssign(id, "tech-external-a");
    expect(jobActions.addNote(id, " Gate code 4321 ")).toBeNull();
    expect(notes("contractor")[0]).toMatchObject({ title: `Client note on ${id}`, detail: "Gate code 4321" });
    expect([jobActions.addNote(id, " "), jobActions.addNote("job-none", "x")]).toStrictEqual(["Write 1–2000 characters", "CONFLICT — notes are only possible on open jobs"]);
    jobActions.techAccept(id);
    expect(notes("contractor")[0]).toMatchObject({ title: `tech-external-a accepted ${id}`, href: "/partner/schedule" });
    jobActions.techCantMake(id, "Van broke down", "");
    expect(notes("contractor")[0]).toMatchObject({ title: `tech-external-a can’t make ${id}`, detail: "Van broke down" });
    jobActions.techCantMake("job-none", "x", "Tue");
    expect(notes("admin")[0]).toMatchObject({ title: "— can’t make job-none", detail: "x · could do Tue" });
  });

  it("reports a problem without a visit day and classifies the follow-up as a new request", () => {
    const done = getJobState().jobs.find((j) => j.status === "completed");
    const made = jobActions.reportProblem(done?.id ?? "", "Still noisy", "Rattle at night", "");
    const id = made.id ?? "";
    expect(job(id)?.preferred).toStrictEqual([]);
    expect(jobActions.classifyFollowUp(id, "new_request", " Different fault ")).toBeNull();
    expect([job(id)?.followUpClass, job(id)?.followUpReason, notes("client")[0]?.title]).toStrictEqual(["new_request", "Different fault", `${id}: New request`]);
    expect(jobActions.classifyFollowUp(id, "rework", "Again")).toBe("CONFLICT — already classified");
    expect(jobActions.reportProblem("job-none", "x", "y", "")).toStrictEqual({ error: "CONFLICT — only completed jobs can be reported" });
  });

  it("withdraws a proposal and keeps the agreed time against a contractor's", () => {
    const id = getJobState().jobs.find((j) => j.status === "requested")?.id ?? "";
    jobActions.withdraw(id); // nothing proposed: back at requested without one
    expect([job(id)?.status, job(id)?.proposal]).toStrictEqual(["requested", null]);
    jobActions.keepTime(id);
    expect(job(id)?.partnerProposal).toBeNull();
  });
});
