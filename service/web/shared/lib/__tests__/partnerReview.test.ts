import { describe, expect, it } from "vitest";
import {
  alertRows, availabilityText, dueRow, evidenceCheck, inspectionRows, readingRows, timeOnSiteText, versionRows,
  type ApiInspectionItem, type ApiReviewJob, type ApiWorkReport,
} from "@ac/web/lib/partnerReview";

const item = (over: Partial<ApiInspectionItem> = {}): ApiInspectionItem => ({ id: over.componentKey ?? "x", componentGroup: "indoor", componentKey: "filter", result: "normal", reason: null, evidenceIds: [], authorId: "tech-a", observedAt: "2026-09-14T01:00:00Z", ...over });
const report = (over: Partial<ApiWorkReport> = {}): ApiWorkReport => ({
  id: "rep", version: 2, jobId: "job", authorId: "tech-a", reviewAvailability: { allowed: true, reason: null },
  items: [item({ componentGroup: "electrical", componentKey: "wiring", result: "not_applicable", reason: "No exposed wiring" }), item(), item({ componentGroup: "outdoor", componentKey: "compressor", result: "attention", reason: "Noisy" })],
  measurements: [{ id: "m1", componentKey: "refrigerant_pipe", metric: "refrigerant_pressure", value: 460, unit: "kPa", quality: "valid", observedAt: "2026-09-14T01:00:00Z" }],
  parts: [], refrigerant: [], signOff: null, workText: "Cleaned the coil.", nextAction: { kind: "none" },
  attachmentRefs: [{ id: "a1", name: "p1.png", mime: "image/png", size: 10, status: "ready" }, { id: "a2", name: "p2.png", mime: "image/png", size: 10, status: "processing" }],
  submittedAt: "2026-09-14T01:09:00Z", acceptedAt: null, reviewHistory: [], ...over,
});
const job = (over: Partial<ApiReviewJob> = {}): ApiReviewJob => ({ projection: "detail", id: "job", version: 6, status: "submitted", type: "preventive", reportRefs: [{ reportId: "rep", reportVersion: 2 }], ...over });
const names = new Map([["tech-a", "tech-external-a"], ["me", "you"]]);

describe("inspection rows", () => {
  it("orders indoor → outdoor → electrical and labels the component", () => {
    const rows = inspectionRows(report().items);
    expect(rows.map((r) => r.label)).toEqual(["Indoor unit — filter condition", "Outdoor unit — compressor", "Electrical — wiring insulation"]);
    expect(rows[2]).toMatchObject({ result: "not_applicable", reason: "No exposed wiring" });
    expect(inspectionRows([item({ componentKey: "new_part" })])[0].label).toBe("Indoor unit — new part");
  });
  it("prints readings with their quality", () => {
    expect(readingRows(report().measurements)).toEqual([["Refrigerant pressure (refrigerant)", "460 kPa · recorded"]]);
    expect(readingRows([{ id: "m", metric: "temperature", value: null, unit: "°C", quality: "missing", observedAt: "" }])).toEqual([["Supply air temperature", "no value"]]);
    expect(readingRows([{ id: "m", componentKey: "outlet", metric: "temperature", value: 20, unit: "°C", quality: "suspect", qualityReason: "unit_mismatch", observedAt: "" }])[0][1]).toBe("20 °C · suspect reading (unit mismatch)");
  });
  it("formats the time on site", () => {
    expect(timeOnSiteText(null)).toBe("not recorded");
    expect(timeOnSiteText({ arrivedAt: "2026-09-14T01:05:00Z", finishedAt: null, onSiteMinutes: null })).toBe("09:05 – now");
    expect(timeOnSiteText({ arrivedAt: "2026-09-14T01:05:00Z", finishedAt: "2026-09-14T03:40:00Z", onSiteMinutes: 155 })).toBe("09:05 – 11:40 (2 h 35 m)");
  });
});

describe("evidence check (DD-P05 step 2)", () => {
  it("accepts a complete report and counts only ready photos", () => {
    const c = evidenceCheck(report(), names);
    expect(c.acceptable).toBe(true);
    expect(c.missing).toEqual([]);
    expect(c.rows.map((r) => [r.label, r.value, r.tone])).toEqual([
      ["Required inspection items", "3 / 3 recorded", "ok"], ["Not-applicable reasons", "2 / 2 given", "ok"], ["Photos", "1 attached", "ok"], ["Readings", "1 recorded", "ok"], ["Contributors to v2", "tech-external-a only", "ok"],
    ]);
  });
  it("names what is missing and lists every contributor", () => {
    const r = report({ items: [item({ result: null }), item({ componentKey: "outlet", result: "attention", reason: " " }), item({ componentKey: "louver", authorId: "hq" })], attachmentRefs: [], measurements: [] });
    const c = evidenceCheck(r, names);
    expect(c.acceptable).toBe(false);
    expect(c.missing).toEqual(["1 inspection item without a result", "1 reason missing"]);
    expect(c.rows[0]).toMatchObject({ value: "2 / 3 recorded", tone: "warn" });
    expect(c.rows[1]).toMatchObject({ value: "0 / 1 given", tone: "warn" });
    expect(c.rows[2]).toMatchObject({ value: "0 attached", tone: "muted" });
    expect(c.rows[4].value).toBe("tech-external-a, hq");
    expect(evidenceCheck(report({ items: [item()] }), names).rows[1].value).toBe("none needed");
  });
});

describe("review availability (IR31)", () => {
  it("is silent when allowed and explains each refusal", () => {
    expect(availabilityText({ allowed: true, reason: null }, "submitted")).toBeNull();
    expect(availabilityText({ allowed: false, reason: "self_authored" }, "submitted")).toMatch(/cannot approve or return it \(IR31\)/);
    expect(availabilityText({ allowed: false, reason: "not_current" }, "submitted")).toMatch(/newer version/);
    expect(availabilityText({ allowed: false, reason: "not_submitted" }, "completed")).toMatch(/accepted; the job is completed/);
    expect(availabilityText({ allowed: false, reason: "not_submitted" }, "rework_requested")).toMatch(/returned for rework/);
    expect(availabilityText({ allowed: false, reason: "not_submitted" }, "in_progress")).toMatch(/not been submitted/);
    expect(availabilityText({ allowed: false, reason: "permission_denied" }, "submitted")).toMatch(/partner\.review/);
  });
});

describe("versions & linked alert", () => {
  it("lists each report version with its review, the rework draft and the job due", () => {
    const r = report({ version: 2, reviewHistory: [{ reviewerUserId: "me", reportVersion: 1, decision: "return", reason: "photo missing", occurredAt: "2026-09-14T00:30:00Z" }] });
    const rows = versionRows(job({ reportRefs: [{ reportId: "rep", reportVersion: 1 }, { reportId: "rep", reportVersion: 2 }], draftReportRef: { reportId: "rep", reportVersion: 3 } }), r, "tech-external-a", names);
    expect(rows[0]).toEqual({ title: "v1 · returned 2026-09-14 08:30", sub: "by you — photo missing", badge: { label: "Returned", tone: "warn" } });
    expect(rows[1]).toEqual({ title: "v2 · submitted 2026-09-14 09:09", sub: "by tech-external-a · reviewing now", badge: { label: "Current", tone: "primary" } });
    expect(rows[2]).toEqual({ title: "v3 · draft", sub: "tech-external-a is working on the rework", badge: { label: "Draft", tone: "muted" } });
    const accepted = versionRows(job({ status: "completed" }), report({ reviewHistory: [{ reviewerUserId: "me", reportVersion: 2, decision: "accept", reason: null, occurredAt: "2026-09-14T01:26:00Z" }] }), "tech-external-a", names);
    expect(accepted[0]).toEqual({ title: "v2 · accepted 2026-09-14 09:26", sub: "by you · visible to the customer", badge: { label: "Accepted", tone: "ok" } });
  });
  it("shows alerts and the work window", () => {
    expect(alertRows([{ id: "a", type: "sensor", severity: "critical", status: "open", evidenceText: "Vibration anomaly", detectedAt: "2026-09-13T14:40:00Z" }])[0]).toEqual({ title: "Vibration anomaly (alert)", sub: "09-13 22:40 · stays open after completion", badge: { label: "Open", tone: "crit" } });
    expect(alertRows([{ id: "a", type: "tamper", severity: "warning", status: "acknowledged", evidenceText: "", detectedAt: "2026-09-13T14:40:00Z" }])[0]).toMatchObject({ title: "tamper (alert)", badge: { label: "Acknowledged", tone: "warn" } });
    expect(alertRows([{ id: "a", type: "sensor", severity: "warning", status: "resolved", evidenceText: "x", detectedAt: "2026-09-13T14:40:00Z" }])[0]).toMatchObject({ sub: "09-13 22:40 · resolved", badge: { label: "Resolved", tone: "ok" } });
    expect(dueRow(job({ assignment: { scheduledStart: "2026-09-14T00:00:00Z", scheduledEnd: "2026-09-20T00:00:00Z" } })).sub).toBe("work window 09-14 08:00 → 09-20 08:00");
    expect(dueRow(job({ dueAt: "2026-09-20T00:00:00Z" })).sub).toBe("due 2026-09-20 08:00");
    expect(dueRow(job()).sub).toBe("no window");
  });
});
