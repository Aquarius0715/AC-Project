import { describe, expect, it } from "vitest";
import {
  actions, activity, bucket, capacity, kpis, partnerJob, period, progress, shift, timeline, timelinePct, until, weekOf,
  type ApiCapacity, type ApiMember, type ApiPartnerJob, type PartnerJob,
} from "@ac/web/lib/partnerOverview";
import { i18nOf } from "@ac/web/lib/i18n";

const NOW = "2026-09-21T01:30:00Z"; // Mon 09:30 in Kuala Lumpur
const now = Date.parse(NOW);
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const offer = (id: string, status: "offered" | "accepted", exp = "2026-09-21T17:00:00Z"): ApiPartnerJob => ({ projection: "offer", status, jobId: id, jobVersion: 2, offerId: `o-${id}`, type: "reactive", siteAddress: "Jalan 1, KL", requestedSlot: slot("2026-09-22T01:00:00Z", "2026-09-22T03:00:00Z"), dueAt: "2026-09-24T10:00:00Z", offerExpiresAt: exp, visitSlot: slot("2026-09-22T01:00:00Z", "2026-09-22T03:00:00Z") });
const summary = (id: string, status: string, over: Partial<Extract<ApiPartnerJob, { projection: "summary" }>> = {}): ApiPartnerJob => ({ projection: "summary", id, version: 3, unitId: `u-${id}`, type: "periodic", status, dueAt: "2026-09-25T00:00:00Z", requestedSlot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z"), scheduledSlot: null, assignmentId: null, displayStatus: status, technicianMembershipId: null, ...over });
const members: ApiMember[] = [
  { id: "m-a", userId: "user-a", displayName: "tech-external-a", role: "technician", qualifications: [{ code: "demo_refrigerant", revokedAt: null }] },
  { id: "m-b", userId: "user-b", displayName: "tech-external-a2", role: "technician", qualifications: [] },
  { id: "m-c", userId: "user-c", displayName: "coordinator", role: "contractor" },
];
const names = new Map(members.map((m) => [m.id, m.displayName]));

function fixture(): PartnerJob[] {
  const units = new Map([["u-p05", "Server room AC"], ["u-p07", "Server room AC"], ["u-ca", "Bedroom AC"], ["u-p02", "Rooftop unit"]]);
  const rows: ApiPartnerJob[] = [
    offer("p09", "offered"), offer("x01", "accepted"),
    summary("p02", "accepted"),
    summary("p07", "assigned", { assignmentId: "as-7", technicianMembershipId: "m-a", scheduledSlot: slot("2026-09-20T01:00:00Z", "2026-09-20T09:00:00Z") }),
    summary("ca", "in_progress", { assignmentId: "as-ca", technicianMembershipId: "m-a", scheduledSlot: slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z") }),
    summary("p05", "submitted", { assignmentId: "as-5", technicianMembershipId: "m-a", scheduledSlot: slot("2026-09-19T01:00:00Z", "2026-09-19T03:00:00Z") }),
    summary("c1", "completed"), summary("z1", "cancelled"),
    { projection: "history", jobId: "h1", type: "reactive", status: "completed", completedAt: "2026-09-10T00:00:00Z" },
  ];
  return rows.map((j) => partnerJob(j, units));
}

describe("partner overview", () => {
  it("defaults to the KL week and accepts a URL period up to 366 days", () => {
    expect(weekOf(NOW)).toEqual({ from: "2026-09-21", to: "2026-09-27" });
    expect(weekOf("2026-09-20T16:30:00Z")).toEqual({ from: "2026-09-21", to: "2026-09-27" }); // Mon 00:30 KL
    const p = period(NOW);
    expect(p).toMatchObject({ from: "2026-09-21", to: "2026-09-27", fromAt: "2026-09-20T16:00:00.000Z", toAt: "2026-09-27T16:00:00.000Z", label: "This week" });
    expect(p.days).toHaveLength(7);
    expect(period(NOW, "2026-09-14", "2026-09-20").label).toBe("Last week");
    expect(period(NOW, ...Object.values(shift(weekOf(NOW), 7)) as [string, string]).label).toBe("Next week");
    expect(period(NOW, "2026-09-27", "2026-09-21").from).toBe("2026-09-21"); // reversed → this week
    expect(period(NOW, "2025-01-01", "2026-09-27").from).toBe("2026-09-21"); // longer than 366 days → this week
  });

  it("buckets jobs, with ended work windows overdue (IR89)", () => {
    const j = fixture();
    expect(j.map((x) => bucket(x, now))).toEqual(["offered", "unassigned", "unassigned", "overdue", "active", "review", "completed", "other", "completed"]);
    expect(progress(j, now)).toEqual({ counts: { offered: 1, unassigned: 2, active: 1, overdue: 1, review: 1, completed: 2 }, total: 8, completedPct: 25 });
    expect(until("2026-09-21T17:00:00Z", now)).toBe("15 h");
    expect(until("2026-09-24T17:00:00Z", now)).toBe("3 d 15 h");
    expect(until("2026-09-21T01:00:00Z", now)).toBe("ended");
  });

  it("builds the KPI tiles from the summary counts and the list", () => {
    const tiles = kpis({ offerCount: 1, activeCount: 5, reviewCount: 1, overdueCount: 1 }, fixture(), now, names, { from: "2026-09-21", to: "2026-09-27" });
    expect(tiles.map((t) => [t.label, t.value, t.sub, t.href])).toEqual([
      ["Offers to answer", 1, "p09 · expires in 15 h", "/partner/jobs?tab=offered&from=2026-09-21&to=2026-09-27"],
      ["Awaiting assignment", 2, "Accepted, no technician yet", "/partner/schedule"],
      ["In progress / scheduled", 3, "tech-external-a on site from today 10:00 am MYT", "/partner/jobs?tab=active&from=2026-09-21&to=2026-09-27"],
      ["Reports to review", 1, "p05 · Server room AC · awaiting review", "/partner/jobs/p05/review"],
      ["Overdue", 1, "Past the due date or the work window (IR89)", "/partner/jobs?tab=active&from=2026-09-21&to=2026-09-27"],
    ]);
    expect(tiles[1].tone).toBe("warn");
    expect(tiles[4].tone).toBe("crit");
    const empty = kpis({ offerCount: 0, activeCount: 0, reviewCount: 0, overdueCount: 0 }, [], now, names);
    expect(empty.map((t) => t.sub)).toEqual(["No open offers", "—", "—", "—", "—"]);
    expect(empty.every((t) => !t.tone)).toBe(true);
  });

  it("lists what needs the company's action in order", () => {
    expect(actions(fixture(), now, names).map((r) => [r.kind, r.title, r.button, r.href])).toEqual([
      ["offer", "p09 · Jalan 1, KL", "Respond", "/partner/jobs/p09"],
      ["overdue", "p07 · Server room AC", "Reassign", "/partner/schedule?jobId=p07"],
      ["review", "p05 · Server room AC", "Review", "/partner/jobs/p05/review"],
      ["unassigned", "x01 · Jalan 1, KL", "Assign", "/partner/schedule?jobId=x01"],
      ["unassigned", "p02 · Rooftop unit", "Assign", "/partner/schedule?jobId=p02"],
    ]);
    const overdue = actions(fixture(), now, names)[1];
    expect(overdue.detail).toBe("Work window ended 20 Sept 2026, 5:00 pm MYT · tech-external-a"); // IR44
  });

  it("draws today's timeline and the week's capacity per technician", () => {
    const today: ApiCapacity[] = [{ membershipId: "m-a", date: "2026-09-21", availableSlots: [], assignedSlots: [slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z")], availableMinutes: 480, assignedMinutes: 120, unavailability: null }];
    expect(timelinePct("2026-09-21T00:00:00Z", "2026-09-21")).toBe(0);
    expect(timelinePct("2026-09-21T05:00:00Z", "2026-09-21")).toBe(50);
    const rows = timeline(today, members, "2026-09-21");
    expect(rows).toEqual([
      { id: "m-a", name: "tech-external-a", sub: "refrigerant", blocks: [{ left: 20, width: 20, text: "10:00–12:00" }], off: null },
      { id: "m-b", name: "tech-external-a2", sub: "Technician", blocks: [], off: null },
    ]);
    const week = capacity([today, [{ ...today[0], date: "2026-09-22", assignedMinutes: 480 }, { membershipId: "m-b", date: "2026-09-22", availableSlots: [], assignedSlots: [], availableMinutes: null, assignedMinutes: 0, unavailability: "leave" }]], members);
    expect(week).toEqual([
      { id: "m-a", name: "tech-external-a", text: "10 h / 16 h · 63%", pct: 63 },
      { id: "m-b", name: "tech-external-a2", text: "0 h / — · —", pct: null },
    ]);
  });

  it("shows the latest job events with the member who acted", () => {
    const jobs = new Map(fixture().map((j) => [j.id, j]));
    const rows = activity([
      { id: "e1", jobId: "p05", actorUserId: "user-a", action: "job.submitted", occurredAt: "2026-09-20T10:10:00Z" },
      { id: "e2", jobId: "p09", actorUserId: null, action: "job.offered", occurredAt: "2026-09-20T03:00:00Z" },
      { id: "e3", jobId: "ca", actorUserId: "user-a", action: "job.checked_in", occurredAt: "2026-09-21T01:05:00Z" },
      { id: "e4", jobId: "zz-unknown-job", actorUserId: "someone", action: "custom.thing", occurredAt: "2026-09-19T00:00:00Z" },
    ], jobs, members, now, 3);
    expect(rows.map((r) => [r.time, r.text, r.href])).toEqual([
      ["today 9:05 am MYT", "Checked in on site — ca · Bedroom AC (tech-external-a)", "/partner/history?jobId=ca"],
      ["yesterday 6:10 pm MYT", "Report submitted — p05 · Server room AC (tech-external-a)", "/partner/history?jobId=p05"],
      ["yesterday 11:00 am MYT", "Offer received from HQ — p09 · Jalan 1, KL", "/partner/history?jobId=p09"],
    ]);
  });

  it("speaks the display language, with instants in its time zone and the timeline in Kuala Lumpur hours (IR270)", () => {
    const i = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
    expect([until("2026-09-21T17:00:00Z", now, i.t), until("2026-09-24T17:00:00Z", now, i.t), until("2026-09-23T01:30:00Z", now, i.t), until("2026-09-21T01:00:00Z", now, i.t), until("2026-09-21T01:50:00Z", now, i.t)])
      .toEqual(["15 j", "3 hari 15 j", "2 hari", "tamat", "20 min"]);
    const tiles = kpis({ offerCount: 1, activeCount: 5, reviewCount: 1, overdueCount: 1 }, fixture(), now, names, undefined, i);
    expect(tiles.map((t) => [t.label, t.sub, t.link])).toEqual([
      ["Tawaran untuk dijawab", "p09 · tamat dalam 15 j", "Buka tawaran →"], ["Menunggu penugasan", "Diterima, belum ada juruteknik", "Tugaskan →"],
      ["Sedang berjalan / dijadualkan", "tech-external-a di tapak sejak hari ini 11:00 PG GMT+9", "Lihat kerja →"], ["Laporan untuk disemak", "p05 · Server room AC · menunggu semakan", "Semak →"],
      ["Tertunggak", "Melepasi tarikh akhir atau tetingkap kerja (IR89)", "Lihat →"],
    ]);
    expect(actions(fixture(), now, names, i).slice(0, 2).map((r) => [r.badge, r.detail, r.button])).toEqual([
      ["Tawaran", "Jawab sebelum 22 Sep 2026, 2:00 PG GMT+9 (15 j lagi)", "Jawab"], ["Tertunggak", "Tetingkap kerja tamat 20 Sep 2026, 6:00 PTG GMT+9 · tech-external-a", "Tugaskan semula"],
    ]);
    const today: ApiCapacity[] = [{ membershipId: "m-a", date: "2026-09-21", availableSlots: [], assignedSlots: [slot("2026-09-21T02:00:00Z", "2026-09-21T04:00:00Z")], availableMinutes: 480, assignedMinutes: 120, unavailability: null }];
    expect(timeline(today, members, "2026-09-21", i.t).map((r) => [r.sub, r.blocks.map((b) => b.text)])).toEqual([["refrigerant", ["10:00–12:00"]], ["Juruteknik", []]]); // Kuala Lumpur hours, like the axis
    expect(capacity([today], members, i.t).map((r) => r.text)).toEqual(["2 j / 8 j · 25%", "0 j / — · —"]);
    const rows = activity([{ id: "e2", jobId: "p09", actorUserId: null, action: "job.offered", occurredAt: "2026-09-20T03:00:00Z" }], new Map(fixture().map((j) => [j.id, j])), members, now, 6, i);
    expect(rows.map((r) => [r.time, r.text])).toEqual([["semalam 12:00 PTG GMT+9", "Tawaran diterima daripada HQ — p09 · Jalan 1, KL"]]);
  });
});
