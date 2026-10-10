import { describe, expect, it } from "vitest";
import { byUser, channelText, communicationRefusal, customerVisible, eventItem, historyRow, noteMode, periodOf, sortOf, tabCounts, tabOf, visibleRows, type ApiEvent } from "@ac/web/lib/partnerHistory";
import type { ApiMember, ApiPartnerJob } from "@ac/web/lib/partnerOverview";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const ms = translator("ms");

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const members = [{ id: "m-a", userId: "u-a", displayName: "tech-external-a", role: "technician" }, { id: "m-c", userId: "u-c", displayName: "contractor-a", role: "contractor" }] as unknown as ApiMember[];
const names = new Map(members.map((m) => [m.id, m.displayName]));
const units = new Map([["u1", "Office AC"]]);
const ev = (id: string, action: string, at: string, note: ApiEvent["note"] = null, actor: string | null = "u-c"): ApiEvent => ({ id, jobId: "j", actorUserId: actor, action, occurredAt: at, note });
const summary = (over: Record<string, unknown>) => ({
  projection: "summary", id: "job-1-aaaa", version: 7, unitId: "u1", type: "reactive", status: "in_progress", dueAt: "x", requestedSlot: slot("a", "b"), scheduledSlot: slot("2026-09-21T01:00:00Z", "2026-09-21T03:00:00Z"),
  assignmentId: "as", displayStatus: "in_progress", technicianMembershipId: "m-a", assignmentAcknowledgement: "accepted", accessValidFrom: "2026-09-14T00:00:00Z", accessValidUntil: "2026-09-25T16:00:00Z", origin: "client_request", ...over,
}) as unknown as ApiPartnerJob;
const events = [
  ev("e1", "offer.accepted", "2026-09-18T00:30:00Z"),
  ev("e2", "note.added", "2026-09-20T06:05:00Z", { visibility: "internal", message: "Missing filter photo" }),
  ev("e3", "job.started", "2026-09-21T01:05:00Z", null, "u-a"),
  ev("e4", "note.added", "2026-09-20T02:00:00Z", { visibility: "customer", message: "Arriving at 09:00" }, "u-x"),
];

describe("partner job history", () => {
  it("summarises a job with its events", () => {
    const r = historyRow(summary({}), events, NOW, units, names)!;
    expect([r.title, r.sub, r.tech, r.last, r.badge, r.tab, r.delegation, r.version, r.origin]).toEqual([
      "Office AC", "4 events · 3 customer-visible · 2 notes · delegation until 26 Sept 2026, 12:00 am MYT", { name: "tech-external-a", sub: "accepted the slot ✓" },
      { title: "Work started", at: "2026-09-21T01:05:00Z", when: "today 9:05 am MYT" }, { text: "In progress", tone: "primary" }, "active", "until 26 Sept 2026, 12:00 am MYT", 7, "client_request",
    ]);
    const overdue = historyRow(summary({ status: "assigned", scheduledSlot: slot("2026-09-20T01:00:00Z", "2026-09-20T03:00:00Z") }), [], NOW, units, names)!;
    expect([overdue.badge, overdue.overdue, overdue.sub, overdue.last]).toEqual([{ text: "Overdue", tone: "crit" }, true, "0 events · 0 customer-visible · 0 notes · delegation until 26 Sept 2026, 12:00 am MYT", null]);
    const unassigned = historyRow(summary({ status: "accepted", technicianMembershipId: null, assignmentAcknowledgement: null }), [], NOW, units, names)!;
    expect(unassigned.tech).toEqual({ name: "Unassigned", sub: "assign in Schedule", tone: "crit" });
    const h = historyRow({ projection: "history", jobId: "job-9-hhhh", type: "periodic", status: "completed", completedAt: null }, [events[0]], NOW, units, names)!;
    expect([h.tab, h.ended, h.sub, h.badge.text, h.version]).toEqual(["ended", true, "1 own decision event · delegation ended (read-only snapshot)", "Completed", null]);
    expect(historyRow({ projection: "offer", status: "offered" } as unknown as ApiPartnerJob, [], NOW, units, names)).toBeNull();
  });

  it("filters, counts and orders the rows", () => {
    const rows = [
      historyRow(summary({ id: "a", status: "submitted" }), [ev("x", "job.submitted", "2026-09-20T00:00:00Z")], NOW, units, names)!,
      historyRow(summary({ id: "b", status: "completed" }), [ev("y", "job.reviewed", "2026-09-21T00:00:00Z")], NOW, units, names)!,
      historyRow(summary({ id: "c", status: "accepted" }), [ev("z", "offer.accepted", "2026-09-19T00:00:00Z")], NOW, units, names)!,
      historyRow({ projection: "history", jobId: "d", type: "periodic", status: "completed", completedAt: null }, [], NOW, units, names)!,
    ];
    expect(tabCounts(rows)).toEqual({ all: 4, active: 1, review: 1, completed: 1, ended: 1 });
    expect(visibleRows(rows, "all", "latest").map((r) => r.id)).toEqual(["b", "a", "c", "d"]);
    expect(visibleRows(rows, "all", "status").map((r) => r.id)).toEqual(["c", "a", "b", "d"]);
    expect(visibleRows(rows, "ended", "latest").map((r) => r.id)).toEqual(["d"]);
    expect([tabOf("review"), tabOf("x"), sortOf("status"), sortOf(), periodOf("all"), periodOf("x")]).toEqual(["review", "all", "status", "latest", "all", "90d"]);
  });

  it("builds the timeline rows and who can see them", () => {
    const users = byUser(members, "u-me");
    expect(customerVisible(events[1])).toBe(false);
    expect(eventItem(events[1], users)).toEqual({ id: "e2", title: "Note added", detail: "“Missing filter photo” · internal · by contractor-a", at: "20 Sept 2026, 2:05 pm MYT", badge: "internal", tone: undefined });
    expect(eventItem(events[3], users)).toMatchObject({ badge: "customer", detail: "“Arriving at 09:00” · customer" }); // someone outside the company is not named
    expect(eventItem(ev("m", "offer.accepted", "2026-09-20T00:00:00Z", null, "u-me"), users).detail).toBe("by you");
    expect([channelText("inApp"), channelText("email"), channelText("whatsapp")]).toEqual(["in-app", "email", "WhatsApp"]);
    expect(eventItem(ev("r", "job.rework_started", "2026-09-20T00:00:00Z", null, null), users)).toMatchObject({ detail: "by system", tone: "warn", badge: "customer" });
  });

  it("decides what the communication form may do and names refusals", () => {
    expect(noteMode({ ended: false, status: "in_progress" })).toEqual({ kind: "note", button: "Save note & create preview" });
    expect(noteMode({ ended: false, status: "completed" })).toMatchObject({ kind: "preview", text: "Notes are closed on completed jobs — only the preview is created." });
    expect(noteMode({ ended: true, status: "completed" }).kind).toBe("closed");
    expect(communicationRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { recipientMembershipId: "errors.recipient_ineligible" } })).toBe("Recipient: not a recipient of this job — choose one from the list");
    expect(communicationRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { message: "error.length" } })).toBe("Message: 1–2000 characters");
    expect(communicationRefusal({ code: "CONFLICT", messageKey: "error.invalidState", fieldErrors: {} })).toMatch(/^Notes are closed/);
    expect(communicationRefusal({ code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} })).toMatch(/your text is kept; retry/);
  });

  it("words the history in Malay with the times in the display time zone (IR278)", () => {
    const r = historyRow(summary({}), events, NOW, units, names, MS)!;
    expect([r.sub, r.tech.sub, r.last, r.badge.text]).toEqual([
      "4 peristiwa · 3 boleh dilihat pelanggan · 2 nota · delegasi hingga 26 Sep 2026, 1:00 PG GMT+9", "menerima slot ✓",
      { title: "Kerja dimulakan", at: "2026-09-21T01:05:00Z", when: "hari ini 10:05 PG GMT+9" }, "Sedang berjalan",
    ]);
    const h = historyRow({ projection: "history", jobId: "job-9-hhhh", type: "periodic", status: "completed", completedAt: null }, [events[0]], NOW, units, names, MS)!;
    expect([h.title, h.sub, h.delegation, h.tech.sub]).toEqual(["Pemeriksaan berkala", "1 peristiwa keputusan sendiri · delegasi tamat (rakaman baca sahaja)", "tamat — sejarah sahaja", "tidak ditunjukkan selepas delegasi"]);
    expect(eventItem(events[1], byUser(members, "u-me", ms), MS)).toMatchObject({ title: "Nota ditambah", detail: "“Missing filter photo” · dalaman · oleh contractor-a", at: "20 Sep 2026, 3:05 PTG GMT+9" });
    expect(eventItem(ev("m", "offer.accepted", "2026-09-20T00:00:00Z", null, "u-me"), byUser(members, "u-me", ms), MS).detail).toBe("oleh anda");
    expect(noteMode({ ended: false, status: "completed" }, ms)).toEqual({ kind: "preview", button: "+ Cipta pratonton", text: "Nota ditutup untuk kerja yang selesai — hanya pratonton dicipta." });
    expect(communicationRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { message: "error.required" } }, ms)).toBe("Mesej: diperlukan");
  });
});
