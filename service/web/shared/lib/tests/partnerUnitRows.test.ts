import { describe, expect, it } from "vitest";
import { evidenceRows, jobContext, pastWork, readingRows, registerRows, snapshotRows, type ApiUnitJob } from "@ac/web/lib/partnerUnit";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiHistory } from "@ac/web/lib/partnerJobDetail";
import { unitDetail } from "./unitFixture";

// The contractor's unit view (FR-P04, FR-P08, DD-P04): a register without location or scope, a job with its
// technician (named or not) and no access window, past work ordered with jobs never completed, an alert of an unknown
// evidence kind, a reading with no value, the power state off or on in a mode without a word, and the history
// snapshot of a declined offer, an accepted report and a completed job.

const NOW = Date.parse("2026-09-14T01:35:00Z");
type Reading = ApiUnitDetail["latestMeasurements"][number];
const m = (metric: Reading["metric"], value: number | null, unit: Reading["unit"]): Reading =>
  ({ metric, value, unit, id: metric, unitId: "u1", sensorId: "s", observedAt: "2026-09-14T01:30:00Z", origin: "measured", quality: "valid", qualityReason: null });
type Summary = Extract<ApiUnitJob, { projection: "summary" }>;
const summary = (over: Partial<Summary>): Summary => ({ projection: "summary", id: "job-p02-aaaa", type: "reactive", status: "assigned", scheduledSlot: null, technicianMembershipId: null, ...over });
const history = (over: Partial<ApiHistory>): ApiHistory => ({ projection: "history", jobId: "job-h", type: "reactive", status: "completed", asOf: "2026-09-14T00:00:00Z", completedAt: null, ownDecisionEvents: [], ...over });
const decision = (action: string, occurredAt: string) => ({ action, occurredAt, id: action, jobId: "job-h", actorUserId: null });

describe("the register and the job context", () => {
  it("shows a dash for an empty location and scope, and a scope it has no word for as stored", () => {
    const empty = registerRows(unitDetail({ location: { pathLabels: [], address: null, accessInstructions: null }, serviceScope: [] })).rows;
    expect([empty[0]?.[1], empty[2]?.[1]]).toStrictEqual(["—", "—"]);
  });

  it("names the technician, an unknown one by their role, and a job without an access window", () => {
    const names = new Map([["m-a", "Aiman"]]);
    const named = jobContext(summary({ technicianMembershipId: "m-a" }), names, NOW);
    expect([named.rows[1]?.[1], named.techMissing, named.rows[2]?.[1], named.pct]).toStrictEqual(["Aiman", false, "—", null]);
    expect(named.note).toBe("Access ends with the accepted offer’s access window; after that only a history snapshot remains.");
    expect(jobContext(summary({ technicianMembershipId: "m-gone" }), names, NOW).rows[1]?.[1]).toBe("technician");
    expect(jobContext(summary({ accessValidFrom: "2026-09-14T01:35:00Z", accessValidUntil: "2026-09-14T01:35:00Z" }), names, NOW).pct).toBeNull(); // an empty window: no bar
  });
});

describe("past work and evidence", () => {
  it("lists the newest completed work first, jobs never completed last", () => {
    const rows = pastWork([
      { projection: "history", jobId: "job-old", type: "reactive", status: "completed", completedAt: "2026-06-01T00:00:00Z" },
      { projection: "history", jobId: "job-none", type: "periodic", status: "cancelled", completedAt: null },
      { projection: "history", jobId: "job-new", type: "reactive", status: "completed", completedAt: "2026-09-01T00:00:00Z" },
    ]);
    expect(rows.map((r) => [r.id, r.badge.tone])).toStrictEqual([["job-new", "ok"], ["job-old", "ok"], ["job-none", "muted"]]);
    expect(rows[2]?.sub).toBe("not completed");
  });

  it("words an unknown evidence kind as evidence", () => {
    const alert: ApiAlert = { id: "a1", unitId: "u1", type: "sensor", severity: "warning", status: "open", causeCode: "unknown", evidenceKind: "telemetry_model", evidenceText: "Pressure drift.", detectedAt: "2026-09-14T01:00:00Z" };
    expect(evidenceRows([alert])[0]?.sub).toMatch(/^Evidence — Pressure drift\. · /);
  });
});

describe("the readings", () => {
  it("shows a reading without a value, the power off, on in a mode without a word, and no time", () => {
    const off = readingRows(unitDetail({ latestMeasurements: [m("temperature", null, "°C")], observedState: { power: false, celsius: null, mode: null, fanLevel: null, observedAt: null } }));
    expect([off.rows.map((r) => r.value), off.observed]).toStrictEqual([["— · good", "Off"], "—"]);
    const fan = readingRows(unitDetail({ latestMeasurements: [], observedState: { power: true, celsius: null, mode: "fan", fanLevel: null, observedAt: null } }));
    expect(fan.rows.map((r) => r.value)).toStrictEqual(["On · fan only"]);
    expect(readingRows(unitDetail({ latestMeasurements: [], observedState: { power: true, celsius: null, mode: null, fanLevel: null, observedAt: null } })).rows.map((r) => r.value)).toStrictEqual(["On"]);
    expect(readingRows(unitDetail({ latestMeasurements: [], observedState: { power: null, celsius: null, mode: null, fanLevel: null, observedAt: null } })).rows).toStrictEqual([]);
  });
});

describe("the history snapshot", () => {
  it("states the latest decision, the work with its day and the report's outcome", () => {
    const declined = snapshotRows(history({ ownDecisionEvents: [decision("offer.accepted", "2026-09-01T00:00:00Z"), decision("offer.declined", "2026-09-02T00:00:00Z"), decision("job.note", "2026-09-03T00:00:00Z")] }));
    expect(declined[0]?.[1]).toMatch(/^Declined · /);
    expect(declined[2]?.[1]).toBe("No report");
    const done = snapshotRows(history({ completedAt: "2026-09-05T00:00:00Z", redactedReportSummary: { hasReport: true, acceptance: "accepted" } }));
    expect([done[0]?.[1], done[1]?.[1].startsWith("completed · "), done[2]?.[1]]).toStrictEqual(["—", true, "Submitted · accepted"]);
    expect(snapshotRows(history({ redactedReportSummary: { hasReport: true, acceptance: "not_accepted" } }))[2]?.[1]).toBe("Submitted · not accepted");
  });
});
