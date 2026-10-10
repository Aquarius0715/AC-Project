import { describe, expect, it } from "vitest";
import {
  clientEventTitle, clientJobRefusal, slotLong, slotShort, clientRows, contactError, declinedNotice, detailFacts, feedback, followUpText, notesOf, planVisit, preferredError, preferredLines, proposalCard, tabOf, tabOfQuery,
  type ApiClientJob, type ApiClientRow,
} from "@ac/web/lib/customerMaintenance";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const NOW = Date.parse("2026-09-14T01:00:00Z"); // Mon 09:00 KL
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const row = (over: Partial<ApiClientRow>): ApiClientRow => ({
  projection: "summary", id: "job-1-aaaa", version: 1, unitId: "u1", type: "reactive", status: "requested", displayStatus: "requested", origin: "client_request", dueAt: "2026-09-16T04:00:00Z",
  requestedSlot: slot("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z"), scheduledSlot: null, preferredSlots: [slot("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z"), slot("2026-09-17T02:00:00Z", "2026-09-17T04:00:00Z"), slot("2026-09-18T02:00:00Z", "2026-09-18T04:00:00Z")],
  assignmentAcknowledgement: null, ...over,
});
const job = (over: Partial<ApiClientJob>): ApiClientJob => ({
  projection: "detail", id: "job-1-aaaa", version: 3, unitId: "u1", type: "reactive", status: "requested", symptom: "Water dripping from the indoor unit", contactWindow: null, origin: "client_request",
  planId: null, occurrenceAt: null, requestedSlot: slot("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z"), preferredSlots: row({}).preferredSlots, preferenceRound: 1, slotProposal: null, scheduledSlot: null,
  dueAt: "2026-09-16T04:00:00Z", completedAt: null, contractorOrgId: null, followUpOfJobId: null, followUpClass: null, rating: null, customerConfirmedAt: null, reportRefs: [], assignment: null, ...over,
});
const unit = { name: "Bedroom AC", place: "Home A › 1F › Bedroom" };

describe("customer maintenance", () => {
  it("groups the requests and says what is next", () => {
    expect([tabOf("time_proposed"), tabOf("accepted"), tabOf("submitted"), tabOf("on_hold"), tabOfQuery("x"), tabOfQuery("reply")]).toEqual(["reply", "scheduled", "progress", "progress", "all", "reply"]);
    const rows = clientRows([
      row({ id: "job-c-done", status: "completed", displayStatus: "completed", scheduledSlot: slot("2026-09-08T02:00:00Z", "2026-09-08T03:00:00Z") }),
      row({}), row({ id: "job-prop", displayStatus: "time_proposed" }),
      row({ id: "job-plan", origin: "periodic_plan", status: "assigned", displayStatus: "assigned", preferredSlots: [], scheduledSlot: slot("2026-12-08T02:00:00Z", "2026-12-08T03:00:00Z"), assignmentAcknowledgement: "accepted" }),
    ], () => unit);
    expect(rows.map((r) => [r.id, r.tab, r.origin, r.type, r.line, r.warn])).toEqual([
      ["job-prop", "reply", "request", "Repair", "⇄ A new visit time needs your reply — nothing is booked until you accept", true],
      ["job-1-aaaa", "requested", "request", "Repair", "◷ Preferred 16 Sept, 10:00 am – 12:00 pm MYT · +2 more (not confirmed yet)", false],
      ["job-plan", "scheduled", "plan", "Periodic inspection", "◷ Scheduled 8 Dec, 10:00 – 11:00 am MYT · technician confirmed", false],
      ["job-c-done", "completed", "request", "Repair", "✓ Completed · visit 8 Sept, 10:00 – 11:00 am MYT · report available", false],
    ]);
  });

  it("shows the detail facts, the preferred times and the technician once accepted", () => {
    const booked = job({ status: "assigned", scheduledSlot: row({}).preferredSlots[1], assignment: { acknowledgement: "accepted", technicianName: "tech-internal-a", scheduledStart: "2026-09-17T02:00:00Z", scheduledEnd: "2026-09-17T04:00:00Z", status: "active" } });
    expect(detailFacts(booked, unit)).toEqual([
      ["Unit", "Bedroom AC · Home A › 1F › Bedroom"], ["Origin · type", "Client request · Repair"], ["Request", "“Water dripping from the indoor unit”"],
      ["Confirmed time", "Thu, 17 Sept, 10:00 am – 12:00 pm MYT"], ["Technician", "tech-internal-a · HQ"],
    ]);
    expect(detailFacts(job({ status: "offered", contractorOrgId: "org-c", scheduledSlot: row({}).preferredSlots[0] }), unit).at(-1)).toEqual(["Technician", "being confirmed — the name shows once the technician accepts"]);
    expect(preferredLines(booked).map((l) => [l.text, l.note])).toEqual([["1st · Wed, 16 Sept, 10:00 am – 12:00 pm MYT", null], ["2nd · Thu, 17 Sept, 10:00 am – 12:00 pm MYT", "✓ booked"], ["3rd · Fri, 18 Sept, 10:00 am – 12:00 pm MYT", null]]);
    expect(detailFacts(job({ followUpOfJobId: "job-0-zzzz", followUpClass: "pending" }), unit)[2]).toEqual(["Follow-up of", "job-0-zz · Under HQ review"]);
  });

  it("explains the proposal, the declined notice and the plan visit", () => {
    const p = proposalCard(job({ slotProposal: { id: "p1", source: "hq", slot: slot("2026-09-19T06:00:00Z", "2026-09-19T08:00:00Z"), hold: { kind: "internal" }, message: "All booked on your times.", replyBy: "2026-09-16T01:00:00Z", status: "pending", decidedAt: null, declineReason: null, declineComment: null } }), NOW)!;
    expect([p.title, p.when, p.who, p.replyBy, p.late]).toEqual(["HQ proposes a new time", "Sat, 19 Sept, 2:00 – 4:00 pm MYT", "An HQ technician (name shown once confirmed)", "16 Sept 2026, 9:00 am MYT", false]);
    expect(proposalCard(job({}), NOW)).toBeNull();
    expect(declinedNotice(job({ slotProposal: { ...p, hold: { kind: "internal" }, source: "hq", slot: slot("2026-09-19T06:00:00Z", "2026-09-19T08:00:00Z"), status: "declined", decidedAt: null, declineReason: "not_home", declineComment: null, replyBy: "" } }))).toBe("You declined the proposed time (not at home). HQ is looking at your new times.");
    expect(planVisit(job({ origin: "periodic_plan", status: "assigned", scheduledSlot: slot("2026-12-08T02:00:00Z", "2026-12-08T03:00:00Z") }), NOW)).toEqual({ can: true, why: null });
    expect(planVisit(job({ origin: "periodic_plan", status: "assigned", scheduledSlot: slot("2026-09-15T02:00:00Z", "2026-09-15T03:00:00Z") }), NOW)?.can).toBe(false);
    expect(planVisit(job({}), NOW)).toBeNull();
  });

  it("offers Confirm & rate for 7 days and edits until editableUntil", () => {
    expect(feedback(job({ status: "completed", completedAt: "2026-09-13T01:00:00Z" }), NOW).kind).toBe("rate");
    expect(feedback(job({ status: "completed", completedAt: "2026-09-01T01:00:00Z" }), NOW).kind).toBe("none");
    const rated = job({ status: "completed", completedAt: "2026-09-13T01:00:00Z", rating: { stars: 4, tags: ["On time"], comment: null, ratedAt: "2026-09-13T02:00:00Z", editableUntil: "2026-09-20T02:00:00Z" } });
    expect(feedback(rated, NOW)).toEqual({ kind: "edit", text: "Rated ★★★★ · On time · editable until 20 Sept 2026, 10:00 am MYT" });
    expect(feedback({ ...rated, rating: { ...rated.rating!, editableUntil: "2026-09-13T03:00:00Z" } }, NOW).kind).toBe("rated");
  });

  it("checks the times, the contact window and names refusals", () => {
    const three = row({}).preferredSlots;
    expect(preferredError(three, NOW)).toBeNull();
    expect(preferredError(three.slice(0, 2), NOW)).toBe("Give 3 preferred times.");
    expect(preferredError([slot("2026-09-14T06:00:00Z", "2026-09-14T08:00:00Z"), ...three.slice(1)], NOW)).toBe("Times start on a later day than today (Kuala Lumpur).");
    expect(preferredError([three[0], three[0], three[1]], NOW)).toBe("The 3 times must be different.");
    expect(preferredError([slot("2026-09-16T02:00:00Z", "2026-09-16T08:00:00Z"), ...three.slice(1)], NOW)).toBe("Each time is 1–4 hours long.");
    expect([contactError("Weekdays 09:00-18:00"), contactError("call +60 12-345 6789"), contactError("me@x.com")]).toEqual([null, expect.stringMatching(/No e-mail/), expect.stringMatching(/No e-mail/)]);
    expect(notesOf([{ id: "e1", action: "note.added", occurredAt: "2026-09-14T01:00:00Z", actorUserId: "me", note: { visibility: "customer", message: "Gate code 4821" } }, { id: "e2", action: "job.created", occurredAt: "2026-09-13T01:00:00Z", actorUserId: "me", note: null }], "me"))
      .toEqual([{ id: "e1", who: "You", at: "14 Sept 2026, 9:00 am MYT", text: "Gate code 4821" }]);
    expect(clientJobRefusal({ code: "CONFLICT", messageKey: "errors.reschedule_too_late", fieldErrors: {} })).toBe("Not saved: less than 48 h before the visit — contact HQ.");
  });

  it("words the list, the detail and the checks in Malay, with times in the display time zone (IR262)", () => {
    const i = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
    const t = translator("ms");
    const one = slot("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z");
    const [row] = clientRows([{ projection: "summary", id: "job-1-aaaa", version: 1, unitId: "u1", type: "reactive", status: "requested", displayStatus: "requested", origin: "client_request", dueAt: "2026-09-20T00:00:00Z", requestedSlot: one, scheduledSlot: null, preferredSlots: [one, one, one], assignmentAcknowledgement: null }], () => ({ name: "Bedroom AC", place: "" }), i);
    expect([row.type, row.line]).toEqual(["Pembaikan", "◷ Pilihan 16 Sep, 11:00 PG – 1:00 PTG GMT+9 · +2 lagi (belum disahkan)"]);
    expect(preferredError([one, { startAt: "", endAt: "" }, one], NOW, 3, t)).toBe("Masukkan tarikh dan kedua-dua masa bagi setiap masa pilihan.");
    expect(contactError("call 0123456789", t)).toMatch(/^Tiada alamat e-mel/);
    expect(clientJobRefusal({ code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: {} }, t)).toBe("Tidak disimpan: permintaan telah berubah — keadaan terkini dipaparkan.");
    expect(followUpText("rework", t)).toBe("Kerja semula (percuma)");
  });

  it("writes multi-day slots and the history in the client's words", () => {
    const week = slot("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z");
    expect([slotShort(week), slotLong(week)]).toEqual(["14 Sept, 8:00 am – 20 Sept, 8:00 am MYT", "Mon, 14 Sept, 8:00 am – Sun, 20 Sept, 8:00 am MYT"]);
    expect([clientEventTitle("job.offered", { "job.offered": "Offer received from HQ" }), clientEventTitle("job.checked_in", { "job.checked_in": "Checked in on site" }), clientEventTitle("x.y", {})])
      .toEqual(["HQ booked a service partner", "Checked in on site", "x y"]);
  });
});

