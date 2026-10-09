import { describe, expect, it } from "vitest";
import {
  cleaningSymptom, filterLines, filterRefusal, filterRow, reminderErrors, reminderForm, reminderInput, reminderLines,
  type ApiFilterSettings, type ApiFilterStatus, type FilterUnit,
} from "@ac/web/lib/customerFilterCare";

const NOW = Date.parse("2026-09-14T01:00:00Z"); // Mon 09:00 KL
const st = (over: Partial<ApiFilterStatus>): ApiFilterStatus => ({
  unitId: "u1", runHoursSinceCleaning: 212, thresholdHours: 250, fallbackDays: 30, lastCleanedAt: "2026-08-30T02:00:00Z", lastCleanedBy: "customer", lastCleaningJobId: null, status: "due_soon", ...over,
});
const unit = (id: string, over: Partial<FilterUnit> = {}): FilterUnit => ({ id, name: `AC ${id}`, place: "Home A › 1F", spaceId: "s-1f", connection: "online", ...over });
const settings: ApiFilterSettings = { id: "", version: 0, customerId: "c", thresholdHours: null, fallbackDays: 30, recipients: "owners", channels: ["inApp"] };

describe("customer filter care", () => {
  it("shows run time, progress and the last cleaning per AC (Figma 07j)", () => {
    const r = filterRow(st({}), unit("u1", { name: "Bedroom AC" }), NOW);
    expect([r.unit, r.run, r.pct, r.progress, r.last, r.jobId, r.state]).toEqual(["Bedroom AC", "212 h since cleaning", 85, "85 % of 250 h", "last cleaned Aug 30", null, "due_soon"]);
    const over = filterRow(st({ runHoursSinceCleaning: 268, status: "overdue", lastCleanedAt: "2026-07-21T02:00:00Z" }), unit("u2"), NOW);
    expect([over.pct, over.progress, over.last]).toEqual([100, "100 % of 250 h", "last cleaned Jul 21"]);
    const tech = filterRow(st({ runHoursSinceCleaning: 40, status: "ok", lastCleanedAt: "2026-09-08T03:00:00Z", lastCleanedBy: "technician", lastCleaningJobId: "job-c02" }), unit("u3"), NOW);
    expect([tech.last, tech.jobId, tech.pct]).toEqual(["cleaned by technician Sep 8", "job-c02", 16]);
  });

  it("falls back to days without run time and never shows 0 for unknown (AT-C18-B)", () => {
    const days = filterRow(st({ runHoursSinceCleaning: null, lastCleanedAt: "2026-09-02T01:00:00Z", status: "ok" }), unit("u1", { connection: "offline" }), NOW);
    expect([days.run, days.runNote, days.pct, days.progress, days.offline]).toEqual(["—", "offline", 40, "40 % of 30 days", true]);
    const unknown = filterRow(st({ runHoursSinceCleaning: null, lastCleanedAt: null, lastCleanedBy: null, status: "unknown" }), unit("u1", { connection: "offline" }), NOW);
    expect([unknown.run, unknown.pct, unknown.progress, unknown.last]).toEqual(["—", null, "run time unknown", "no cleaning recorded yet"]);
    expect(filterRow(st({ runHoursSinceCleaning: 0, lastCleanedAt: "2026-09-14T01:00:00Z", status: "ok" }), unit("u1"), NOW).run).toBe("0 h since cleaning");
    expect(filterRow(st({ runHoursSinceCleaning: 12.4, lastCleanedAt: null, lastCleanedBy: null, status: "ok" }), unit("u1"), NOW).run).toBe("12 h so far");
    expect(filterRow(st({ runHoursSinceCleaning: null, lastCleanedAt: null, lastCleanedBy: null, status: "unknown" }), unit("u1"), NOW).runNote).toBe("no power readings");
  });

  it("folds an area with 4 or more ACs into a summary row where its most urgent AC is", () => {
    const area = (id: string, over: Partial<ApiFilterStatus>, conn = "online") => [st({ unitId: id, ...over }), unit(id, { place: "Office A › Open office", spaceId: "s-open", connection: conn })] as const;
    const g = [
      area("o1", { runHoursSinceCleaning: 230 }), area("o2", { runHoursSinceCleaning: 210 }), area("o3", { runHoursSinceCleaning: 100, status: "ok" }),
      area("o4", { runHoursSinceCleaning: null, lastCleanedAt: null, lastCleanedBy: null, status: "unknown" }, "offline"),
    ];
    const items = [st({ unitId: "m1", runHoursSinceCleaning: 268, status: "overdue" }), g[0][0], g[1][0], st({ unitId: "b1", status: "ok", runHoursSinceCleaning: 20 }), g[2][0], g[3][0]];
    const lines = filterLines(items, [unit("m1", { spaceId: "s-meet" }), unit("b1", { spaceId: null }), ...g.map((x) => x[1])], NOW);
    expect(lines.map((l) => (l.kind === "unit" ? l.row.unitId : l.group.key))).toEqual(["m1", "s-open", "b1"]);
    const sum = lines[1].kind === "group" ? lines[1].group : null;
    expect([sum?.title, sum?.place, sum?.run, sum?.pct, sum?.progress, sum?.badge, sum?.rows.length]).toEqual([
      "Open office · 4 ACs", "Office A › Open office", "2 due soon · 1 offline", 72, "72 % of 250 h (average)", { label: "2 due", tone: "warn" }, 4,
    ]);
  });

  it("writes the reminders and checks the dialog (AT-C18-E)", () => {
    expect(reminderLines(settings)).toEqual([["Remind at", "250 h (model default)"], ["Also remind", "every 30 days if run time unknown"], ["Send by", "App only"], ["Who", "Owners of the location"]]);
    expect(reminderLines({ ...settings, thresholdHours: 300, channels: ["inApp", "email"], recipients: "all_users" }).map((l) => l[1])).toEqual(["300 h of running", "every 30 days if run time unknown", "App + email", "All users of the location"]);
    const f = reminderForm(settings);
    expect([f.useDefault, f.hours, f.days, f.email]).toEqual([true, "250", "30", false]);
    expect(reminderErrors({ ...f, useDefault: false, hours: "20" })).toEqual({ hours: "Whole hours from 50 to 2000." });
    expect(reminderErrors({ ...f, days: "181" })).toEqual({ days: "Whole days from 7 to 180." });
    expect(reminderErrors({ ...f, useDefault: true, hours: "x" })).toEqual({});
    expect(reminderInput({ ...f, useDefault: false, hours: "120", email: true })).toEqual({ thresholdHours: 120, fallbackDays: 30, recipients: "owners", channels: ["inApp", "email"] });
    expect(reminderInput(f).thresholdHours).toBeNull();
  });

  it("prefills the cleaning request and names refusals", () => {
    expect(cleaningSymptom(filterRow(st({ runHoursSinceCleaning: 268, status: "overdue" }), unit("u1"), NOW))).toBe("Filter cleaning: 268 h of running since the last cleaning (reminder at 250 h).");
    expect(cleaningSymptom(filterRow(st({ runHoursSinceCleaning: null, lastCleanedAt: "2026-08-01T02:00:00Z" }), unit("u1"), NOW))).toBe("Filter cleaning: 43 days since the last cleaning (reminder every 30 days).");
    expect(cleaningSymptom(filterRow(st({ runHoursSinceCleaning: null, lastCleanedAt: null }), unit("u1"), NOW))).toBe("Filter cleaning: the filter is due for cleaning.");
    expect(filterRefusal({ code: "FORBIDDEN", messageKey: "error.ownerOnly", fieldErrors: {} })).toBe("Not saved: only the account owner can change the reminders.");
    expect(filterRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { thresholdHours: "error.range" } })).toMatch(/50–2000 h/);
    expect(filterRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { unitId: "error.unitArchived" } })).toBe("Not saved: that AC is archived.");
  });
});
