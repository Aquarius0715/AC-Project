import { describe, expect, it } from "vitest";
import { inboxAlerts, type ApiAlert } from "@ac/web/lib/alerts";
import { inboxRow, type ApiNotification } from "@ac/web/lib/notifications";
import { i18nOf } from "@ac/web/lib/i18n";

const note = (over: Partial<ApiNotification>): ApiNotification => ({
  id: "n1", version: 1, type: "job_update", sourceAlertId: null, target: { kind: "job", id: "job-1" }, templateKey: "job_update",
  params: { targetName: "job-1", at: "2026-09-14T01:00:00Z", status: "requested", reason: null, message: null }, channel: "inApp", occurredAt: "2026-09-14T01:00:00Z", readAt: null, ...over,
});

describe("inbox rows and alert rows", () => {
  it("titles by template, links the target and hides the in-app channel", () => {
    const r = inboxRow(note({}), "client");
    expect([r.t, r.d, r.href, r.read]).toEqual(["Job update — job-1", "requested", "/customer/maintenance?jobId=job-1", false]);
    expect(inboxRow(note({ channel: "email" }), "client").d).toBe("requested · email");
    expect(inboxRow(note({ type: "fault", templateKey: "alert", sourceAlertId: "a1", target: { kind: "unit", id: "u1" }, params: { ...note({}).params, targetName: "Bedroom AC", status: "open" } }), "client"))
      .toMatchObject({ t: "Alert on Bedroom AC", href: "/customer/alerts" });
  });

  it("titles and times a row in the user's display language and time zone (FR-X01, IR258)", () => {
    expect(inboxRow(note({}), "client").w).toBe("14 Sept 2026, 9:00 am MYT");
    const r = inboxRow(note({}), "client", { locale: "ms", timeZone: "Asia/Tokyo" });
    expect([r.t, r.d, r.w]).toEqual(["Kemas kini kerja — job-1", "requested", "14 Sep 2026, 10:00 PG GMT+9"]);
    expect(inboxRow(note({ templateKey: "unknown" }), "technician", { locale: "ms", timeZone: "UTC" }).t).toBe("Pemberitahuan — job-1");
  });

  it("opens Filter care from a cleaning reminder (IR239)", () => {
    const r = inboxRow(note({ type: "cleaning_due", templateKey: "alert", sourceAlertId: "a1", target: { kind: "unit", id: "u1" },
      params: { targetName: "Bedroom AC", at: "2026-09-14T01:00:00Z", status: "open", reason: null, message: "Cleaning due (268 h of run time since the last cleaning). Not a fault." } }), "client");
    expect([r.t, r.d, r.href]).toEqual(["Filter cleaning due on Bedroom AC", "Cleaning due (268 h of run time since the last cleaning). Not a fault. · open", "/customer/maintenance?tab=filter-care"]);
  });

  it("lists alerts with their status and the read state of their notifications (DD-C08, IR242)", () => {
    const alert = (id: string, severity: ApiAlert["severity"], status: string, over: Partial<ApiAlert> = {}): ApiAlert => ({
      id, unitId: "u1", type: "sensor", severity, status, causeCode: "unknown", evidenceKind: "inferred", evidenceText: "Cleaning due (250 h of run time since the last cleaning). Not a fault.", detectedAt: "2026-09-14T01:00:00Z", ...over,
    });
    const rows = inboxAlerts([
      alert("rem", "normal", "open", { type: "maintenance" }), alert("old", "critical", "resolved", { resolvedAt: "2026-09-14T02:00:00Z", resolutionReason: "filter cleaned" }),
      alert("win", "warning", "acknowledged", { causeCode: "window_open", acknowledgedAt: "2026-09-14T01:30:00Z", detectedAt: "2026-09-14T00:30:00Z" }), alert("hot", "critical", "open"),
    ], [{ id: "n1", version: 1, sourceAlertId: "win", readAt: null }, { id: "n2", version: 2, sourceAlertId: "hot", readAt: "2026-09-14T01:05:00Z" }, { id: "n3", version: 1, sourceAlertId: null, readAt: null }],
    () => ({ name: "Bedroom AC", place: "Home A › Bedroom" }));
    expect(rows.map((r) => [r.id, r.group, r.status.text, r.unread.length, r.notes])).toEqual([["hot", "attn", "Unresolved", 0, 1], ["win", "attn", "Acknowledged", 1, 1], ["old", "info", "Resolved", 0, 0], ["rem", "info", "Unresolved", 0, 0]]);
    expect([rows[3].title, rows[3].kind, rows[3].status.tone, rows[2].status.detail, rows[1].unread, rows[0].where])
      .toEqual(["Filter cleaning reminder", "◷ Maintenance reminder", "muted", "Resolved 14 Sept 2026, 10:00 am MYT · filter cleaned", [{ id: "n1", version: 1 }], "Bedroom AC · Home A › Bedroom · detected 14 Sept 2026, 9:00 am MYT"]);
    expect(rows.map((r) => r.status.state)).toEqual(["open", "acknowledged", "resolved", "open"]);
    // a load cause is only suspected, never a fault (Figma 06a, DD-C08, IR315); an inspection record says so first
    expect(rows.map((r) => r.kind)).toEqual(["✕ Fault", "⌂ Load cause (possible)", "✕ Fault", "◷ Maintenance reminder"]);
    expect(inboxAlerts([alert("ins", "normal", "open", { causeCode: "insulation_loss", evidenceKind: "inspection" }), alert("loss", "warning", "open", { causeCode: "insulation_loss" })], [], () => undefined).map((r) => r.kind))
      .toEqual(["⌂ Load cause (possible)", "✎ Inspection record"]);
    // the user's language and display time zone (IR261); the evidence text and resolution reason stay as recorded
    const [, win, old] = inboxAlerts([alert("hot", "critical", "open"), alert("win", "warning", "acknowledged", { causeCode: "window_open", acknowledgedAt: "2026-09-14T01:30:00Z" }), alert("old", "critical", "resolved", { resolvedAt: "2026-09-14T02:00:00Z", resolutionReason: "filter cleaned" })],
      [], () => ({ name: "Bedroom AC", place: "" }), i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" }));
    expect([win.title, win.kind, win.status.text, win.status.detail, old.status.detail, old.where, old.evidence.split(":")[0]]).toEqual([
      "Kemungkinan tingkap terbuka", "⌂ Punca beban (mungkin)", "Diakui", "Diakui 14 Sep 2026, 10:30 PG GMT+9 · masih belum selesai", "Diselesaikan 14 Sep 2026, 11:00 PG GMT+9 · filter cleaned", "Bedroom AC · dikesan 14 Sep 2026, 10:00 PG GMT+9", "Bukti (disimpulkan)",
    ]);
  });
});
