import { describe, expect, it } from "vitest";
import { assignRefusal, candidates, formMode, freeText, scheduleRow, sortOf, weekGrid } from "@ac/web/lib/partnerSchedule";
import type { ApiDetail, ApiOffer } from "@ac/web/lib/partnerJobDetail";
import type { ApiCapacity, ApiMember, ApiPartnerJob } from "@ac/web/lib/partnerOverview";

const NOW = Date.parse("2026-09-21T01:30:00Z"); // Mon 09:30 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const visit = slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z"); // Tue 10:00–12:00 KL
const quals = (codes: string[]) => codes.map((code) => ({ code, validFrom: "2026-09-01T00:00:00Z", validUntil: "2027-03-31T00:00:00Z", revokedAt: null }));
const members = [
  { id: "m-a", userId: "u-a", displayName: "tech-external-a", role: "technician", qualifications: quals(["demo_indoor", "demo_outdoor", "demo_electrical"]) },
  { id: "m-b", userId: "u-b", displayName: "tech-external-a2", role: "technician", qualifications: quals(["demo_indoor"]) },
  { id: "m-c", userId: "u-c", displayName: "tech-external-a3", role: "technician", qualifications: quals(["demo_indoor", "demo_outdoor", "demo_electrical"]) },
  { id: "m-d", userId: "u-d", displayName: "coordinator", role: "contractor" },
] as unknown as ApiMember[];
const names = new Map(members.map((m) => [m.id, m.displayName]));
const units = new Map([["u1", "Lobby AC"]]);
const summary = (over: Record<string, unknown>) => ({
  projection: "summary", id: "job-1-aaaa", version: 3, unitId: "u1", type: "reactive", status: "accepted", dueAt: "2026-09-25T00:00:00Z", requestedSlot: visit,
  scheduledSlot: null, assignmentId: null, displayStatus: "accepted", technicianMembershipId: null, assignmentAcknowledgement: null,
  accessValidFrom: "2026-09-20T01:30:00Z", accessValidUntil: "2026-09-22T01:30:00Z", ...over,
}) as unknown as ApiPartnerJob;
const cap = (id: string, date: string, avail: [string, string][], assigned: [string, string][], am: number, unavailability: string | null = null): ApiCapacity =>
  ({ membershipId: id, date, availableSlots: avail.map(([s, e]) => slot(s, e)), assignedSlots: assigned.map(([s, e]) => slot(s, e)), availableMinutes: am, assignedMinutes: 0, unavailability });
const workday = (d: string): [string, string] => [`${d}T01:00:00Z`, `${d}T09:00:00Z`];

describe("partner schedule", () => {
  it("lists the jobs with their state and delegation window", () => {
    const rows = [
      scheduleRow(summary({}), NOW, units, names)!,
      scheduleRow(summary({ status: "assigned", scheduledSlot: visit, technicianMembershipId: "m-a", assignmentAcknowledgement: "cant_make" }), NOW, units, names)!,
      scheduleRow(summary({ status: "in_progress", scheduledSlot: slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z"), technicianMembershipId: "m-a" }), NOW, units, names)!,
      scheduleRow(summary({ status: "on_hold", accessValidUntil: "2026-09-21T01:00:00Z" }), NOW, units, names)!,
      scheduleRow({ projection: "offer", status: "accepted", jobId: "job-2-bbbb", jobVersion: 2, offerId: "o", type: "reactive", siteAddress: "2 Demo Avenue", requestedSlot: visit, dueAt: "x", offerExpiresAt: "x", visitSlot: visit, accessValidFrom: "2026-09-22T00:00:00Z", accessValidUntil: "2026-09-26T00:00:00Z" }, NOW, units, names)!,
    ];
    expect(rows.map((r) => [r.title, r.line, r.tone ?? ""])).toEqual([
      ["Lobby AC", "Accepted · awaiting assignment", ""],
      ["Lobby AC", "Assigned · tech-external-a · can’t make it ⚠", "warn"],
      ["Lobby AC", "Work window ended · reassignment required", "crit"],
      ["Lobby AC", "On hold by HQ", "warn"],
      ["2 Demo Avenue", "Accepted · delegation starts 09-22 08:00", "muted"],
    ]);
    expect([rows[0].windowText, rows[0].left, rows[0].leftTone, rows[0].elapsedPct]).toEqual(["09-20 09:30 → 09-22 09:30", "1 d left", "primary", 50]);
    expect([rows[3].left, rows[3].leftTone]).toEqual(["Ended", "crit"]);
    expect(scheduleRow({ projection: "history", jobId: "h", type: "reactive", status: "completed", completedAt: null }, NOW, units, names)).toBeNull();
    expect(sortOf("dueAt")).toBe("dueAt");
    expect(sortOf("nope")).toBe("status");
  });

  it("decides what the form of a job allows", () => {
    const detail = (over: Record<string, unknown>) => ({ projection: "detail", id: "job-1", version: 4, unitId: "u1", type: "reactive", status: "accepted", scheduledSlot: null, assignment: null, offer: { id: "o", termsVersion: "t", visitSlot: visit, offeredAt: "x", accessValidFrom: "x", accessValidUntil: "x", decision: "accept", decidedAt: "x" }, ...over }) as unknown as ApiDetail;
    const asg = (s: { startAt: string; endAt: string }, ack = "pending", why: string | null = null) => ({ technicianMembershipId: "m-a", scheduledStart: s.startAt, scheduledEnd: s.endAt, status: "active", acknowledgement: ack, cantMakeReason: why });
    const a = formMode(detail({}), "job-1 · Lobby AC", NOW, names);
    expect(a.kind === "assign" && [a.title, a.slot, a.endEditable, a.reasonRequired, a.button]).toEqual(["Assign — job-1 · Lobby AC", visit, false, false, "Confirm assignment"]);
    const r = formMode(detail({ status: "assigned", scheduledSlot: visit, assignment: asg(visit, "cant_make", "sick") }), "job-1", NOW, names);
    expect(r.kind === "reassign" && [r.currentTech, r.banner?.text.startsWith("tech-external-a can’t make this time: “sick”")]).toEqual(["tech-external-a", true]);
    const past = slot("2026-09-20T02:00:00Z", "2026-09-20T04:00:00Z");
    const gone = formMode(detail({ status: "assigned", scheduledSlot: past, assignment: asg(past) }), "job-1", NOW, names);
    expect([gone.kind, gone.banner?.tone]).toEqual(["blocked", "crit"]);
    const ext = formMode(detail({ status: "in_progress", scheduledSlot: past, assignment: asg(past) }), "job-1", NOW, names);
    expect(ext.kind === "extend" && [ext.title, ext.endEditable, ext.reasonRequired, ext.banner?.tone]).toEqual(["Reschedule — job-1", true, true, "crit"]);
    const running = formMode(detail({ status: "in_progress", scheduledSlot: visit, assignment: asg(visit, "accepted") }), "job-1", NOW, names);
    expect(running.kind === "extend" && [running.title, running.banner]).toEqual(["Reassign — job-1 (in progress)", undefined]);
    const offer = { projection: "offer", status: "accepted", jobId: "job-2", visitSlot: visit, accessValidFrom: "2026-09-22T00:00:00Z" } as unknown as ApiOffer;
    expect(formMode(offer, "job-2", NOW, names)).toMatchObject({ kind: "blocked", banner: { text: "Accepted — the delegation period starts 2026-09-22 08:00; the technician can be assigned from then, for exactly 09-22 10:00–12:00." } });
    expect(formMode(detail({ status: "completed" }), "job-1", NOW, names).kind).toBe("blocked");
  });

  it("names the free time and the technicians that can take the slot", () => {
    const day = [
      cap("m-a", "2026-09-22", [workday("2026-09-22")], [["2026-09-22T05:00:00Z", "2026-09-22T07:00:00Z"]], 480),
      cap("m-b", "2026-09-22", [workday("2026-09-22")], [], 480),
      cap("m-c", "2026-09-22", [], [], 0, "leave"),
    ];
    expect(freeText(day[0])).toBe("09:00–13:00, 15:00–17:00");
    expect(freeText(day[2])).toBe("no hours");
    expect(freeText(cap("m-a", "d", [workday("2026-09-22")], [workday("2026-09-22")], 480))).toBe("fully booked");
    const rows = candidates(members, new Set(["m-a"]), ["demo_indoor", "demo_electrical"], visit, day, null);
    expect(rows.map((c) => [c.name, c.badge.text, c.eligible])).toEqual([
      ["tech-external-a", "Qualified", true], ["tech-external-a2", "Missing: Electrical work", false], ["tech-external-a3", "Unavailable (leave)", false],
    ]);
    expect(rows[0].sub).toBe("Indoor unit work · Outdoor unit work · Electrical work · free 09:00–13:00, 15:00–17:00");
    const busy = candidates(members, new Set(), ["demo_indoor"], visit, [cap("m-a", "2026-09-22", [workday("2026-09-22")], [["2026-09-22T01:00:00Z", "2026-09-22T09:00:00Z"]], 480)], null);
    expect(busy[0].badge.text).toBe("Busy at this time");
    expect(candidates(members, new Set(["m-a"]), ["demo_indoor"], visit, [], "m-a")[0].badge.text).toBe("Current");
  });

  it("builds the team's week with jobs, leave and the proposal", () => {
    const dates = ["2026-09-21", "2026-09-22", "2026-09-23"];
    const week = [
      [cap("m-a", dates[0], [workday(dates[0])], [["2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z"]], 480), cap("m-b", dates[0], [workday(dates[0])], [], 480)],
      [cap("m-a", dates[1], [workday(dates[1])], [], 480), cap("m-b", dates[1], [], [], 0, "leave")],
      [cap("m-a", dates[2], [], [], 0), cap("m-b", dates[2], [workday(dates[2])], [], 480)],
    ];
    const jobsOf = new Map([["m-a", [{ short: "job-9", slot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z") }]]]);
    const rows = weekGrid(dates, members.slice(0, 2), week, jobsOf, { technicianId: "m-a", slot: visit, short: "job-1" });
    expect(rows[0].cells).toEqual([{ kind: "booked", text: "10:00–12:00", sub: "job-9" }, { kind: "proposal", text: "10:00–12:00", sub: "job-1" }, { kind: "none", text: "no hours" }]);
    expect(rows[1].cells.map((c) => c.kind)).toEqual(["free", "leave", "free"]);
    expect([rows[0].sub, rows[1].sub]).toEqual(["Indoor, Outdoor, Electrical · 16 h avail.", "Indoor · 16 h avail."]);
  });

  it("states the refusals of the assignment", () => {
    const f = (code: string, messageKey: string, fieldErrors: Record<string, string> = {}) => assignRefusal({ code, messageKey, fieldErrors }, "tech-external-a");
    expect(f("CONFLICT", "errors.assignment_overlap")).toBe("Overlaps a confirmed schedule for tech-external-a (CONFLICT) — choose another technician.");
    expect(f("FORBIDDEN", "errors.access_window")).toMatch(/^Outside the delegation period/);
    expect(f("FORBIDDEN", "errors.qualification_missing")).toMatch(/revoked or expiring, FORBIDDEN/);
    expect(f("VALIDATION", "error.validation", { startAt: "errors.slot_not_agreed" })).toMatch(/^startAt: the slot must be the agreed visit time/);
    expect(f("VALIDATION", "error.validation", { reason: "error.required" })).toBe("reason: required (1–1000 characters)");
    expect(f("VALIDATION", "error.validation", { endAt: "errors.extension_only" })).toBe("endAt: keep the start and set an end at or after the current end");
    expect(f("NOT_FOUND", "error.notFound")).toBe("The job is no longer delegated to your company.");
  });
});
