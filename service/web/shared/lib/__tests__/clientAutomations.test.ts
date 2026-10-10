import { describe, expect, it } from "vitest";
import {
  actionLabel, actionOptions, apiErrors, consentCard, draftErrors, draftOf, eventTest, ruleCard, runClock, runText, saveInput, scheduleTest, summaryText, weekdaysText, whenText, type ApiAutomation, type Caps,
} from "@ac/web/lib/clientAutomations";
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
    expect(ruleCard(off, unit, null, false)).toMatchObject({ status: { text: "Disabled · consent revoked", tone: "warn" }, note: "Will not run: location consent withdrawn 10 Sept 2026. Existing commands are not cancelled.", place: "Bedroom AC (+1 more AC) · Home A › Bedroom", toggle: { allowed: false, hint: "Grant location consent first" } });
    expect(ruleCard(off, null, null, false, MS)).toMatchObject({ status: { text: "Dilumpuhkan · persetujuan ditarik balik" }, place: "AC tidak lagi tersedia", when: "Semua orang meninggalkan premis AC (peristiwa berlepas)" });
  });

  it("words the editor in both languages: summary, checks, options, tests and the consent card (IR264)", () => {
    const d = draftOf(schedule);
    expect(summaryText(d, "Bedroom AC", "Home A")).toBe("When Weekdays at 18:00 → Bedroom AC power ON. At 22:00 → Bedroom AC power OFF.");
    expect(summaryText({ ...d, endsNextDay: true, endLocal: "06:00", startAction: "temp:24" }, "Bedroom AC", "Home A", ms)).toBe("Apabila Hari bekerja pada 18:00 → tetapkan Bedroom AC kepada 24°C. Pada 06:00 (hari berikutnya) → Bedroom AC kuasa MATI.");
    expect(draftErrors({ ...d, name: "", weekdays: [], startLocal: "18:00", endLocal: "18:00" }, ms)).toEqual({ name: "1–120 aksara", weekdays: "Pilih sekurang-kurangnya satu hari", endLocal: "Mula dan tamat tidak boleh sama — bukan operasi 24 jam" });
    expect(apiErrors({ "draft.endsNextDay": "errors.over_24_hours", name: "error.length" }, ms)).toEqual({ endsNextDay: "Paling lama 24 jam", name: "1–120 aksara" });
    const opts = actionOptions({ control: true, modeControl: true, fanControl: false, temperature: { min: 24, max: 25, step: 1 }, modes: ["cool"], fanLevels: [] }, ms);
    expect(opts.map((g) => [g.group, g.options.map((o) => o.label)])).toEqual([["Kuasa", ["Kuasa HIDUP", "Kuasa MATI"]], ["Suhu", ["Tetapkan suhu 24°C", "Tetapkan suhu 25°C"]], ["Mod", ["Mod Sejuk"]]]);
    // the schedule test: a Monday start, its end, and the first day not selected (Saturday)
    const runs = [{ automationId: null, phase: "schedule_start" as const, at: "2026-09-18T10:00:00Z", action: { kind: "set_power" as const, power: true } }, { automationId: null, phase: "schedule_end" as const, at: "2026-09-18T14:00:00Z", action: { kind: "set_power" as const, power: false } }];
    expect(scheduleTest(runs, d, "Bedroom AC", MS).map((r) => [r.title, r.detail])).toEqual([
      ["Jum, 18 Sep, 18:00 MYT — sepadan", "Akan menghantar 1 arahan: Bedroom AC · Kuasa HIDUP"],
      ["Jum, 18 Sep, 22:00 MYT — tindakan akhir", "Akan menghantar 1 arahan: Bedroom AC · Kuasa MATI"],
      ["Sab, 19 Sep, 18:00 MYT — tidak sepadan", "Sabtu tidak dipilih — tiada apa-apa akan dihantar"],
    ]);
    expect(eventTest({ unitId: "u1", decision: "suppressed", ruleId: null, reason: "offline" }, "a1", new Map(), "Bedroom AC", { kind: "set_power", power: true }, "2026-09-14T01:00:00Z", MS))
      .toMatchObject({ title: "Isn, 14 Sep, 09:00 MYT — tidak dihantar", detail: "Tiada apa-apa akan dihantar: AC di luar talian" });
    expect(consentCard({ id: "c", version: 2, granted: true, grantedAt: "2026-09-10T02:00:00Z", revokedAt: null }, 1, 0, MS)).toMatchObject({ title: "Diberikan", text: "Diberikan 10 Sep 2026, 11:00 PG GMT+9 · digunakan hanya oleh 1 automasi lokasi · peristiwa lokasi demo (tiada sejarah GPS sebenar)." });
    expect([runClock("2026-09-14T14:00:00Z"), runClock("2026-09-14T14:00:00Z", "Asia/Tokyo")]).toEqual(["22:00 MYT", "23:00 GMT+9"]);
  });
});

describe("ventilation in a customer rule (DD-C05 “within unit capabilities”, IR314)", () => {
  const fresh: Caps = { control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: true, ventilationLevels: ["low", "high"] };
  it("offers ventilation on a model with a fresh-air function, and the AC settings only with control", () => {
    expect(actionOptions(fresh).map((g) => g.group)).toEqual(["Power", "Ventilation"]);
    expect(actionOptions(fresh)[1].options).toEqual([{ key: "vent:low", label: "Ventilate low" }, { key: "vent:high", label: "Ventilate high" }]);
    expect(actionOptions({ ...fresh, control: false }).map((g) => g.group)).toEqual(["Ventilation"]);
    expect(actionOptions({ ...fresh, ventilation: false }).map((g) => g.group)).toEqual(["Power"]);
  });
  it("edits a ventilating rule back unchanged and says what it does", () => {
    const rule = { ...base, kind: "event", condition: { type: "occupancy", occupied: true }, action: { kind: "ventilate", level: "high" } } as ApiAutomation;
    const d = draftOf(rule);
    expect(d.action).toBe("vent:high"); // was "fan:undefined", which the API refused on save
    expect(saveInput(d)).toMatchObject({ kind: "event", action: { kind: "ventilate", level: "high" } });
    expect(summaryText(d, "Bedroom AC", "Home A")).toBe("When someone is in the room → set Bedroom AC to ventilation high.");
    expect(summaryText(d, "Bedroom AC", "Home A", ms)).toContain("pengudaraan tinggi");
  });
});
