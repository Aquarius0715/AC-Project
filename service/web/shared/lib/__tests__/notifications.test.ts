import { describe, expect, it } from "vitest";
import { alertRow } from "@ac/web/lib/alerts";
import { inboxRow, type ApiNotification } from "@ac/web/lib/notifications";

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

  it("opens Filter care from a cleaning reminder (IR239)", () => {
    const r = inboxRow(note({ type: "cleaning_due", templateKey: "alert", sourceAlertId: "a1", target: { kind: "unit", id: "u1" },
      params: { targetName: "Bedroom AC", at: "2026-09-14T01:00:00Z", status: "open", reason: null, message: "Cleaning due (268 h of run time since the last cleaning). Not a fault." } }), "client");
    expect([r.t, r.d, r.href]).toEqual(["Filter cleaning due on Bedroom AC", "Cleaning due (268 h of run time since the last cleaning). Not a fault. · open", "/customer/maintenance?tab=filter-care"]);
  });

  it("shows a filter cleaning reminder as information, not a fault (Figma Client 06a)", () => {
    const a = alertRow({ id: "a1", unitId: "u1", type: "maintenance", severity: "normal", status: "open", causeCode: "unknown", evidenceKind: "inferred",
      evidenceText: "Cleaning due (250 h of run time since the last cleaning). Not a fault.", detectedAt: "2026-09-14T01:00:00Z" });
    expect([a.title, a.kind, a.sev, a.group, a.read, a.ev]).toEqual(["Filter cleaning reminder", "◷ Maintenance reminder", "normal", "info", false, "Evidence (inferred): Cleaning due (250 h of run time since the last cleaning). Not a fault."]);
  });
});
