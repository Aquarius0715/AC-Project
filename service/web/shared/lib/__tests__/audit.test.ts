import { describe, expect, it } from "vitest";
import {
  actorOptions, auditDeviceEvents, auditFilters, auditItem, auditQuery, auditRow, correlationQuery, correlationTrace, periodError, relatedRecords, resultWord, roleAtTimeWord, targetKinds,
  type ApiAudit, type ApiAuditDeviceEvent,
} from "@ac/web/lib/audit";
import { EN, i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-15T01:00:00Z"); // 09:00 in Kuala Lumpur, 10:00 in Tokyo
const entry = (over: Partial<ApiAudit>): ApiAudit => ({
  id: "a1", actorId: "user-0001", actorName: "hq-operator", actorRoleAtTime: "admin", action: "restrictions.defer", targetRef: { kind: "restriction", id: "r1" }, previousVersion: 3, nextVersion: 4,
  occurredAt: "2026-09-15T00:30:00Z", correlationId: "corr-1", result: "success", maskedBefore: { state: "applied", contactPhone: "***" }, maskedAfter: { state: "applied", "exception.until": "2026-09-20T00:00:00Z", contactPhone: "***" },
  reason: "Invoice under review", ...over,
});
const event = (over: Partial<ApiAuditDeviceEvent>): ApiAuditDeviceEvent => ({
  id: "e42", version: 1, deviceId: "d1", eventType: "tamper", evidenceSource: "tamper_signal", sequence: 42, occurredAt: "2026-09-15T00:41:00Z", restoredAt: null, alertIds: ["al1"], recovery: null,
  responseNotes: [{ actorId: "user-0001", message: "Customer confirms nobody touched the unit.", at: "2026-09-15T00:50:00Z" }], unitIdAtOccurrence: "u1", ...over,
});

describe("HQ audit log (FR-A16, DD-A16, SCR-A16)", () => {
  it("reads the filters from the URL; the default period is 30 days up to tomorrow in the display time zone", () => {
    expect(auditFilters({}, NOW)).toEqual({ from: "2026-08-16", to: "2026-09-16", actorId: "", targetKind: "", targetId: "", correlationId: "", result: "All", limit: 25 });
    expect(auditFilters({}, new Date("2026-09-14T15:30:00Z"), "Asia/Tokyo").to).toBe("2026-09-16"); // already the 15th in Tokyo
    expect(auditFilters({ from: "2026-09-01", to: "2026-09-10", actorId: "u9", targetKind: "job", targetId: "j1", correlationId: "corr-9", result: "Denied", limit: "500" }, NOW))
      .toEqual({ from: "2026-09-01", to: "2026-09-10", actorId: "u9", targetKind: "job", targetId: "j1", correlationId: "corr-9", result: "Denied", limit: 100 });
    expect([auditFilters({ result: "denied", limit: "1" }, NOW).result, auditFilters({ limit: "1" }, NOW).limit, auditFilters({ corr: "old-key" }, NOW).correlationId]).toEqual(["All", 25, ""]);
    expect([periodError({ from: "2026-09-10", to: "2026-09-01" }), periodError({ from: "2025-01-01", to: "2026-09-01" }), periodError({ from: "2026-09-01", to: "2026-09-10" })])
      .toEqual(["Period must be start < end, at most 366 days", "Period must be start < end, at most 366 days", undefined]);
  });

  it("asks audit.list for the days of the display time zone with the actor, target and correlation filters", () => {
    const f = auditFilters({ from: "2026-09-01", to: "2026-09-10", result: "Failed", correlationId: "c", actorId: "u9", targetKind: "job", targetId: "j1" }, NOW);
    expect(auditQuery(f)).toEqual({ limit: 25, filters: { from: "2026-08-31T16:00:00.000Z", to: "2026-09-09T16:00:00.000Z", actorId: "u9", targetKind: "job", targetId: "j1", correlationId: "c", result: "failed" } });
    expect(auditQuery(f, "Asia/Tokyo").filters).toMatchObject({ from: "2026-08-31T15:00:00.000Z", to: "2026-09-09T15:00:00.000Z" });
    expect(auditQuery(auditFilters({}, NOW)).filters).toEqual({ from: "2026-08-15T16:00:00.000Z", to: "2026-09-15T16:00:00.000Z" }); // no empty keys
    // the entry's correlation: a day either side, oldest first
    expect(correlationQuery(entry({}))).toEqual({ limit: 100, sort: { field: "occurredAt", direction: "asc" }, filters: { from: "2026-09-14T00:30:00.000Z", to: "2026-09-16T00:30:00.000Z", correlationId: "corr-1" } });
  });

  it("rows an entry with its times, the recorded role, the versions and the masked changes", () => {
    const r = auditRow(entry({}), EN, NOW.getTime());
    expect(r).toMatchObject({ op: "restrictions.defer", target: "restriction · r1", targetKind: "restriction", targetId: "r1", versions: "version 3 → 4", actor: "hq-operator", actorId: "user-0001", role: "Admin", at: "today 8:30 am MYT", occurred: "15 Sept 2026, 8:30 am MYT", res: "Success" });
    expect([auditRow(entry({ previousVersion: null, nextVersion: 1 })).versions, auditRow(entry({ previousVersion: null, nextVersion: null })).versions]).toEqual(["version 1", null]);
    expect([auditRow(entry({ actorId: "system-demo", actorName: null, actorRoleAtTime: "system" })).actor]).toEqual(["system-demo"]); // a system actor has no name (IR305)
    expect(r.changes).toEqual([
      { field: "contactPhone", before: "***", after: "***", changed: false }, { field: "exception.until", before: "null", after: "2026-09-20T00:00:00Z", changed: true }, { field: "state", before: "applied", after: "applied", changed: false },
    ]);
    expect(auditRow(entry({ actorRoleAtTime: "partner_bot" })).at).toBe("15 Sept 2026, 8:30 am MYT"); // without now: the full time; an unknown role as recorded
    expect(auditRow(entry({ actorRoleAtTime: "partner_bot" })).role).toBe("partner_bot");
    expect(auditItem(entry({ result: "denied", reason: null }))).toEqual({ time: "15 Sept 2026, 8:30 am MYT", title: "restrictions.defer · Denied", detail: "hq-operator (Admin)" });
  });

  it("offers actors by name and the business target kinds with the shown ones", () => {
    const rows = [auditRow(entry({})), auditRow(entry({ id: "a2", actorId: "system-demo", actorName: null, targetRef: { kind: "filter_care_settings", id: "f1" } }))];
    expect(actorOptions([{ userId: "user-0001", displayName: "hq-operator" }, { userId: "user-0002", displayName: "contractor-a" }], rows, "user-9999")).toEqual([
      { id: "user-0002", name: "contractor-a" }, { id: "user-0001", name: "hq-operator" }, { id: "system-demo", name: "system-demo" }, { id: "user-9999", name: "user-9999" },
    ]);
    const kinds = targetKinds(rows, "zz_kind");
    expect([kinds.includes("restriction"), kinds.includes("command"), kinds.includes("filter_care_settings"), kinds.at(-1), kinds.length]).toEqual([true, true, true, "zz_kind", 16]);
  });

  it("finds the related restriction, command, job and unit; the trace keeps every entry of the correlation", () => {
    const sel = entry({});
    const cmd = entry({ id: "a2", action: "commands.create", targetRef: { kind: "command", id: "c1" }, occurredAt: "2026-09-15T00:30:05Z", result: "pending" });
    const fin = entry({ id: "a3", action: "commands.create", targetRef: { kind: "command", id: "c1" }, occurredAt: "2026-09-15T00:30:09Z", actorName: null, actorId: "system-demo", actorRoleAtTime: "system", reason: null });
    expect(relatedRecords(sel, [fin, cmd, sel])).toEqual({
      restriction: { id: "r1", via: "target", note: "Applied · exception until 20 Sept 2026, 8:00 am MYT — as this entry recorded it" },
      command: { id: "c1", via: "correlation", note: "same correlation ID" }, job: null, unit: null,
    });
    expect(relatedRecords(entry({ maskedAfter: {}, targetRef: { kind: "job", id: "j1" } }), [entry({ id: "x", targetRef: { kind: "unit", id: "u1" } })])).toEqual({
      restriction: null, command: null, job: { id: "j1", via: "target", note: "this entry’s target" }, unit: { id: "u1", via: "correlation", note: "same correlation ID" },
    });
    expect(relatedRecords(entry({ maskedAfter: {} }), []).restriction?.note).toBe("this entry’s target"); // nothing recorded: no state
    expect(correlationTrace([fin, cmd, sel]).map((x) => [x.id, x.title])).toEqual([["a1", "restrictions.defer · Success"], ["a2", "commands.create · Pending"], ["a3", "commands.create · Success"]]);
    expect([correlationTrace([sel]), correlationTrace([])]).toEqual([[], []]); // an entry on its own has no trace
  });

  it("words the device tab's events: sequence gaps, the unit at the time, alerts and notes", () => {
    const names = new Map([["user-0001", "hq-operator"]]), units = new Map([["u1", "Bedroom AC"]]);
    const alerts = new Map([["al1", { id: "al1", version: 1, status: "acknowledged" as const, type: "tamper", severity: "warning" }]]);
    const back = event({ id: "e41", eventType: "restored", evidenceSource: "heartbeat", sequence: 40, occurredAt: "2026-09-14T22:00:00Z", alertIds: [], responseNotes: [], recovery: { axis: "connection", sourceEventId: "e39", value: "online" } });
    const [t, r] = auditDeviceEvents([event({}), back], alerts, names, units, NOW.getTime());
    expect(t).toMatchObject({ title: "Tamper · removal suspected", type: "tamper", tone: "warn", meta: "tamper_signal · seq 42 · today 8:41 am MYT", badge: { text: "Not restored", tone: "crit" } });
    expect(Object.fromEntries(t.facts)).toEqual({
      "Event type": "tamper", Evidence: "tamper_signal — Cover opened while powered", Sequence: "42 (previous 40 — 1 missing)", "Unit at the time": "Bedroom AC", Occurred: "15 Sept 2026, 8:41 am MYT",
      "Restored at": "— not yet", Recovery: "open · acknowledged",
    });
    expect(t.alerts).toEqual([{ id: "al1", text: "tamper · acknowledged", href: "/admin/alerts?alertId=al1&status=acknowledged" }]);
    expect(t.notes).toEqual([{ text: "Customer confirms nobody touched the unit.", by: "hq-operator · 15 Sept 2026, 8:50 am MYT" }]);
    expect([r.badge, Object.fromEntries(r.facts).Sequence, Object.fromEntries(r.facts)["Restored at"], Object.fromEntries(r.facts).Evidence]).toEqual([null, "40", undefined, "heartbeat — Connection restored"]);
    const [gapless] = auditDeviceEvents([event({}), event({ id: "e41", sequence: 41 })], new Map(), new Map(), new Map(), NOW.getTime());
    expect([Object.fromEntries(gapless.facts).Sequence, gapless.alerts[0].href, gapless.notes[0].by.startsWith("user-000 · ")]).toEqual(["42 (previous 41 — no gap)", "/admin/alerts?alertId=al1", true]);
  });
});

describe("HQ audit log in Malay with the display time zone (IR304)", () => {
  it("words results, roles and the period check; times in the display zone", () => {
    const t = translator("ms");
    expect(auditRow(entry({ actorRoleAtTime: "technician", result: "pending" }), MS, NOW.getTime())).toMatchObject({ role: "Juruteknik", at: "hari ini 9:30 PG GMT+9", occurred: "15 Sep 2026, 9:30 PG GMT+9", res: "Pending", versions: "versi 3 → 4" });
    expect([resultWord("Success", t), resultWord("Pending", t), resultWord("Other", t), roleAtTimeWord("system", t), roleAtTimeWord("client", t)]).toEqual(["Berjaya", "Belum selesai", "Other", "Sistem", "Pelanggan"]);
    expect(periodError({ from: "2026-09-10", to: "2026-09-01" }, t)).toBe("Tempoh mesti mula < tamat, paling lama 366 hari");
    expect(auditItem(entry({}), MS).title).toBe("restrictions.defer · Berjaya");
    expect(relatedRecords(entry({}), [], MS).restriction?.note).toBe("Dikenakan · pengecualian hingga 20 Sep 2026, 9:00 PG GMT+9 — seperti direkodkan oleh entri ini");
    const [e] = auditDeviceEvents([event({})], new Map(), new Map(), new Map(), NOW.getTime(), MS);
    expect([e.title, e.badge?.text, Object.fromEntries(e.facts)["Dipulihkan pada"]]).toEqual(["Gangguan · penanggalan disyaki", "Tidak dipulihkan", "— belum lagi"]);
  });
});
