import { describe, expect, it } from "vitest";
import { actionLabel, ruleCard, runText, weekdaysText, whenText, type ApiAutomation } from "@ac/web/lib/clientAutomations";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const ms = translator("ms");
const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const base = { id: "a1", version: 1, createdAt: "", updatedAt: "2026-09-10T02:00:00Z", name: "Evening cool", unitIds: ["u1"], ownerMembershipId: "m", timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 1, disabledReason: null, onlyIf: [], lastRun: null };
const schedule = { ...base, kind: "schedule", weekdays: [1, 2, 3, 4, 5], startLocal: "18:00", endLocal: "22:00", endsNextDay: false, startAction: { kind: "set_power", power: true }, endAction: { kind: "set_power", power: false } } as ApiAutomation;
const unit = { name: "Bedroom AC", path: "Home A › Bedroom", property: "Home A" };

describe("customer automation sentences (FR-C04, FR-C05, IR260)", () => {
  it("names days and actions in English and Malay", () => {
    expect([weekdaysText([1, 2, 3, 4, 5]), weekdaysText([6, 7]), weekdaysText([3, 1], true), weekdaysText([1, 2, 3, 4, 5, 6, 7])]).toEqual(["Weekdays", "Weekends", "Mon & Wed", "Every day"]);
    expect([weekdaysText([1, 5], true, ms), actionLabel({ kind: "set_mode", mode: "dry" }, ms), actionLabel({ kind: "ventilate", level: "high" }, ms)]).toEqual(["Isn & Jum", "Mod Kering", "Pengudaraan tinggi"]);
  });

  it("states when a rule runs, with its only-if conditions", () => {
    expect(whenText(schedule, "Home A")).toBe("Weekdays at 18:00 (Asia/Kuala_Lumpur)");
    const event = { ...base, kind: "event", condition: { type: "location", event: "arrival" }, action: { kind: "set_power", power: true }, onlyIf: [{ type: "occupancy", occupied: true }, { type: "weekday", weekdays: [6, 7] }] } as ApiAutomation;
    expect(whenText(event, "Home A")).toBe("Arrival at Home A · Only if: someone is home and weekends");
    expect(whenText(event, "Home A", ms)).toBe("Tiba di Home A · Hanya jika: ada orang di rumah dan hujung minggu");
  });

  it("keeps a rule's own time zone for its runs and adds the abbreviation (IR44)", () => {
    expect(runText("2026-09-14T10:00:00Z")).toBe("Mon, 14 Sept, 18:00 MYT");
    expect(runText("2026-09-14T10:00:00Z", "Asia/Kuala_Lumpur", MS)).toBe("Isn, 14 Sep, 18:00 MYT");
  });

  it("builds the card: status, next run, a withdrawn consent and the toggle hint", () => {
    expect(ruleCard(schedule, unit, "2026-09-14T10:00:00Z", true)).toMatchObject({ status: { text: "On", tone: "ok" }, then: "Power ON · at 22:00 → Power OFF (end action)", place: "Bedroom AC · Home A › Bedroom · next run Mon, 14 Sept, 18:00 MYT", note: null });
    const off = { ...base, kind: "event", enabled: false, disabledReason: "consent_revoked", unitIds: ["u1", "u2"], condition: { type: "location", event: "departure" }, action: { kind: "set_power", power: false } } as ApiAutomation;
    expect(ruleCard(off, unit, null, false)).toMatchObject({ status: { text: "Disabled · consent revoked", tone: "warn" }, note: "Will not run: location consent withdrawn 2026-09-10. Existing commands are not cancelled.", place: "Bedroom AC (+1 more AC) · Home A › Bedroom", toggle: { allowed: false, hint: "Grant location consent first" } });
    expect(ruleCard(off, null, null, false, MS)).toMatchObject({ status: { text: "Dilumpuhkan · persetujuan ditarik balik" }, place: "AC tidak lagi tersedia", when: "Semua orang meninggalkan premis AC (peristiwa berlepas)" });
  });
});
