import { describe, expect, it } from "vitest";
import { chart, chartMetrics, contextJob, evidenceRows, jobContext, pastWork, readingRows, registerRows, snapshotRows, type ApiUnitJob } from "@ac/web/lib/partnerUnit";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import type { ApiAlert } from "@ac/web/lib/alerts";
import type { ApiHistory } from "@ac/web/lib/partnerJobDetail";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = Date.parse("2026-09-14T01:35:00Z"); // 09:35 in Kuala Lumpur
const m = (metric: string, value: number | null, unit: string, quality: "valid" | "stale" | "suspect" | "missing" = "valid") =>
  ({ id: metric, unitId: "u1", sensorId: "s", metric, value, unit, observedAt: "2026-09-14T01:35:00Z", origin: "measured" as const, quality, qualityReason: null });
const unit = {
  id: "unit-p-rooftop", displayName: "Rooftop unit", connection: "online", lastSeenAt: "2026-09-14T01:35:00Z", serviceScope: ["indoor", "outdoor"],
  location: { pathLabels: ["customer-b", "Tower A rooftop"], address: null, accessInstructions: null }, capabilities: { manufacturer: "cap-split-std", model: "v2" },
  observedState: { power: true, celsius: 22, mode: "cool", fanLevel: "mid", observedAt: "2026-09-14T01:35:00Z" },
  latestMeasurements: [m("temperature", 14.2, "°C"), m("refrigerant_pressure", 310, "kPa"), m("vibration", 2.1, "mm/s", "suspect")],
} as unknown as ApiUnitDetail;
const job: ApiUnitJob = { projection: "summary", id: "job-p02-aaaa", type: "reactive", status: "accepted", scheduledSlot: null, technicianMembershipId: null, accessValidFrom: "2026-09-13T17:00:00Z", accessValidUntil: "2026-09-14T16:00:00Z" };

describe("partner unit view", () => {
  it("states the register, the job context and the past work", () => {
    expect(registerRows(unit)).toEqual({
      connection: "online", lastSeen: "14 Sept 2026, 9:35 am MYT",
      rows: [["Location", "customer-b › Tower A rooftop"], ["Model", "cap-split-std v2"], ["Maintenance scope", "Indoor unit · Outdoor unit"]],
    });
    const other: ApiUnitJob = { ...job, id: "job-p09-bbbb" };
    expect(contextJob([other, job], "job-p02-aaaa")).toBe(job);
    expect(contextJob([other, job], "nope")).toBe(other);
    expect(contextJob([{ projection: "history", jobId: "h", type: "periodic", status: "completed", completedAt: null }])).toBeNull();
    expect(jobContext(job as Extract<ApiUnitJob, { projection: "summary" }>, new Map(), NOW)).toEqual({
      id: "job-p02-aaaa", techMissing: true, pct: 37,
      rows: [["Job", "job-p02- · Repair"], ["Technician", "Not assigned yet"], ["Access window", "14 Sept, 1:00 am – 15 Sept, 12:00 am MYT"]],
      note: "Access ends in 14 h. After that, live values are cleared and only a history snapshot remains.",
    });
    expect(pastWork([job, { projection: "history", jobId: "job-c01-cccc", type: "preventive", status: "completed", completedAt: "2026-06-12T04:00:00Z" }])).toEqual([
      { id: "job-c01-cccc", title: "job-c01- · Preventive maintenance", sub: "12 Jun 2026", badge: { text: "completed", tone: "ok" } },
    ]);
  });

  it("shows the alert evidence, the readings and a 24-hour chart", () => {
    const alerts = [{ id: "a1", unitId: "u1", type: "fault", severity: "warning", status: "open", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "amplitude 2.1x baseline", detectedAt: "2026-09-13T14:40:00Z" }] as ApiAlert[];
    expect(evidenceRows(alerts)).toEqual([{ id: "a1", title: "Alert", severity: "warning", resolved: false, sub: "Suspected — amplitude 2.1x baseline · 13 Sept 2026, 10:40 pm MYT" }]);
    expect(readingRows(unit)).toEqual({
      observed: "14 Sept 2026, 9:35 am MYT",
      rows: [{ label: "Temperature", value: "14.2 °C · good", tone: undefined }, { label: "Refrigerant pressure", value: "310 kPa · good", tone: undefined }, { label: "Vibration", value: "2.1 mm/s · suspect", tone: "crit" }, { label: "Power state", value: "On · cooling", tone: undefined }],
    });
    expect(chartMetrics(unit)).toEqual(["vibration", "refrigerant_pressure"]);
    const c = chart("vibration", "mm/s", [{ value: 1, observedAt: "2026-09-13T03:00:00Z" }, { value: 2.1, observedAt: "2026-09-14T01:00:00Z" }, { value: 9, observedAt: "2026-09-14T01:10:00Z", quality: "suspect" }], NOW);
    expect([c.title, c.now, c.labels.length, c.values[0], c.values[11], c.labels[0]]).toEqual(["Vibration (mm/s)", "now 2.1", 12, 1, 2.1, "9:35 am"]);
    expect(chart("humidity", "%", [], NOW).now).toBe("no reading");
  });

  it("keeps only the company's own decisions after the delegation", () => {
    const h: ApiHistory = { projection: "history", jobId: "job-p02-aaaa", type: "reactive", status: "completed", asOf: "2026-09-21T16:00:00Z", completedAt: "2026-09-21T08:00:00Z",
      ownDecisionEvents: [{ id: "e", jobId: "job-p02-aaaa", actorUserId: "u", action: "offer.accepted", occurredAt: "2026-09-14T02:02:00Z" }], redactedReportSummary: { hasReport: true, acceptance: "accepted" } };
    expect(snapshotRows(h)).toEqual([
      ["Your decision", "Accepted · 14 Sept 2026, 10:02 am MYT"], ["Work", "completed · 21 Sept 2026"], ["Report", "Submitted · accepted"], ["Live unit values", "Not available after the period"],
    ]);
    expect(snapshotRows({ ...h, ownDecisionEvents: [], redactedReportSummary: { hasReport: false, acceptance: "not_accepted" } }).slice(0, 3).map((r) => r[1])).toEqual(["—", "completed · 21 Sept 2026", "No report"]);
  });

  it("words the unit view in Malay with the display time zone (IR280)", () => {
    expect(registerRows(unit, MS).rows[2]).toEqual(["Skop penyelenggaraan", "Unit dalaman · Unit luaran"]);
    expect(jobContext(job as Extract<ApiUnitJob, { projection: "summary" }>, new Map(), NOW, MS).rows[1]).toEqual(["Juruteknik", "Belum ditugaskan"]);
    expect(readingRows(unit, MS).rows.map((r) => r.value)).toEqual(["14.2 °C · baik", "310 kPa · baik", "2.1 mm/s · diragui", "Hidup · menyejuk"]);
    expect(chart("vibration", "mm/s", [], NOW, MS)).toMatchObject({ title: "Getaran (mm/s)", now: "tiada bacaan" });
  });
});
