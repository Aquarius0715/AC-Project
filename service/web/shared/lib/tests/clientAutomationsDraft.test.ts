import { describe, expect, it } from "vitest";
import {
  actionOf, blankDraft, blankExtra, consentCard, draftErrors, draftOf, eventTest, extraChoices, firstAction, fitExtras, saveInput, scheduleTest, summaryText, testFact,
  type ActionGroup, type ApiAutomation, type ApiConsent, type Draft,
} from "@ac/web/lib/clientAutomations";

// The customer's automation editor (FR-C04, FR-C05, DD-C04, DD-C05): the actions as option keys and back, the "Only
// if" rows a trigger may add, every check before saving, the summary sentence for each trigger and action, the test
// result of a schedule and of an event, the synthetic fact a test sends, and the location consent card.

const draft = (over: Partial<Draft> = {}): Draft => ({ ...blankDraft("u-1", "Asia/Kuala_Lumpur"), name: "Evening", endAction: "power:off", ...over });
const base = { id: "a-1", version: 2, createdAt: "", updatedAt: "2026-09-10T02:00:00Z", name: "Rule", unitIds: ["u-1", "u-2"], ownerMembershipId: "m", timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 3, disabledReason: null, onlyIf: [], lastRun: null };

const eventSummary = (over: Partial<Draft>) => summaryText(draft({ action: "power:off", ...over }), "Bedroom AC", "Home A");
const consent = (over: Partial<ApiConsent>): ApiConsent => ({ id: "c-1", version: 1, granted: false, grantedAt: null, revokedAt: null, ...over });

describe("actions and the rules saved elsewhere", () => {
  it("reads every action key, falling back to a safe value for one it does not know", () => {
    expect([actionOf("power:on"), actionOf("power:off"), actionOf("temp:22.5"), actionOf("mode:dry"), actionOf("mode:bogus"), actionOf("vent:high"), actionOf("vent:x"), actionOf("fan:low"), actionOf("whatever")]).toStrictEqual([
      { kind: "set_power", power: true }, { kind: "set_power", power: false }, { kind: "set_temperature", celsius: 22.5 }, { kind: "set_mode", mode: "dry" }, { kind: "set_mode", mode: "cool" },
      { kind: "ventilate", level: "high" }, { kind: "ventilate", level: "mid" }, { kind: "set_fan", fanLevel: "low" }, { kind: "set_fan", fanLevel: "mid" },
    ]);
  });

  it("edits each saved action and condition back unchanged", () => {
    const actions = [{ kind: "set_mode", mode: "fan" }, { kind: "set_fan", fanLevel: "high" }, { kind: "ventilate", level: "low" }, { kind: "set_temperature", celsius: 26 }] as const;
    for (const action of actions) {
      const rule: ApiAutomation = { action, ...base, kind: "event", condition: { type: "occupancy", occupied: true } };
      expect(saveInput(draftOf(rule))).toMatchObject({ action, kind: "event", condition: { type: "occupancy", occupied: true } });
    }
    const conditions = [{ type: "location", event: "departure" }, { type: "pattern", localTime: "07:30" }, { type: "weather", metric: "temperature", operator: "lt", value: 18 }] as const;
    for (const condition of conditions) {
      const rule: ApiAutomation = { ...base, kind: "event", condition, action: { kind: "set_power", power: false }, onlyIf: [{ type: "weather", metric: "temperature", operator: "gte", value: 30 }] };
      expect(saveInput(draftOf(rule))).toMatchObject({ condition, unitIds: ["u-1", "u-2"] });
    }
  });
});

describe("the “Only if” rows", () => {
  it("offers the types a trigger may still add, each once and three at most", () => {
    expect(extraChoices(draft())).toStrictEqual(["occupancy", "weather"]); // a schedule has its own weekdays
    expect(extraChoices(draft({ trigger: "presence" }))).toStrictEqual(["weekday", "weather"]);
    expect(extraChoices(draft({ trigger: "weather", extras: [blankExtra("weekday")] }))).toStrictEqual(["occupancy"]);
    expect(extraChoices(draft({ trigger: "location", extras: [blankExtra("weekday"), blankExtra("occupancy"), blankExtra("weather")] }))).toStrictEqual([]);
    expect([blankExtra("weekday"), blankExtra("occupancy"), blankExtra("weather")]).toStrictEqual([{ type: "weekday", weekdays: [1, 2, 3, 4, 5] }, { type: "occupancy", occupied: true }, { type: "weather", operator: "gte", value: "30" }]);
  });

  it("drops the rows a new trigger no longer allows", () => {
    const extras = [blankExtra("weekday"), blankExtra("occupancy"), blankExtra("weather")];
    expect(fitExtras(draft({ extras })).map((x) => x.type)).toStrictEqual(["occupancy", "weather"]);
    expect(fitExtras(draft({ trigger: "presence", extras })).map((x) => x.type)).toStrictEqual(["weekday", "weather"]);
    expect(fitExtras(draft({ trigger: "routine", extras })).map((x) => x.type)).toStrictEqual(["weekday", "occupancy", "weather"]);
  });
});

describe("the checks before saving", () => {
  it("checks a schedule's name, AC, weekdays, end action and times", () => {
    expect(draftErrors(draft())).toStrictEqual({});
    expect(draftErrors(draft({ name: " ", unitId: "", weekdays: [], endAction: "", startLocal: "25:00", endLocal: "x" }))).toStrictEqual({
      name: "1–120 characters", unitId: "Choose the AC", weekdays: "Choose at least one weekday", endAction: "Choose what happens at the end", startLocal: "Choose a time", endLocal: "Choose a time",
    });
    expect(draftErrors(draft({ startLocal: "18:00", endLocal: "18:00" }))).toStrictEqual({ endLocal: "Start and end cannot be equal — not 24-hour operation" });
    expect(draftErrors(draft({ startLocal: "22:00", endLocal: "06:00" }))).toStrictEqual({ endsNextDay: "The end is before the start — tick “Ends next day” for an overnight schedule" });
    expect(draftErrors(draft({ startLocal: "22:00", endLocal: "06:00", endsNextDay: true }))).toStrictEqual({});
    expect(draftErrors(draft({ startLocal: "06:00", endLocal: "22:00", endsNextDay: true }))).toStrictEqual({ endsNextDay: "With “Ends next day” the end must be at or before the start (at most 24 hours)" });
  });

  it("checks a routine's time, an outdoor temperature, and the “Only if” rows", () => {
    expect(draftErrors(draft({ trigger: "routine", localTime: "7:30" }))).toStrictEqual({ localTime: "Choose a time" });
    expect([" ", "-51", "101", "abc"].map((value) => draftErrors(draft({ trigger: "weather", value })).value)).toStrictEqual(Array.from({ length: 4 }, () => "−50 to 100 °C"));
    expect(draftErrors(draft({ trigger: "weather", value: "-10" }))).toStrictEqual({});
    expect(draftErrors(draft({ trigger: "presence", extras: [{ type: "weekday", weekdays: [] }, { type: "weather", operator: "lt", value: "200" }] }))).toStrictEqual({
      extra0: "Choose at least one weekday", extra1: "−50 to 100 °C",
    });
  });
});

describe("the summary sentence", () => {
  it("says what each schedule action sets, overnight or not, and what is not chosen yet", () => {
    expect(summaryText(draft({ startAction: "mode:dry", endAction: "" }), "Bedroom AC", "Home")).toBe("When Weekdays at 18:00 → set Bedroom AC to mode dry. At 22:00 → (not chosen yet).");
    expect(summaryText(draft({ startAction: "fan:high", endAction: "vent:low", endsNextDay: true, weekdays: [] }), "Bedroom AC", "Home")).toBe("When no weekday at 18:00 → set Bedroom AC to fan high. At 22:00 (next day) → set Bedroom AC to ventilation low.");
    expect(summaryText(draft({ startAction: "power:on", endAction: "temp:26" }), "Bedroom AC", "Home")).toBe("When Weekdays at 18:00 → Bedroom AC power ON. At 22:00 → set Bedroom AC to 26°C.");
  });

  it("says when each event trigger fires, with its “Only if” rows", () => {
    expect(eventSummary({ trigger: "location", event: "arrival" })).toBe("When you arrive at Home A → Bedroom AC power OFF.");
    expect(eventSummary({ trigger: "location", event: "departure" })).toBe("When everyone leaves Home A → Bedroom AC power OFF.");
    expect(eventSummary({ trigger: "presence", occupied: true })).toBe("When someone is in the room → Bedroom AC power OFF.");
    expect(eventSummary({ trigger: "presence", occupied: false, extras: [{ type: "weather", operator: "gte", value: "" }] })).toBe("When no one is detected → Bedroom AC power OFF."); // an empty temperature row says nothing
    expect(eventSummary({ trigger: "routine", localTime: "07:30" })).toBe("When it is your usual time 07:30 → Bedroom AC power OFF.");
    expect(eventSummary({ trigger: "weather", operator: "lte", value: "20", extras: [{ type: "occupancy", occupied: true }] })).toMatch(/^When the outdoor temperature is ≤ 20°C → Bedroom AC power OFF\. Only if .+\.$/);
  });
});

describe("the test result", () => {
  const at = "2026-09-14T10:00:00Z";
  const names = new Map([["a-other", "Weekend cool"]]);

  it("says an event rule matches, loses to another rule, or would send nothing and why", () => {
    const off = { kind: "set_power", power: false } as const;
    expect(eventTest(undefined, "a-1", names, "Bedroom AC", off, at).title).toBe("No result");
    expect(eventTest({ unitId: "u-1", decision: "selected", ruleId: "a-1", reason: null }, "a-1", names, "Bedroom AC", off, at)).toMatchObject({ tone: "ok", detail: "Would send 1 command: Bedroom AC · Power OFF" });
    expect(eventTest({ unitId: "u-1", decision: "selected", ruleId: "a-other", reason: null }, "a-1", names, "Bedroom AC", off, at).detail).toMatch(/^“Weekend cool” has priority for Bedroom AC/);
    expect(eventTest({ unitId: "u-1", decision: "selected", ruleId: "a-gone", reason: null }, "a-1", names, "Bedroom AC", off, at).detail).toMatch(/^“Another automation/);
    expect(eventTest({ unitId: "u-1", decision: "suppressed", ruleId: null, reason: "offline" }, "a-1", names, "Bedroom AC", off, at).detail).toBe("Nothing would be sent: the AC is offline");
    expect(eventTest({ unitId: "u-1", decision: "suppressed", ruleId: null, reason: "new_reason" }, "a-1", names, "Bedroom AC", off, at).detail).toBe("Nothing would be sent: new_reason");
    expect(eventTest({ unitId: "u-1", decision: "suppressed", ruleId: null, reason: null }, "a-1", names, "Bedroom AC", off, at).detail).toBe("Nothing would be sent: no reason");
  });

  it("lists a schedule's start, end and next unselected day, or nothing without a start", () => {
    const runs = [
      { automationId: null, phase: "schedule_start" as const, at: "2026-09-18T10:00:00Z", action: { kind: "set_power", power: true } as const },
      { automationId: null, phase: "schedule_end" as const, at: "2026-09-18T14:00:00Z", action: { kind: "set_power", power: false } as const },
    ];
    const rows = scheduleTest(runs, draft(), "Bedroom AC");
    expect(rows.map((r) => r.tone)).toStrictEqual(["ok", "ok", "muted"]); // Friday start, end, then Saturday is not selected
    expect(rows[2]?.detail).toBe("Saturday is not selected — nothing would be sent");
    expect(scheduleTest(runs, draft({ weekdays: [1, 2, 3, 4, 5, 6, 7] }), "Bedroom AC")).toHaveLength(2);
    expect(scheduleTest([], draft(), "Bedroom AC")).toStrictEqual([]);
  });

  it("sends the trigger's own fact, none for a routine", () => {
    expect(testFact(draft({ trigger: "location", event: "departure" }))).toStrictEqual([{ metric: "location", value: "departure", unit: "event", quality: "valid" }]);
    expect(testFact(draft({ trigger: "presence", occupied: true }))).toStrictEqual([{ metric: "occupied", value: true, unit: "boolean", quality: "valid" }]);
    expect(testFact(draft({ trigger: "weather", value: "35" }))).toStrictEqual([{ metric: "weather_temperature", value: 35, unit: "°C", quality: "valid" }]);
    expect(testFact(draft({ trigger: "routine" }))).toStrictEqual([]);
  });
});

describe("the editor's defaults and the consent card", () => {
  it("starts a setting at 24 °C or the middle of its range, else its first value, and none without it", () => {
    const groups: ActionGroup[] = [
      { id: "temp", group: "Temperature", options: ["temp:18", "temp:20", "temp:22"].map((key) => ({ key, label: key, short: key })) },
      { id: "fan", group: "Fan", options: [{ key: "fan:low", label: "Fan low", short: "low" }] },
      { id: "mode", group: "Mode", options: [] },
    ];
    expect([firstAction(groups, "temp"), firstAction(groups, "fan"), firstAction(groups, "mode"), firstAction(groups, "vent")]).toStrictEqual(["temp:20", "fan:low", "", ""]);
    const with24: ActionGroup[] = [{ id: "temp", group: "Temperature", options: ["temp:18", "temp:24", "temp:30"].map((key) => ({ key, label: key, short: key })) }];
    expect(firstAction(with24, "temp")).toBe("temp:24");
  });

  it("words a consent never recorded, granted, withdrawn with or without rules turned off, and not granted", () => {
    expect(consentCard(null, 0, 0)).toMatchObject({ granted: false, title: "Not recorded", tone: "muted" });
    expect(consentCard(consent({ granted: true, grantedAt: "2026-09-01T00:00:00Z" }), 2, 0)).toMatchObject({ granted: true, title: "Granted", tone: "ok" });
    expect(consentCard(consent({ granted: true, grantedAt: "2026-09-01T00:00:00Z" }), 1, 0).text).toMatch(/used only by 1 location automation/);
    expect(consentCard(consent({ revokedAt: "2026-09-10T00:00:00Z" }), 0, 0)).toMatchObject({ title: "Revoked", tone: "muted" });
    expect(consentCard(consent({ revokedAt: "2026-09-10T00:00:00Z" }), 2, 1).text).toMatch(/1 location automation was turned off/);
    expect(consentCard(consent({ revokedAt: "2026-09-10T00:00:00Z" }), 2, 2)).toMatchObject({ title: "Revoked", tone: "warn" });
    expect(consentCard(consent({}), 0, 0)).toMatchObject({ title: "Not granted", tone: "muted" });
  });
});
