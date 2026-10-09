import { describe, expect, it } from "vitest";
import { alertRows, dayTimeline, isToday, kpis, reportRows, sortOf, tabOf, techRow, type ApiTechJobRow } from "@ac/web/lib/techOverview";
import type { ApiAlert } from "@ac/web/lib/alerts";

const NOW = Date.parse("2026-09-21T03:45:00Z"); // Mon 11:45 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const units = new Map([["u1", "Bedroom AC"], ["u2", "Server room AC"]]);
const row = (over: Record<string, unknown>) => ({
  projection: "summary", id: "job-1-aaaa", version: 3, unitId: "u1", type: "reactive", status: "assigned", severity: "normal", dueAt: "x", requestedSlot: slot("a", "b"),
  scheduledSlot: slot("2026-09-21T06:00:00Z", "2026-09-21T08:00:00Z"), origin: "client_request", assignmentAcknowledgement: "accepted", ...over,
}) as unknown as ApiTechJobRow;

describe("technician overview", () => {
  it("states what the technician can do with each job", () => {
    const later = techRow(row({}), NOW, units)!;
    expect([later.badge.text, later.line, later.today, later.opensAt, later.ackPending]).toEqual(["Work not started", "Repair · window 14:00–16:00 · opens 14:00 today (read-only until then)", true, "2026-09-21T06:00:00Z", false]);
    const pending = techRow(row({ assignmentAcknowledgement: "pending", scheduledSlot: slot("2026-09-23T02:00:00Z", "2026-09-23T04:00:00Z") }), NOW, units)!;
    expect([pending.badge, pending.ackPending, pending.today, pending.line]).toEqual([{ text: "Not accepted", tone: "warn" }, true, false, "Repair · window 09-23 10:00–12:00 · new assignment — review and accept it"]);
    expect(techRow(row({ status: "completed", scheduledSlot: slot("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z") }), NOW, units)!.slotText).toBe("09-14 08:00 → 09-20 08:00");
    const running = techRow(row({ status: "in_progress", severity: "critical", scheduledSlot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z") }), NOW, units)!;
    expect([running.badge.text, running.line]).toEqual(["In progress", "Repair · window 10:00–12:00 · critical alert on the unit · report draft open"]);
    expect(techRow(row({ status: "in_progress", scheduledSlot: slot("2026-09-21T00:00:00Z", "2026-09-21T02:00:00Z") }), NOW, units)!.badge).toEqual({ text: "Work window ended", tone: "crit" });
    expect(techRow(row({ status: "rework_requested" }), NOW, units)!.badge.text).toBe("Rework requested");
    expect(techRow({ projection: "history", jobId: "h", type: "periodic", status: "completed", completedAt: null }, NOW, units)).toBeNull();
    expect([tabOf("all"), tabOf("x"), sortOf("dueAt"), sortOf("x")]).toEqual(["all", "today", "dueAt", "severity"]);
  });

  it("fills the tiles, today's timeline and the side cards", () => {
    const rows = [
      techRow(row({ id: "job-a", status: "in_progress", scheduledSlot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z") }), NOW, units)!,
      techRow(row({ id: "job-b" }), NOW, units)!,
      techRow(row({ id: "job-c", status: "submitted", scheduledSlot: slot("2026-09-19T02:00:00Z", "2026-09-19T04:00:00Z") }), NOW, units)!,
    ];
    expect(kpis({ total: 2, scheduledCount: 1, inProgressCount: 1, overdueCount: 0, assignedCount: 3 }, rows)).toEqual([
      { label: "Assigned units", value: 2, sub: "in your assignments" }, { label: "Not started", value: 1, sub: "job-b", tone: "warn" }, { label: "Overdue", value: 0, sub: "No overdue jobs", tone: "ok" },
    ]);
    const t = dayTimeline(rows, NOW);
    expect(t.blocks.map((b) => [b.short, b.left, b.width, b.current])).toEqual([["job-a", 20, 20, true], ["job-b", 60, 20, false]]);
    expect(t.nowPct).toBe(37.5);
    expect(isToday(slot("2026-09-20T15:00:00Z", "2026-09-20T16:30:00Z"), NOW)).toBe(true); // 23:00–00:30 KL touches Monday
    const alerts = [
      { id: "a1", unitId: "u2", type: "sensor", severity: "warning", status: "open", causeCode: "window_open", evidenceKind: "inferred", evidenceText: "", detectedAt: "2026-09-21T01:12:00Z" },
      { id: "a2", unitId: "u1", type: "maintenance", severity: "critical", status: "acknowledged", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "", detectedAt: "2026-09-20T01:00:00Z" },
      { id: "a3", unitId: "u1", type: "sensor", severity: "warning", status: "resolved", causeCode: "unknown", evidenceKind: "inferred", evidenceText: "", detectedAt: "2026-09-20T01:00:00Z" },
    ] as ApiAlert[];
    expect(alertRows(alerts, units, new Map([["u1", "job-a"]]))).toEqual([
      { id: "a2", title: "Maintenance reminder", sub: "Bedroom AC · acknowledged · 09-20 09:00", severity: "critical", href: "/technician/units/u1/alerts?jobId=job-a" },
      { id: "a1", title: "Possible open window", sub: "Server room AC · open · 09-21 09:12", severity: "warning", href: "/technician/units/u2/alerts" },
    ]);
    expect(reportRows(rows).map((r) => [r.title, r.badge.text])).toEqual([["job-a · Bedroom AC", "Draft"], ["job-c · Bedroom AC", "Submitted"]]);
  });
});
