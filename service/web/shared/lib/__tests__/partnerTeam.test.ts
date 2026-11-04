import { describe, expect, it } from "vitest";
import { clearEffect, clearError, dayRange, grantRows, listed, memberRows, teamQuery, teamStats, technicianNames, unavailabilityConflicts, unavailabilityErrors, unavailabilityRefusal, unavailabilityRows, weekday, weekRows, type ApiTeamMember, type ApiUnavailability, type TeamJob } from "@ac/web/lib/partnerTeam";
import type { ApiCapacity } from "@ac/web/lib/partnerOverview";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const ms = translator("ms");
const NOW = Date.parse("2026-09-15T01:30:00Z"); // Tue 09:30 in Kuala Lumpur
const grant = (code: string, validUntil = "2027-03-31T00:00:00Z", revokedAt: string | null = null) => ({ validFrom: "2025-04-01T00:00:00Z", code, validUntil, revokedAt });
const member = (id: string, name: string, quals: ReturnType<typeof grant>[], validUntil: string | null = null): ApiTeamMember =>
  ({ id, validUntil, userId: `u-${id}`, organizationId: "org-a", displayName: name, role: "technician", employment: "external", validFrom: "2025-04-01T00:00:00+08:00", qualifications: quals });
const a = member("m-a", "tech-external-a", [grant("demo_outdoor"), grant("demo_electrical", "2026-10-01T00:00:00Z")]);
const a2 = member("m-b", "tech-external-a2", [grant("demo_indoor", "2027-01-31T00:00:00Z")]);
const old = member("m-c", "tech-external-old", [grant("demo_outdoor", "2026-08-01T00:00:00Z")], "2026-08-01T00:00:00+08:00");
const slot = (s: string, e: string) => ({ startAt: s, endAt: e });
const cap = (id: string, date: string, assigned: [string, string][], available: number | null = 480, unavailability: string | null = null): ApiCapacity => {
  return {
    membershipId: id,
    availableSlots: available ? [slot(`${date}T01:00:00Z`, `${date}T09:00:00Z`)] : [],
    assignedSlots: assigned.map(([s, e]) => slot(s, e)),
    availableMinutes: available,
    assignedMinutes: assigned.reduce((m, [s, e]) => m + (Date.parse(e) - Date.parse(s)) / 60_000, 0),
    date,
    unavailability,
  };
};
const dates = ["2026-09-14", "2026-09-15", "2026-09-16", "2026-09-17", "2026-09-18", "2026-09-19", "2026-09-20"];
const week = dates.map((d, k) => {
  return k >= 5 ? [cap("m-a", d, [], 0), cap("m-b", d, [], 0)] : [
    cap("m-a", d, k === 1 ? [["2026-09-15T01:00:00Z", "2026-09-15T05:00:00Z"]] : []),
    cap("m-b", d, k === 1 ? [["2026-09-15T05:00:00Z", "2026-09-15T09:00:00Z"]] : [], 480, k === 3 ? "annual_leave" : null),
  ];
});
const job: TeamJob = { jobId: "job-p12-full", short: "job-p12", technicianId: "m-b", slot: slot("2026-09-15T05:00:00Z", "2026-09-15T09:00:00Z"), text: "15 Sept, 1:00 pm – 5:00 pm MYT" };

describe("partner team & capacity", () => {
  it("reads the URL's conditions with today in Kuala Lumpur", () => {
    expect(teamQuery({}, "2026-09-14T17:00:00Z")).toEqual({ date: "2026-09-15", today: "2026-09-15", qualification: null, activeOnly: true }); // 01:00 on the 15th in KL
    expect(teamQuery({ date: "2026-09-19", qualification: "demo_indoor", activeOnly: "false" }, "2026-09-15T01:30:00Z")).toMatchObject({ date: "2026-09-19", qualification: "demo_indoor", activeOnly: false });
    expect(teamQuery({ date: "19/09/2026", qualification: "gas_leak" }, "2026-09-15T01:30:00Z")).toMatchObject({ date: "2026-09-15", qualification: null });
    expect(weekday("2026-09-15", "en")).toBe("Tue");
    expect(weekday("2026-09-15", "ms")).toBe("Sel");
  });

  it("lists the technicians of the conditions and counts the hidden expired ones", () => {
    const q = { date: "2026-09-15", qualification: null, activeOnly: true };
    expect(listed([a, a2, old], q, NOW)).toEqual({ rows: [a, a2], hidden: 1, expired: [] });
    expect(listed([a, a2, old], { ...q, activeOnly: false }, NOW)).toEqual({ rows: [a, a2, old], hidden: 0, expired: ["tech-external-old"] });
    expect(listed([a, a2, old], { ...q, qualification: "demo_indoor" }, NOW).rows).toEqual([a2]);
    expect(listed([a, a2], { ...q, date: "2026-10-05", qualification: "demo_electrical" }, NOW).rows).toEqual([]); // the grant ends on 1 Oct
  });

  it("shows each technician's week and the chosen day", () => {
    const rows = memberRows([a, a2, old], week, 1, [job], NOW);
    expect(rows[0]).toEqual({
      id: "m-a", name: "tech-external-a", sub: "Outdoor unit work · Electrical work · active since Apr 2025", active: true,
      week: { text: "4 h / 40 h", pct: 10 }, day: { main: "09:00–13:00 assigned", sub: "13:00–17:00 free" }, util: "50%",
    });
    expect(rows[1].day).toEqual({ main: "13:00–17:00 assigned (job-p12)", sub: "09:00–13:00 free" });
    expect(rows[2]).toMatchObject({ sub: "Outdoor unit work · expired 1 Aug 2026", active: false, day: { main: "Expired — cannot be assigned", tone: "crit" }, util: "—" });
    expect(memberRows([a], week, 5, [], NOW)[0]).toMatchObject({ day: { main: "No available hours set", sub: "—" }, util: "—" }); // Saturday
    expect(memberRows([a2], week, 3, [], NOW)[0]).toMatchObject({ day: { main: "Unavailable · annual leave", tone: "warn" }, util: "—" });
    expect(memberRows([a], week, 0, [], NOW)[0].day).toEqual({ main: "No assignments", sub: "09:00–17:00 free" });
  });

  it("builds the team's week and its totals", () => {
    const rows = weekRows([a, a2], week);
    expect(rows[0].cells.map((c) => c.text)).toEqual(["0 / 8 h", "4 / 8 h", "0 / 8 h", "0 / 8 h", "0 / 8 h", "—", "—"]);
    expect(rows[1].cells.slice(1, 4)).toEqual([{ text: "4 / 8 h", kind: "busy" }, { text: "0 / 8 h", kind: "free" }, { text: "annual leave", kind: "off" }]);
    expect(teamStats([a, a2], week, 1)).toEqual({ assigned: "8 h", available: "80 h", utilization: "10%", free: "8 h" });
    expect(teamStats([], week, 1)).toEqual({ assigned: "0 h", available: "0 h", utilization: "—", free: "0 h" });
  });

  it("states the qualification grants and the register qualifications nobody holds", () => {
    const revoked = member("m-d", "tech-external-a3", [grant("demo_outdoor", "2027-03-31T00:00:00Z", "2026-09-01T00:00:00Z")]);
    expect(grantRows([a, a2, old, revoked], NOW).map((g) => [g.name, g.sub, g.badge.text])).toEqual([
      ["Electrical work", "tech-external-a · expires 1 Oct 2026", "Expiring · 16 d"],
      ["Indoor unit work", "tech-external-a2 · valid to 31 Jan 2027", "Valid"],
      ["Outdoor unit work", "tech-external-a · valid to 31 Mar 2027", "Valid"],
      ["Outdoor unit work", "tech-external-a3 · revoked 1 Sept 2026", "Revoked"],
      ["Outdoor unit work", "tech-external-old · expired 1 Aug 2026", "Expired"],
    ]);
    expect(grantRows([a2], NOW).filter((g) => g.sub === "not held").map((g) => g.name)).toEqual(["Electrical work", "Outdoor unit work"]);
  });

  it("checks the unavailable days and finds the assignments they overlap", () => {
    const f = { membershipId: "m-b", from: "2026-09-15", to: "2026-09-15", type: "annual_leave", note: "" };
    expect(unavailabilityErrors(f)).toEqual({});
    expect(unavailabilityErrors({ ...f, from: "", to: "2026-09-14" })).toEqual({ from: "Choose the first day" });
    expect(unavailabilityErrors({ ...f, to: "2026-09-14" })).toEqual({ to: "The last day must be on or after the first day" });
    expect(unavailabilityErrors({ ...f, to: "2026-10-16", note: "x".repeat(501) })).toEqual({ to: "At most 31 days at a time", note: "At most 500 characters" });
    expect(unavailabilityErrors({ ...f, to: "2026-10-15" })).toEqual({}); // 31 days
    expect(unavailabilityConflicts([job], f)).toEqual([job]);
    expect(unavailabilityConflicts([job], { ...f, membershipId: "m-a" })).toEqual([]);
    expect(unavailabilityConflicts([job], { ...f, membershipId: "" })).toEqual([job]); // the whole team
    expect(unavailabilityConflicts([job], { ...f, from: "2026-09-16", to: "2026-09-16" })).toEqual([]);
    const late = { ...job, slot: slot("2026-09-15T15:30:00Z", "2026-09-15T16:30:00Z") }; // 23:30–00:30 in Kuala Lumpur
    expect(unavailabilityConflicts([late], { ...f, from: "2026-09-16", to: "2026-09-16" })).toEqual([late]);
  });

  it("reads the refusals of members.setUnavailability", () => {
    expect(unavailabilityRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { to: "error.range", type: "error.invalid" } })).toEqual({
      fields: { to: "The last day must be on or after the first day, at most 31 days", type: "Choose a type" }, text: null,
    });
    expect(unavailabilityRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} }).text).toBe("That technician is no longer in your company — reload the page.");
    expect(unavailabilityRefusal({ code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} })).toEqual({ fields: {}, text: null });
  });

  it("lists the upcoming unavailable days and says what a removal does (IR612)", () => {
    const signer = (id: string) => ({ id, displayName: "E2E Signer" });
    const names = technicianNames([a, signer("m-s1"), signer("m-s2"), signer("m-s3")], new Map([["m-s1", "s1@example.com"], ["m-s2", "s2@example.com"], ["m-a", "a@example.com"]]));
    expect([...names.values()]).toEqual(["tech-external-a", "E2E Signer · s1@example.com", "E2E Signer · s2@example.com", "E2E Signer"]); // no e-mail known: the name only
    const u = (id: string, membershipId: string | null, from: string, to: string, type = "annual_leave", note: string | null = null): ApiUnavailability => ({ version: 1, id, membershipId, from, to, type, note });
    const rows = unavailabilityRows([u("u2", "m-s1", "2026-09-17", "2026-09-18", "training", "Refresher"), u("u1", "m-a", "2026-09-10", "2026-09-16"), u("u3", null, "2026-09-21", "2026-09-21", "public_holiday"), u("u4", "m-gone", "2026-09-30", "2026-09-30")], names, "2026-09-15");
    expect(rows.map((r) => [r.who, r.type, r.note, r.days, r.first, r.started])).toEqual([
      ["tech-external-a", "Annual leave", null, "Thu 10 – Wed 16 Sept", "2026-09-15", true],
      ["E2E Signer · s1@example.com", "Training", "Refresher", "Thu 17 – Fri 18 Sept", "2026-09-17", false],
      ["All technicians", "Public holiday", null, "Mon, 21 Sept", "2026-09-21", false],
      ["Technician", "Annual leave", null, "Wed, 30 Sept", "2026-09-30", false],
    ]);
    expect(clearError(rows[0], "2026-09-14", "en")).toBe("Choose a day from Tue, 15 Sept to Wed, 16 Sept"); // the past days stay
    expect(clearError(rows[0], "2026-09-17", "en")).toBe("Choose a day from Tue, 15 Sept to Wed, 16 Sept");
    expect(clearError(rows[0], "", "en")).not.toBeNull();
    expect(clearError(rows[0], "2026-09-15", "en")).toBeNull();
    expect(clearEffect(rows[1], "2026-09-17", "en")).toBe("All of these days are removed.");
    expect(clearEffect(rows[1], "2026-09-18", "en")).toBe("Still unavailable: Thu, 17 Sept; the days from Fri, 18 Sept on are removed.");
    expect(clearEffect(rows[0], "2026-09-15", "en")).toBe("Still unavailable: Thu 10 – Mon 14 Sept; the days from Tue, 15 Sept on are removed. Days before today stay recorded.");
    expect(dayRange("2026-09-29", "2026-10-02", "ms")).toBe("Sel, 29 Sep – Jum, 2 Okt");
    expect(unavailabilityRows([u("u3", null, "2026-09-21", "2026-09-21", "public_holiday")], names, "2026-09-15", MS)[0]).toMatchObject({ who: "Semua juruteknik", type: "Cuti umum", days: "Isn, 21 Sep" });
    expect(clearEffect(rows[1], "2026-09-18", "ms", ms)).toBe("Masih tidak tersedia: Kha, 17 Sep; hari mulai Jum, 18 Sep dibuang.");
  });

  it("words the team in Malay with Kuala Lumpur days (IR276)", () => {
    const rows = memberRows([a, a2, old], week, 1, [job], NOW, MS);
    expect([rows[0].sub, rows[0].week.text, rows[0].day]).toEqual(["Kerja unit luaran · Kerja elektrik · aktif sejak Apr 2025", "4 j / 40 j", { main: "09:00–13:00 ditugaskan", sub: "13:00–17:00 lapang" }]);
    expect(rows[2].day.main).toBe("Tamat tempoh — tidak boleh ditugaskan");
    expect(memberRows([a], week, 5, [], NOW, MS)[0].day.main).toBe("Tiada jam tersedia ditetapkan");
    expect(weekRows([a2], week, ms)[0].cells[3]).toEqual({ text: "cuti tahunan", kind: "off" });
    expect(teamStats([a, a2], week, 1, ms).assigned).toBe("8 j");
    expect(grantRows([a], NOW, MS).map((g) => [g.name, g.sub, g.badge.text])).toEqual([
      ["Kerja elektrik", "tech-external-a · akan tamat 1 Okt 2026", "Hampir tamat · 16 hari"], ["Kerja unit dalaman", "tidak dipegang", "—"],
      ["Kerja unit luaran", "tech-external-a · sah hingga 31 Mac 2027", "Sah"],
    ]);
    expect(unavailabilityErrors({ membershipId: "", from: "2026-09-15", to: "2026-09-14", type: "other", note: "" }, ms)).toEqual({ to: "Hari terakhir mesti pada atau selepas hari pertama" });
  });
});
