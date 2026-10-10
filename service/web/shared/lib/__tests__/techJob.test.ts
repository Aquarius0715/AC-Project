import { describe, expect, it } from "vitest";
import { componentsFor, draftFrom, draftInput, fmtOf, followUpText, groupOf, historyCard, knownOf, progress, submitIssues, timeRows, versionRows, windowState, type ApiTechReport, type Draft } from "@ac/web/lib/techJob";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });

const NOW = Date.parse("2026-09-14T03:00:00Z"); // 11:00 in Kuala Lumpur
const all = componentsFor(["indoor", "outdoor", "electrical"]);
const report = (over: Partial<ApiTechReport> = {}): ApiTechReport => ({
  id: "rep", version: 3, jobId: "job", authorId: "tech", items: [
    { id: "i1", componentGroup: "indoor", componentKey: "filter", result: "attention", reason: "dusty", evidenceIds: [], authorId: "tech", observedAt: "2026-09-14T02:00:00Z" },
    { id: "i2", componentGroup: "electrical", componentKey: "wiring", result: "not_applicable", reason: "no wiring", evidenceIds: [], authorId: "tech", observedAt: "2026-09-14T02:00:00Z" },
  ],
  measurements: [{ id: "m1", componentKey: "outlet", metric: "temperature", value: 13.9, unit: "°C", observedAt: "2026-09-14T02:10:00Z", quality: "valid" }],
  parts: [], refrigerant: [], signOff: null, workText: "Cleaned the filter.", nextAction: { kind: "none" },
  attachmentRefs: [{ id: "a1", name: "p1.png", mime: "image/png", size: 10, status: "ready" }, { id: "a2", name: "p2.heic", mime: "image/png", size: 10, status: "failed" }],
  submittedAt: null, acceptedAt: null, reviewHistory: [], ...over,
});

describe("components", () => {
  it("follows the unit's service scope in canonical order", () => {
    expect(all).toHaveLength(18);
    expect(componentsFor(["indoor"]).map((c) => c.key)).toEqual(["filter", "evaporator_coil", "blower_motor", "blower_fan", "drain_pipe", "drain_pan", "outlet", "louver"]);
    expect(componentsFor(["electrical", "outdoor"]).map((c) => c.group)).toEqual([...Array(5).fill("outdoor"), ...Array(5).fill("electrical")]);
    expect(componentsFor([])).toEqual([]);
    expect(groupOf("compressor")).toBe("outdoor");
    expect(groupOf("nope")).toBeNull();
  });
});

describe("draft", () => {
  it("starts every component without a result and keeps the saved sections", () => {
    const d = draftFrom(report(), all);
    expect(d.items).toHaveLength(18);
    expect(d.items.find((i) => i.componentKey === "filter")).toMatchObject({ result: "attention", reason: "dusty" });
    expect(d.items.find((i) => i.componentKey === "louver")).toMatchObject({ result: null, reason: "" });
    expect(d.readings).toEqual([{ id: "m1", componentKey: "outlet", metric: "temperature", value: "13.9", observedAt: "2026-09-14T02:10:00Z" }]);
    expect(d.attachmentIds).toEqual(["a1"]); // a failed upload is not part of the draft
    expect(draftFrom(null, componentsFor(["indoor"])).items.every((i) => i.result === null)).toBe(true);
  });
  it("builds the saveDraft body with trimmed reasons and numeric readings", () => {
    const d = draftFrom(report(), all);
    d.items[1] = { ...d.items[1], result: "normal", reason: "   " };
    d.readings.push({ componentKey: "compressor", metric: "vibration", value: "", observedAt: "2026-09-14T02:20:00Z" });
    d.nextAction = { kind: "follow_up", date: "2026-10-01T00:00:00.000Z", note: "  replace filter  " };
    const body = draftInput("job", "rep", d);
    expect(body.reportId).toBe("rep");
    expect(body.items[1]).toEqual({ componentGroup: "indoor", componentKey: "evaporator_coil", result: "normal", reason: null, evidenceIds: [] });
    expect(body.measurements).toEqual([
      { id: "m1", componentKey: "outlet", metric: "temperature", value: 13.9, unit: "°C", observedAt: "2026-09-14T02:10:00Z" },
      { componentKey: "compressor", metric: "vibration", value: null, unit: "mm/s", observedAt: "2026-09-14T02:20:00Z" },
    ]);
    expect(body.nextAction).toEqual({ kind: "follow_up", date: "2026-10-01T00:00:00.000Z", note: "replace filter" });
    expect("reportId" in draftInput("job", null, d)).toBe(false);
  });
});

describe("submit checks", () => {
  const complete = (): Draft => {
    const d = draftFrom(report(), all);
    d.items = d.items.map((i) => (i.result === null ? { ...i, result: "normal" } : i));
    d.workText = "Cleaned the filter and checked the coil.";
    return d;
  };
  it("passes a complete draft", () => expect(submitIssues(complete(), report().attachmentRefs, NOW)).toEqual([]));
  it("lists every blocking issue with its group", () => {
    const d = complete();
    d.items[7] = { ...d.items[7], result: null };
    d.items[4] = { ...d.items[4], result: "not_inspected", reason: "" };
    d.readings.push({ componentKey: "fan", metric: "vibration", value: "abc", observedAt: "2026-09-14T02:00:00Z" });
    d.workText = "short";
    d.nextAction = { kind: "follow_up", date: "2026-09-01T00:00:00Z", note: "" };
    d.parts = [{ name: "Filter", quantity: 0, catalogCode: null, source: "van_stock", lotSerial: null, replacesComponentKey: null, oldPartDisposal: null, receiptAttachmentId: null }];
    d.attachmentIds = ["a1", "a2"];
    const issues = submitIssues(d, report().attachmentRefs, NOW);
    expect(issues.map((i) => i.text)).toEqual([
      "Indoor › Drain pipe needs a reason", "Indoor › Louver has no result", "Fan reading is not a number", "Work performed must be 10–4000 characters",
      "A follow-up needs a future date and a note", "Part 1 (Filter) needs a quantity of 1–999", "Photo p2.heic is failed — remove or retry it",
    ]);
    expect(progress(d, issues).map((g) => [g.group, g.done, g.total, g.issues])).toEqual([["indoor", 7, 8, 2], ["outdoor", 5, 5, 1], ["electrical", 5, 5, 0]]);
  });
});

describe("work window (IR76 / IR89)", () => {
  const a = { scheduledStart: "2026-09-14T02:00:00Z", scheduledEnd: "2026-09-14T04:00:00Z" }; // 10:00–12:00
  it("tells before, open, ending and ended apart", () => {
    expect(windowState(null, NOW).phase).toBe("none");
    expect(windowState(a, Date.parse("2026-09-14T01:00:00Z"))).toEqual({ phase: "before", text: "Starts 14 Sept 2026, 10:00 am MYT · 14 Sept, 10:00 am – 12:00 pm MYT" });
    expect(windowState(a, NOW)).toEqual({ phase: "open", text: "14 Sept, 10:00 am – 12:00 pm MYT" });
    expect(windowState(a, Date.parse("2026-09-14T03:46:00Z"))).toEqual({ phase: "ending", text: "Ends in 14 min — unsaved input is discarded at 12:00 pm MYT (IR89)" });
    expect(windowState(a, Date.parse("2026-09-14T04:00:00Z")).phase).toBe("ended");
  });
});

describe("time on site and versions", () => {
  it("shows the check-in evidence, pauses and the counted minutes", () => {
    expect(timeRows(null, NOW)).toEqual([["Arrived", "not checked in"]]);
    const rows = timeRows({ arrivedAt: "2026-09-14T02:05:00Z", checkInMethod: "location_qr", distanceMeters: 78.4, startedAt: "2026-09-14T02:05:00Z", pauses: [{ from: "2026-09-14T02:30:00Z", to: "2026-09-14T02:40:00Z" }], finishedAt: null }, NOW);
    expect(rows).toEqual([["Arrived", "10:05 am MYT · checked in"], ["Location", "78 m from site · QR matched"], ["Started", "10:05 am MYT"], ["Paused", "1 pause · 10 min"], ["Finished", "— (on submit)"], ["On-site time", "45 min"]]);
    const paused = timeRows({ arrivedAt: "2026-09-14T02:05:00Z", checkInMethod: "manual", checkInReason: "no GPS", startedAt: "2026-09-14T02:05:00Z", pauses: [{ from: "2026-09-14T02:50:00Z", to: null }], finishedAt: null }, NOW);
    expect(paused[1]).toEqual(["Location", "manual — no GPS"]);
    expect(paused[3]).toEqual(["Paused", "since 10:50 am MYT"]);
    expect(paused[5]).toEqual(["On-site time", "45 min"]);
    expect(timeRows({ arrivedAt: "2026-09-14T02:05:00Z", startedAt: "2026-09-14T02:05:00Z", finishedAt: "2026-09-14T04:00:00Z", onSiteMinutes: 115, pauses: [] }, NOW)[5]).toEqual(["On-site time", "1 h 55 min"]);
  });
  it("lists the draft and the submitted versions with their review", () => {
    const reviews = report({ reviewHistory: [{ reviewerUserId: "c", reportVersion: 2, decision: "return", reason: "photo missing", occurredAt: "2026-09-14T02:30:00Z" }] }).reviewHistory;
    expect(versionRows({ version: 3, results: 2, photos: 1, savedAt: "2026-09-14T02:44:00Z" }, [{ reportVersion: 2 }], reviews)).toEqual([
      { title: "Draft v3 · saved 10:44 am MYT", sub: "2 results · 1 photo", badge: { text: "Draft", tone: "warn" } },
      { title: "v2 · returned", sub: "14 Sept 2026, 10:30 am MYT — photo missing", badge: { text: "Returned", tone: "warn" } },
    ]);
    // a first save made the draft without a reload: no saved time yet from the page, one result
    expect(versionRows({ version: 1, results: 1, photos: 0, savedAt: null }, [], [])).toEqual([{ title: "Draft v1", sub: "1 result · 0 photos", badge: { text: "Draft", tone: "warn" } }]);
    expect(versionRows(null, [{ reportVersion: 1 }], [])).toEqual([{ title: "v1 · submitted", sub: "awaiting quality review", badge: { text: "Submitted", tone: "primary" } }]);
  });
});

describe("the server's formatting and Malay (IR282)", () => {
  const a = { scheduledStart: "2026-09-14T02:00:00Z", scheduledEnd: "2026-09-14T04:00:00Z" };
  it("uses the instants the loader formatted and the browser only for new ones", () => {
    const known = knownOf(MS, ["2026-09-14T04:00:00Z"], ["2026-09-14T02:00:00Z"], [[a.scheduledStart, a.scheduledEnd]]);
    expect(known).toEqual({ clock: { "2026-09-14T04:00:00Z": "1:00 PTG GMT+9" }, stamp: { "2026-09-14T02:00:00Z": "14 Sep 2026, 11:00 PG GMT+9" }, span: { "2026-09-14T02:00:00Z|2026-09-14T04:00:00Z": "14 Sep, 11:00 PG – 1:00 PTG GMT+9" } });
    const f = fmtOf(MS, { clock: { "2026-09-14T04:00:00Z": "SERVER" }, stamp: {}, span: {} }); // a known instant is never formatted again
    expect([f.clock("2026-09-14T04:00:00Z"), f.clock("2026-09-14T05:00:00Z")]).toEqual(["SERVER", "2:00 PTG GMT+9"]);
    expect(windowState(a, Date.parse("2026-09-14T03:46:00Z"), fmtOf(MS, known)).text).toBe("Tamat dalam 14 min — input yang belum disimpan dibuang pada 1:00 PTG GMT+9 (IR89)");
  });
  it("words the checks, time on site, versions and the history in Malay", () => {
    const ms = translator("ms");
    const d = draftFrom(null, componentsFor(["indoor"]));
    expect(submitIssues(d, [], NOW, ms)[0].text).toBe("Dalaman › Penapis tiada keputusan");
    expect(progress(d, [], ms)[0].label).toBe("Dalaman");
    expect(timeRows(null, NOW, fmtOf(MS))).toEqual([["Tiba", "belum daftar masuk"]]);
    expect(versionRows(null, [{ reportVersion: 1 }], [], fmtOf(MS))).toEqual([{ title: "v1 · dihantar", sub: "menunggu semakan kualiti", badge: { text: "Dihantar", tone: "primary" } }]);
    expect(followUpText({ kind: "follow_up", date: "2026-09-25T01:00:00Z", note: "Replace filter" }, MS)).toBe("Susulan 25 Sep 2026 — Replace filter");
    expect(historyCard({ projection: "history", jobId: "job-9-aaaa", type: "periodic", status: "completed", asOf: "2026-09-21T08:00:00Z", completedAt: "2026-09-21T07:00:00Z", redactedReportSummary: { hasReport: true, acceptance: "accepted" } }, MS)).toEqual({
      title: "job-9-aa · Pemeriksaan berkala", sub: "Selesai — tugasan anda tamat bersama kerja",
      rows: [["Status", "Selesai 21 Sep 2026"], ["Laporan anda", "Diterima dalam semakan kualiti"], ["Tugasan tamat", "21 Sep 2026, 5:00 PTG GMT+9"]],
      note: "Masa anda lapang untuk kerja lain. Laporan, unit dan perantinya kekal dengan HQ dan pelanggan.", back: "← Gambaran keseluruhan",
    });
  });
});
