// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { fitFor, fmt, getJobState, jobActions, longDate, SEED, statusLabel, type Slot } from "@ac/web/lib/jobs";

// The Phase 1A maintenance demo (IR113, FR-X05): one store for the four apps of a tab. The actions are local and
// synchronous, so a refusal comes back at once.
const job = (id: string) => getJobState().jobs.find((j) => j.id === id)!;
const notes = (role: string) => getJobState().notes.filter((n) => n.role === role);
const slot: Slot = { date: "2026-10-05", win: "14:00–16:00" };
const prefs: Slot[] = [{ date: "2026-10-06", win: "10:00–12:00" }, { date: "2026-10-07", win: "10:00–12:00" }, { date: "2026-10-08", win: "10:00–12:00" }];
const completed = SEED.find((j) => j.status === "completed")!;

beforeEach(() => {
  sessionStorage.clear();
  jobActions.reset();
});

describe("a request in the maintenance demo", () => {
  it("is created with three preferred times and announced to HQ", () => {
    const id = jobActions.create({ unitId: "unit-bedroom-2", unit: "Bedroom AC #2", loc: "Home A", type: "Reactive", symptom: "Water dripping", preferred: prefs });
    expect(id).toBe("job-c09"); // after the seed's job-c08
    expect(job(id)).toMatchObject({ status: "requested", preferred: prefs, customer: "customer-a", history: [{ text: "customer-a submitted the request with 3 preferred times" }] });
    expect(notes("admin")[0]).toMatchObject({ title: `New client request ${id}`, href: `/admin/jobs?jobId=${id}`, read: false });
  });

  it("is booked at a preferred time for an internal technician, or offered to a contractor", () => {
    jobActions.book("job-c07", slot, "internal", "tech-internal-a");
    expect(job("job-c07")).toMatchObject({ status: "assigned", scheduled: slot, delivery: "internal", technician: "tech-internal-a", techAck: { status: "pending" } });
    expect([notes("technician")[0].title, notes("client")[0].title]).toEqual(["New assignment job-c07", "Booked: job-c07"]);
    jobActions.book("job-internal-a", slot, "contractor", "contractor-a");
    expect(job("job-internal-a")).toMatchObject({ status: "offered", delivery: "contractor", contractor: "contractor-a", technician: null });
    expect(notes("contractor")[0].title).toBe("New offer job-internal-a");
  });

  it("follows HQ's proposal: accepted with whoever was held, declined with new times, or withdrawn", () => {
    jobActions.propose("job-c07", slot, "tech-internal-a", "Can we come then?", "09-24");
    expect(job("job-c07")).toMatchObject({ status: "time_proposed", proposal: { by: "hq", status: "pending", replyBy: "09-24" } });
    jobActions.accept("job-c07");
    expect(job("job-c07")).toMatchObject({ status: "assigned", delivery: "internal", technician: "tech-internal-a", proposal: { status: "accepted" } });
    jobActions.propose("job-internal-a", slot, "contractor-a (held)", "Or then?", "09-24");
    jobActions.accept("job-internal-a");
    expect(job("job-internal-a")).toMatchObject({ status: "offered", delivery: "contractor", contractor: "contractor-a" });

    jobActions.reset();
    jobActions.propose("job-c07", slot, "tech-internal-a", "Can we come then?", "09-24");
    jobActions.decline("job-c07", "time_not_suitable", "evenings only", prefs);
    expect(job("job-c07")).toMatchObject({ status: "requested", round: 2, preferred: prefs, declined: { reason: "time_not_suitable", comment: "evenings only" }, proposal: { status: "declined" } });
    jobActions.propose("job-c07", slot, "tech-internal-a", "Or then?", "09-24");
    jobActions.withdraw("job-c07");
    expect(job("job-c07")).toMatchObject({ status: "requested", proposal: { status: "withdrawn" } });
    expect(notes("client")[0].title).toBe("Proposal withdrawn — job-c07");
  });

  it("forwards a contractor's time to the customer, who approves it, or HQ keeps the agreed time", () => {
    jobActions.partnerPropose("job-a11", slot, "tech-external-a", "van in repair");
    expect(job("job-a11").partnerProposal).toMatchObject({ status: "pending", tech: "tech-external-a", reason: "van in repair" });
    jobActions.forwardPartner("job-a11", "09-24");
    expect(job("job-a11")).toMatchObject({ status: "time_proposed", proposal: { by: "contractor", who: "contractor-a · tech-external-a" }, partnerProposal: { status: "sent_to_client" } });
    jobActions.accept("job-a11");
    expect(job("job-a11")).toMatchObject({ status: "offered", scheduled: slot, proposal: { status: "accepted" }, partnerProposal: { status: "approved" } });
    jobActions.partnerPropose("job-a11", slot, "tech-external-a", "again");
    jobActions.keepTime("job-a11");
    expect(job("job-a11").partnerProposal?.status).toBe("rejected");
    jobActions.partnerWithdraw("job-a11");
    expect(job("job-a11").partnerProposal).toBeNull();
  });

  it("asks for another time for a periodic visit", () => {
    jobActions.requestOther("job-c08", prefs, "travelling");
    expect(job("job-c08")).toMatchObject({ status: "requested", preferred: prefs, scheduled: null, technician: null, techAck: null, round: 2 });
  });
});

describe("notes, follow-ups and answers in the maintenance demo", () => {
  it("refuses a note on a closed job or an empty one at once, and tells the contractor of its own job", () => {
    expect(jobActions.addNote(completed.id, "Call first")).toBe("CONFLICT — notes are only possible on open jobs");
    expect(jobActions.addNote("job-c07", "  ")).toBe("Write 1–2000 characters");
    expect(jobActions.addNote("job-c07", " Gate code 4821 ")).toBeUndefined();
    expect(job("job-c07").notes?.at(-1)).toMatchObject({ by: "customer-a", text: "Gate code 4821" });
    jobActions.addNote("job-a11", "Parking at the back");
    expect(notes("contractor")[0].title).toBe("Client note on job-a11");
  });

  it("opens a follow-up from a completed job only, and classifies it once", () => {
    expect(jobActions.reportProblem("job-c07", "same_problem", "still dripping", "")).toEqual({ error: "CONFLICT — only completed jobs can be reported" });
    const { id } = jobActions.reportProblem(completed.id, "same_problem", "still dripping", "2026-10-06") as { id: string };
    expect(job(id)).toMatchObject({ status: "requested", followUpOf: completed.id, followUpClass: "pending", unitId: completed.unitId, preferred: [{ date: "2026-10-06", win: "09:00–12:00" }] });
    expect(jobActions.classifyFollowUp(id, "rework", " ")).toBe("A reason is required (1–1000 characters)");
    expect(jobActions.classifyFollowUp(id, "rework", "same fault")).toBeUndefined();
    expect(job(id)).toMatchObject({ followUpClass: "rework", followUpReason: "same fault" });
    expect(jobActions.classifyFollowUp(id, "new_request", "again")).toBe("CONFLICT — already classified");
    expect(notes("client")[0].title).toBe(`${id}: Rework (free)`);
  });

  it("takes the contractor's and the technician's answers, and the customer's cancellation", () => {
    jobActions.partnerAccept("job-a11");
    expect(job("job-a11").status).toBe("accepted");
    jobActions.partnerAssign("job-a11", "tech-external-a");
    expect(job("job-a11")).toMatchObject({ status: "assigned", technician: "tech-external-a", techAck: { status: "pending" } });
    jobActions.techCantMake("job-a11", "sick", "09-26 AM");
    expect(job("job-a11").techAck).toMatchObject({ status: "cant_make", reason: "sick", alt: "09-26 AM" });
    expect(notes("contractor")[0].title).toBe("tech-external-a can’t make job-a11"); // the contractor's own job
    jobActions.techAccept("job-c05");
    expect(job("job-c05").techAck?.status).toBe("accepted");
    expect(notes("client")[0].title).toBe("Technician confirmed — job-c05");
    jobActions.book("job-internal-a", slot, "contractor", "contractor-a");
    jobActions.partnerDecline("job-internal-a", "no free hours");
    expect(job("job-internal-a")).toMatchObject({ status: "requested", contractor: null, delivery: null, scheduled: null });
    jobActions.cancel("job-c07", "moved out");
    expect(job("job-c07")).toMatchObject({ status: "cancelled", history: expect.arrayContaining([expect.objectContaining({ text: "Cancelled by client · moved out" })]) });
  });

  it("marks a notice read and resets to the seed", () => {
    jobActions.cancel("job-c07", "moved out");
    const n = notes("admin")[0];
    jobActions.markRead(n.id);
    expect(notes("admin")[0].read).toBe(true);
    jobActions.reset();
    expect([job("job-c07").status, getJobState().notes]).toEqual(["requested", []]);
  });
});

describe("demo availability and slot texts", () => {
  it("names who fits a time and formats slots", () => {
    expect([fitFor({ date: "2026-09-28", win: "14:00–16:00" }).fits, fitFor({ date: "2026-10-02", win: "14:00–16:00" }).fits, fitFor(slot).fits, fitFor({ date: "2026-10-05", win: "10:00–12:00" }).fits])
      .toEqual(["none", "none", "both", "contractor"]);
    expect([fmt(slot), fmt(null), longDate(slot)]).toEqual(["10-05 · 14:00–16:00", "—", "Mon 10-05 · 14:00–16:00"]);
    expect(Object.keys(statusLabel)).toHaveLength(11);
  });
});
