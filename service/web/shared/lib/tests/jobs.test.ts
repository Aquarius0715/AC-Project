// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { fitFor, fmt, getJobState, jobActions, longDate, SEED, statusLabel, type Slot } from "@ac/web/lib/jobs";
import { must } from "./must";

const TEXT_14_00_16_00 = "14:00–16:00";
const TEXT_2026_10_06 = "2026-10-06";
const TEXT_10_00_12_00 = "10:00–12:00";
const TECH_INTERNAL_A = "tech-internal-a";
const JOB_INTERNAL_A = "job-internal-a";
const CONTRACTOR = "contractor";
const CONTRACTOR_A = "contractor-a";
const TECH_EXTERNAL_A = "tech-external-a";

// The Phase 1A maintenance demo (IR113, FR-X05): one store for the four apps of a tab. The actions are local and
// synchronous, so a refusal comes back at once.
const job = (id: string) => must(getJobState().jobs.find((j) => j.id === id));
const notes = (role: string) => getJobState().notes.filter((n) => n.role === role);
const slot: Slot = { date: "2026-10-05", win: TEXT_14_00_16_00 };
const prefs: Slot[] = [{ date: TEXT_2026_10_06, win: TEXT_10_00_12_00 }, { date: "2026-10-07", win: TEXT_10_00_12_00 }, { date: "2026-10-08", win: TEXT_10_00_12_00 }];
const completed = must(SEED.find((j) => j.status === "completed"));

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
    jobActions.book("job-c07", slot, "internal", TECH_INTERNAL_A);
    expect(job("job-c07")).toMatchObject({ status: "assigned", scheduled: slot, delivery: "internal", technician: TECH_INTERNAL_A, techAck: { status: "pending" } });
    expect([notes("technician")[0]?.title, notes("client")[0]?.title]).toStrictEqual(["New assignment job-c07", "Booked: job-c07"]);
    jobActions.book(JOB_INTERNAL_A, slot, CONTRACTOR, CONTRACTOR_A);
    expect(job(JOB_INTERNAL_A)).toMatchObject({ status: "offered", delivery: CONTRACTOR, contractor: CONTRACTOR_A, technician: null });
    expect(notes(CONTRACTOR)[0]?.title).toBe("New offer job-internal-a");
  });

  it("follows HQ's proposal: accepted with whoever was held, declined with new times, or withdrawn", () => {
    jobActions.propose("job-c07", slot, TECH_INTERNAL_A, "Can we come then?", "09-24");
    expect(job("job-c07")).toMatchObject({ status: "time_proposed", proposal: { by: "hq", status: "pending", replyBy: "09-24" } });
    jobActions.accept("job-c07");
    expect(job("job-c07")).toMatchObject({ status: "assigned", delivery: "internal", technician: TECH_INTERNAL_A, proposal: { status: "accepted" } });
    jobActions.propose(JOB_INTERNAL_A, slot, "contractor-a (held)", "Or then?", "09-24");
    jobActions.accept(JOB_INTERNAL_A);
    expect(job(JOB_INTERNAL_A)).toMatchObject({ status: "offered", delivery: CONTRACTOR, contractor: CONTRACTOR_A });

    jobActions.reset();
    jobActions.propose("job-c07", slot, TECH_INTERNAL_A, "Can we come then?", "09-24");
    jobActions.decline("job-c07", "time_not_suitable", "evenings only", prefs);
    expect(job("job-c07")).toMatchObject({ status: "requested", round: 2, preferred: prefs, declined: { reason: "time_not_suitable", comment: "evenings only" }, proposal: { status: "declined" } });
    jobActions.propose("job-c07", slot, TECH_INTERNAL_A, "Or then?", "09-24");
    jobActions.withdraw("job-c07");
    expect(job("job-c07")).toMatchObject({ status: "requested", proposal: { status: "withdrawn" } });
    expect(notes("client")[0]?.title).toBe("Proposal withdrawn — job-c07");
  });

  it("forwards a contractor's time to the customer, who approves it, or HQ keeps the agreed time", () => {
    jobActions.partnerPropose("job-a11", slot, TECH_EXTERNAL_A, "van in repair");
    expect(job("job-a11").partnerProposal).toMatchObject({ status: "pending", tech: TECH_EXTERNAL_A, reason: "van in repair" });
    jobActions.forwardPartner("job-a11", "09-24");
    expect(job("job-a11")).toMatchObject({ status: "time_proposed", proposal: { by: CONTRACTOR, who: "contractor-a · tech-external-a" }, partnerProposal: { status: "sent_to_client" } });
    jobActions.accept("job-a11");
    expect(job("job-a11")).toMatchObject({ status: "offered", scheduled: slot, proposal: { status: "accepted" }, partnerProposal: { status: "approved" } });
    jobActions.partnerPropose("job-a11", slot, TECH_EXTERNAL_A, "again");
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
    expect(jobActions.addNote("job-c07", " Gate code 4821 ")).toBeNull();
    expect(job("job-c07").notes?.at(-1)).toMatchObject({ by: "customer-a", text: "Gate code 4821" });
    jobActions.addNote("job-a11", "Parking at the back");
    expect(notes(CONTRACTOR)[0]?.title).toBe("Client note on job-a11");
  });

  it("opens a follow-up from a completed job only, and classifies it once", () => {
    expect(jobActions.reportProblem("job-c07", "same_problem", "still dripping", "")).toStrictEqual({ error: "CONFLICT — only completed jobs can be reported" });
    const reported = jobActions.reportProblem(completed.id, "same_problem", "still dripping", TEXT_2026_10_06);
    expect(reported).toHaveProperty("id");
    const id = "id" in reported ? reported.id ?? "" : "";
    expect(job(id)).toMatchObject({ status: "requested", followUpOf: completed.id, followUpClass: "pending", unitId: completed.unitId, preferred: [{ date: TEXT_2026_10_06, win: "09:00–12:00" }] });
    expect(jobActions.classifyFollowUp(id, "rework", " ")).toBe("A reason is required (1–1000 characters)");
    expect(jobActions.classifyFollowUp(id, "rework", "same fault")).toBeNull();
    expect(job(id)).toMatchObject({ followUpClass: "rework", followUpReason: "same fault" });
    expect(jobActions.classifyFollowUp(id, "new_request", "again")).toBe("CONFLICT — already classified");
    expect(notes("client")[0]?.title).toBe(`${id}: Rework (free)`);
  });

  it("takes the contractor's and the technician's answers, and the customer's cancellation", () => {
    jobActions.partnerAccept("job-a11");
    expect(job("job-a11").status).toBe("accepted");
    jobActions.partnerAssign("job-a11", TECH_EXTERNAL_A);
    expect(job("job-a11")).toMatchObject({ status: "assigned", technician: TECH_EXTERNAL_A, techAck: { status: "pending" } });
    jobActions.techCantMake("job-a11", "sick", "09-26 AM");
    expect(job("job-a11").techAck).toMatchObject({ status: "cant_make", reason: "sick", alt: "09-26 AM" });
    expect(notes(CONTRACTOR)[0]?.title).toBe("tech-external-a can’t make job-a11"); // the contractor's own job
    jobActions.techAccept("job-c05");
    expect(job("job-c05").techAck?.status).toBe("accepted");
    expect(notes("client")[0]?.title).toBe("Technician confirmed — job-c05");
    jobActions.book(JOB_INTERNAL_A, slot, CONTRACTOR, CONTRACTOR_A);
    jobActions.partnerDecline(JOB_INTERNAL_A, "no free hours");
    expect(job(JOB_INTERNAL_A)).toMatchObject({ status: "requested", contractor: null, delivery: null, scheduled: null });
    jobActions.cancel("job-c07", "moved out");
    const cancelled = job("job-c07");
    expect(cancelled.status).toBe("cancelled");
    expect(cancelled.history.map((h) => h.text)).toContain("Cancelled by client · moved out");
  });

  it("marks a notice read and resets to the seed", () => {
    jobActions.cancel("job-c07", "moved out");
    const n = must(notes("admin")[0]);
    jobActions.markRead(n.id);
    expect(notes("admin")[0]?.read).toBe(true);
    jobActions.reset();
    expect([job("job-c07").status, getJobState().notes]).toStrictEqual(["requested", []]);
  });
});

describe("demo availability and slot texts", () => {
  it("names who fits a time and formats slots", () => {
    expect([fitFor({ date: "2026-09-28", win: TEXT_14_00_16_00 }).fits, fitFor({ date: "2026-10-02", win: TEXT_14_00_16_00 }).fits, fitFor(slot).fits, fitFor({ date: "2026-10-05", win: TEXT_10_00_12_00 }).fits])
      .toStrictEqual(["none", "none", "both", CONTRACTOR]);
    expect([fmt(slot), fmt(null), longDate(slot)]).toStrictEqual(["10-05 · 14:00–16:00", "—", "Mon 10-05 · 14:00–16:00"]);
    expect(Object.keys(statusLabel)).toHaveLength(11);
  });
});
