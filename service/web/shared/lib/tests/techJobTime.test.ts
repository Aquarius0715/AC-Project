import { describe, expect, it } from "vitest";
import { followUpText, historyCard, timeInstants, timeRows, versionRows, type ApiTechHistory } from "@ac/web/lib/techJob";

// The technician's job workspace (FR-T13, FR-T14, IR49, IR124): the time on site for a manual check-in without a
// reason, one started without an arrival, one counted by the server, several ended pauses; the versions with a draft
// never saved and an accepted review; the follow-up text; and the snapshot of a job whose assignment ended (completed
// without its day, cancelled, reassigned; no report, accepted, not accepted).

const NOW = Date.parse("2026-09-14T03:00:00Z");
const row = (rows: [string, string][], key: string) => rows.find(([k]) => k === key)?.[1];
const history = (over: Partial<ApiTechHistory>): ApiTechHistory => ({
  projection: "history", jobId: "job-c02-0000", type: "periodic", status: "completed", asOf: "2026-09-14T02:00:00Z", completedAt: null, redactedReportSummary: { hasReport: false, acceptance: "not_accepted" }, ...over,
});

describe("the time on site", () => {
  it("lists every instant to format, and none without a time on site", () => {
    expect(timeInstants({ arrivedAt: "a", startedAt: "s", finishedAt: null, pauses: [{ from: "p1", to: null }] })).toStrictEqual(["a", "s", null, "p1", null]);
    expect(timeInstants(null)).toStrictEqual([undefined, undefined, undefined]);
  });

  it("shows a manual check-in without a reason, a start without an arrival, and the server's own count", () => {
    const rows = timeRows({ startedAt: "2026-09-14T01:00:00Z", checkInMethod: "manual", checkInReason: null, onSiteMinutes: 95 }, NOW);
    expect([row(rows, "Arrived")?.endsWith("· manual check-in"), row(rows, "Location"), row(rows, "On-site time"), row(rows, "Finished")]).toStrictEqual([true, "manual — no reason", "1 h 35 min", "— (on submit)"]);
  });

  it("counts several ended pauses, and a check-in without a distance", () => {
    const rows = timeRows({
      arrivedAt: "2026-09-14T01:00:00Z", checkInMethod: "location_qr", distanceMeters: null, finishedAt: "2026-09-14T02:30:00Z",
      pauses: [{ from: "2026-09-14T01:10:00Z", to: "2026-09-14T01:20:00Z" }, { from: "2026-09-14T01:40:00Z", to: "2026-09-14T01:45:00Z" }],
    }, NOW);
    expect([row(rows, "Location"), row(rows, "Started"), row(rows, "Paused"), row(rows, "On-site time")]).toStrictEqual(["—", "—", "2 pauses · 15 min", "1 h 15 min"]);
  });
});

describe("the versions", () => {
  it("shows a draft never saved, and an accepted version without a reason", () => {
    const rows = versionRows({ version: 3, results: 1, photos: 1, savedAt: null }, [{ reportVersion: 2 }], [{ reviewerUserId: "r", reportVersion: 2, decision: "accept", reason: null, occurredAt: "2026-09-13T05:00:00Z" }]);
    expect(rows.map((r) => [r.title, r.badge.text])).toStrictEqual([["Draft v3", "Draft"], ["v2 · accepted", "Accepted"]]);
    expect([rows[0]?.sub, rows[1]?.sub.includes(" — ")]).toStrictEqual(["1 result · 1 photo", false]);
  });

  it("words no next action and a follow-up without its date", () => {
    expect([followUpText({ kind: "none" }), followUpText({ kind: "follow_up", date: "", note: "Bring the part" })]).toStrictEqual(["None", "Follow-up — — Bring the part"]);
  });
});

describe("a job whose assignment ended", () => {
  it("states a completed job without its day and no report", () => {
    const card = historyCard(history({}));
    expect([card.sub, row(card.rows, "Status"), row(card.rows, "Your report")]).toStrictEqual(["Completed — your assignment ended with the job", "Completed", "No report from your assignment"]);
    expect(card.note).toBe("Your time is free for other jobs. The report, the unit and its devices stay with HQ and the customer.");
  });

  it("states a cancelled job with an accepted report, and a reassigned one not accepted", () => {
    const cancelled = historyCard(history({ status: "cancelled", redactedReportSummary: { hasReport: true, acceptance: "accepted" } }));
    expect([row(cancelled.rows, "Status"), row(cancelled.rows, "Your report"), cancelled.note]).toStrictEqual([
      "cancelled when your assignment ended", "Accepted in the quality review", "The job was cancelled, so the job details and the unit are no longer shown to you.",
    ]);
    const moved = historyCard(history({ status: "in_progress", redactedReportSummary: { hasReport: true, acceptance: "not_accepted" } }));
    expect([moved.sub, row(moved.rows, "Your report"), moved.note.startsWith("Your work window ended")]).toStrictEqual(["Your assignment on this job has ended", "Submitted — not accepted while you were assigned", true]);
  });
});
