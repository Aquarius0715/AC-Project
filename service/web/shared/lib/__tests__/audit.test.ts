import { describe, expect, it } from "vitest";
import { auditFilters, auditItem, auditQuery, auditRow, periodError, resultWord, roleAtTimeWord, type ApiAudit } from "@ac/web/lib/audit";
import { EN, i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-15T01:00:00Z"); // 09:00 in Kuala Lumpur, 10:00 in Tokyo
const entry = (over: Partial<ApiAudit>): ApiAudit => ({
  id: "a1", actorId: "user-0001", actorName: "hq-operator", actorRoleAtTime: "admin", action: "restrictions.defer", targetRef: { kind: "restriction", id: "r1" }, occurredAt: "2026-09-15T00:30:00Z",
  correlationId: "corr-1", result: "success", maskedBefore: { state: "applied", contactPhone: "***" }, maskedAfter: { state: "applied", "exception.until": "2026-09-20T00:00:00Z", contactPhone: "***" },
  reason: "Invoice under review", ...over,
});

describe("HQ audit log (FR-A16, DD-A16)", () => {
  it("reads the filters from the URL; the default period is 30 days up to tomorrow in the display time zone", () => {
    expect(auditFilters({}, NOW)).toEqual({ from: "2026-08-16", to: "2026-09-16", correlationId: "", result: "All", limit: 25 });
    expect(auditFilters({}, new Date("2026-09-14T15:30:00Z"), "Asia/Tokyo").to).toBe("2026-09-16"); // already the 15th in Tokyo
    expect(auditFilters({ from: "2026-09-01", to: "2026-09-10", corr: "corr-9", result: "Denied", limit: "500" }, NOW)).toEqual({ from: "2026-09-01", to: "2026-09-10", correlationId: "corr-9", result: "Denied", limit: 100 });
    expect([auditFilters({ result: "denied", limit: "1" }, NOW).result, auditFilters({ limit: "1" }, NOW).limit]).toEqual(["All", 25]);
    expect([periodError({ from: "2026-09-10", to: "2026-09-01" }), periodError({ from: "2025-01-01", to: "2026-09-01" }), periodError({ from: "2026-09-01", to: "2026-09-10" })])
      .toEqual(["Period must be start < end, at most 366 days", "Period must be start < end, at most 366 days", undefined]);
  });

  it("asks audit.list for the days of the display time zone", () => {
    const f = auditFilters({ from: "2026-09-01", to: "2026-09-10", result: "Failed", corr: "c" }, NOW);
    expect(auditQuery(f)).toEqual({ limit: 25, filters: { from: "2026-08-31T16:00:00.000Z", to: "2026-09-09T16:00:00.000Z", correlationId: "c", result: "failed" } });
    expect(auditQuery(f, "Asia/Tokyo").filters).toMatchObject({ from: "2026-08-31T15:00:00.000Z", to: "2026-09-09T15:00:00.000Z" });
  });

  it("rows an entry with its times, the recorded role and the masked changes", () => {
    const r = auditRow(entry({}), EN, NOW.getTime());
    expect(r).toMatchObject({ op: "restrictions.defer", target: "restriction · r1", actor: "hq-operator", actorId: "user-0001", role: "Admin", at: "today 8:30 am MYT", occurred: "15 Sept 2026, 8:30 am MYT", res: "Success" });
    expect([auditRow(entry({ actorId: "system-demo", actorName: null, actorRoleAtTime: "system" })).actor]).toEqual(["system-demo"]); // a system actor has no name (IR305)
    expect(r.changes).toEqual([
      { field: "contactPhone", before: "***", after: "***", changed: false }, { field: "exception.until", before: "null", after: "2026-09-20T00:00:00Z", changed: true }, { field: "state", before: "applied", after: "applied", changed: false },
    ]);
    expect(auditRow(entry({ actorRoleAtTime: "partner_bot" })).at).toBe("15 Sept 2026, 8:30 am MYT"); // without now: the full time; an unknown role as recorded
    expect(auditRow(entry({ actorRoleAtTime: "partner_bot" })).role).toBe("partner_bot");
    expect(auditItem(entry({ result: "denied", reason: null }))).toEqual({ time: "15 Sept 2026, 8:30 am MYT", title: "restrictions.defer · Denied", detail: "hq-operator (Admin)" });
  });
});

describe("HQ audit log in Malay with the display time zone (IR304)", () => {
  it("words results, roles and the period check; times in the display zone", () => {
    const t = translator("ms");
    expect(auditRow(entry({ actorRoleAtTime: "technician", result: "pending" }), MS, NOW.getTime())).toMatchObject({ role: "Juruteknik", at: "hari ini 9:30 PG GMT+9", occurred: "15 Sep 2026, 9:30 PG GMT+9", res: "Pending" });
    expect([resultWord("Success", t), resultWord("Pending", t), resultWord("Other", t), roleAtTimeWord("system", t), roleAtTimeWord("client", t)]).toEqual(["Berjaya", "Belum selesai", "Other", "Sistem", "Pelanggan"]);
    expect(periodError({ from: "2026-09-10", to: "2026-09-01" }, t)).toBe("Tempoh mesti mula < tamat, paling lama 366 hari");
    expect(auditItem(entry({}), MS).title).toBe("restrictions.defer · Berjaya");
  });
});
