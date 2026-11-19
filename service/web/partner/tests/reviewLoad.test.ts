import { beforeEach, describe, expect, it, vi } from "vitest";

// the contractor's quality review (FR-P05): the delegated job with its unit, the last submitted report version, the
// author and reviewer names, the linked alerts it may read and at most 6 ready photos of ≤ 2 MB as data URLs.
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) { super(code, options); this.name = "CoreError"; this.error = { code }; }
  }
  return {
    CoreError, coreAll: vi.fn(), coreOp: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(),
    orAbsent: async <T>(read: Promise<T>): Promise<T | null> => {
      try { return await read; } catch (error) {
        if (error instanceof CoreError && (error.error.code === "FORBIDDEN" || error.error.code === "NOT_FOUND")) return null;
        throw error;
      }
    },
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadReview } from "../app/partner/jobs/[id]/review/_lib/load";

const job = {
  projection: "detail", id: "job-p05-aaaa", version: 4, status: "submitted", type: "maintenance", unitId: "unit-p-01-xyz", alertIds: ["al-1", "al-2"],
  dueAt: "2026-09-21T00:00:00Z", reportRefs: [{ reportId: "rep-old", reportVersion: 1 }, { reportId: "rep-1", reportVersion: 2 }], draftReportRef: null, timeOnSite: null, assignment: null,
};
const attachment = (id: string, size: number, status = "ready") => ({ name: `${id}.jpg`, mime: "image/jpeg", id, size, status });
const item = (id: string, authorId: string) => ({ id, authorId, componentGroup: "indoor", componentKey: "filter", result: "normal", reason: null, evidenceIds: [], observedAt: "2026-09-15T03:00:00Z" });
// both reviews are of version 1: the history is given oldest first, so the loader must sort it
const review = (decision: string, occurredAt: string, reason: string | null) => ({ decision, occurredAt, reason, reviewerUserId: "u-me", reportVersion: 1 });
const report = {
  id: "rep-1", version: 2, jobId: "job-p05-aaaa", authorId: "u-tech-1", reviewAvailability: { allowed: true, reason: null }, items: [item("i1", "u-tech-1"), item("i2", "u-tech-2")],
  measurements: [], parts: [], refrigerant: [], signOff: { signerName: "Mr Lee", signedAt: "2026-09-15T04:00:00Z", absentReason: null }, workText: "Cleaned the filter.",
  nextAction: { kind: "follow_up", date: "2026-10-01", note: "check the drain" }, submittedAt: "2026-09-15T04:30:00Z", acceptedAt: null,
  // 8 attachments: one still processing, one over 2 MB, one whose content is refused, 5 others
  attachmentRefs: [attachment("a1", 10), attachment("a2", 10, "processing"), attachment("a3", 2_000_001), attachment("a4", 2_000_000), attachment("a5", 10), attachment("gone", 10), attachment("a7", 10), attachment("a8", 10), attachment("a9", 10)],
  reviewHistory: [review("accept", "2026-09-14T06:00:00Z", null), review("return", "2026-09-14T08:00:00Z", "photos missing")],
};
const attachmentOf = (input: unknown) => (typeof input === "object" && input !== null && "attachmentId" in input ? input.attachmentId : null);
const operationOf = (call: unknown[]) => call[0];
/** the attachment IDs whose content was read, in order */
const contentReads = () => dal.coreOp.mock.calls.filter((call: unknown[]) => operationOf(call) === "attachments.getContent").map((call: unknown[]) => attachmentOf(call[1]));

describe("partner review loader", () => {
  let current: object;
  let currentReport: object;
  beforeEach(() => {
    vi.clearAllMocks();
    current = job;
    currentReport = report;
    dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
    dal.corePrincipal.mockResolvedValue({ userId: "u-me" });
    dal.coreAll.mockResolvedValue([{ userId: "u-tech-1", displayName: "Tech One" }, { userId: "u-tech-2", displayName: null }]);
    dal.coreOp.mockImplementation((operation: string, input: { id?: string; attachmentId?: string }) => {
      if (operation === "jobs.get") return Promise.resolve(current);
      if (operation === "units.get") return Promise.resolve({ displayName: "Lobby AC", location: { pathLabels: ["Tower A", "Floor 1"] } });
      if (operation === "reports.get") return Promise.resolve(currentReport);
      if (operation === "alerts.get" && input.id === "al-1") return Promise.resolve({ id: "al-1", type: "high_temp", severity: "critical", status: "open", evidenceText: "", detectedAt: "2026-09-14T00:30:00Z" });
      if (operation === "attachments.getContent" && input.attachmentId !== "gone") return Promise.resolve({ mime: "image/jpeg", bytes: `b-${input.attachmentId ?? ""}` });
      return Promise.reject(new dal.CoreError("NOT_FOUND"));
    });
  });

  it("reads the last report version with its readable alerts, at most 6 small ready photos and the latest review", async () => {
    const live = await loadReview("job-p05-aaaa");
    expect(live.job).toStrictEqual({ id: "job-p05-aaaa", version: 4, status: "submitted", type: "maintenance", unit: "Lobby AC · Tower A › Floor 1" });
    expect(live.reviewer).toBe("u-me");
    expect(dal.coreOp).toHaveBeenCalledWith("reports.get", { jobId: "job-p05-aaaa", reportId: "rep-1", reportVersion: 2 });
    const r = live.report;
    if (!r) throw new Error("expected the report");
    // a2 is processing and a3 over 2 MB; of the next 6 the refused one is dropped
    expect(contentReads()).toStrictEqual(["a1", "a4", "a5", "gone", "a7", "a8"]);
    expect(r.photos.map((p) => p.id)).toStrictEqual(["a1", "a4", "a5", "a7", "a8"]);
    expect(r.photos[0]).toStrictEqual({ id: "a1", name: "a1.jpg", url: "data:image/jpeg;base64,b-a1" });
    expect(r.author).toBe("Tech One");
    // the latest review of the history, whatever its order
    expect(r.last).toStrictEqual({ decision: "return", version: 1, reason: "photos missing" });
    expect(r.next).toBe("follow-up 1 Oct 2026 — check the drain");
    expect(r.signOff).toBe("Mr Lee · 15 Sept 2026, 12:00 pm MYT");
    expect(r.submitted).toBe("15 Sept 2026, 12:30 pm MYT");
    // two report versions, the one readable alert, the due row
    expect(r.versions.map((v) => v.title)).toStrictEqual([
      "v1 · returned 14 Sept 2026, 4:00 pm MYT", "v2 · submitted 15 Sept 2026, 12:30 pm MYT", "high_temp (alert)", "Job due",
    ]);
    // a contributor without a display name is named by the start of the user ID
    expect(r.evidence.at(-1)?.value).toBe("Tech One, u-tech-2");
    expect([r.allowed, r.blocked]).toStrictEqual([true, null]);
  });

  it("names the signed-in reviewer \"you\" and an unknown author by the start of the ID", async () => {
    currentReport = { ...report, authorId: "u-me", items: [item("i1", "u-me")] };
    const mine = await loadReview("job-p05-aaaa");
    expect(mine.report?.author).toBe("you");
    current = { ...job, alertIds: null };
    dal.coreOp.mockClear();
    currentReport = { ...report, authorId: "u-unknown-123", nextAction: { kind: "none" }, signOff: null, submittedAt: null, reviewHistory: [], attachmentRefs: [] };
    const other = await loadReview("job-p05-aaaa");
    const r = other.report;
    if (!r) throw new Error("expected the report");
    expect([r.author, r.next, r.signOff, r.submitted, r.last, r.photos]).toStrictEqual(["technician u-unknow", "no follow-up needed", null, null, null, []]);
    // without attachments nothing is read, and a job without linked alerts reads none
    expect(contentReads()).toStrictEqual([]);
    expect(dal.coreOp).not.toHaveBeenCalledWith("alerts.get", expect.anything());
    expect(r.versions.map((v) => v.title).at(-1)).toBe("Job due");
  });

  it("shows no report without a submitted version, and names a refused unit by its ID", async () => {
    current = { ...job, reportRefs: [] };
    dal.coreOp.mockImplementation((operation: string) => (operation === "jobs.get" ? Promise.resolve(current) : Promise.reject(new dal.CoreError("FORBIDDEN"))));
    const none = await loadReview("job-p05-aaaa");
    expect(none).toStrictEqual({ job: { id: "job-p05-aaaa", version: 4, status: "submitted", type: "maintenance", unit: "unit unit-p-0" }, reviewer: "u-me", report: null });
    expect(dal.coreOp).not.toHaveBeenCalledWith("reports.get", expect.anything());
  });

  it("reads neither the unit nor a report outside the detail projection, and falls back to the job ID asked for", async () => {
    current = { projection: "history", jobId: "job-p05-aaaa", status: "completed", type: "maintenance" };
    const history = await loadReview("job-p05-aaaa");
    expect(history).toStrictEqual({ job: { id: "job-p05-aaaa", version: 0, status: "completed", type: "maintenance", unit: null }, reviewer: "u-me", report: null });
    current = { projection: "offer", status: "offered", type: "maintenance", unitId: "unit-p-01-xyz", reportRefs: job.reportRefs };
    const offer = await loadReview("job-asked");
    expect([offer.job.id, offer.job.unit, offer.report]).toStrictEqual(["job-asked", "unit unit-p-0", null]);
    expect(dal.coreOp.mock.calls.map((call: unknown[]) => operationOf(call))).toStrictEqual(["jobs.get", "jobs.get"]);
  });

  it("fails on an unavailable unit, alert or photo, and on a refused report", async () => {
    for (const failing of ["units.get", "alerts.get", "attachments.getContent", "reports.get"]) {
      const code = failing === "reports.get" ? "NOT_FOUND" : "UNAVAILABLE";
      dal.coreOp.mockImplementation((operation: string) => {
        if (operation === failing) return Promise.reject(new dal.CoreError(code));
        if (operation === "jobs.get") return Promise.resolve(current);
        if (operation === "reports.get") return Promise.resolve(currentReport);
        if (operation === "attachments.getContent") return Promise.resolve({ mime: "image/jpeg", bytes: "x" });
        return Promise.resolve({ id: "al-1", type: "t", severity: "warning", status: "open", evidenceText: "", detectedAt: "2026-09-14T00:30:00Z", displayName: "U", location: { pathLabels: [] } });
      });
      await expect(loadReview("job-p05-aaaa"), failing).rejects.toThrow(code);
    }
  });
});
