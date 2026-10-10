import { describe, expect, it } from "vitest";
import {
  conditionText, defaultRuleRows, durationText, policyCards, policyErrors, policyForm, policyInput, policyRefusal, summaryText, toggledInput, windowText,
  type ApiAlertPolicy, type ApiDefaultPolicy,
} from "@ac/web/lib/customerPolicies";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const rule = (ruleKey: string, name: string, over: Partial<ApiDefaultPolicy["rules"][number]> = {}): ApiDefaultPolicy["rules"][number] =>
  ({ ruleKey, name, category: "air_quality", metric: "co2", operator: "gte", threshold: 1000, recoveryThreshold: 900, durationSeconds: 600, activeWindow: null, severity: "warning", ...over });
const defaults: ApiDefaultPolicy = {
  id: "def", version: 1, kind: "default_alert", name: "Default policy",
  rules: [rule("ventilation_co2", "Ventilation"), rule("refrigerant_low_pressure", "Refrigerant low pressure", { category: "fault", metric: "refrigerant_pressure", operator: "lte", threshold: 350, recoveryThreshold: 380, durationSeconds: 120, severity: "critical" }),
    rule("heartbeat", "AC offline", { category: "connection", metric: "heartbeat_gap", threshold: 15, recoveryThreshold: 5, durationSeconds: 60 })],
  ruleSettings: [{ id: "s1", version: 2, ruleKey: "heartbeat", customerId: "cust-a", enabled: false, changedByMembershipId: "me", reason: null, updatedAt: "2026-09-28T02:00:00Z" },
    { id: "s2", version: 1, ruleKey: "ventilation_co2", customerId: "cust-b", enabled: false, changedByMembershipId: "x", reason: null, updatedAt: "2026-09-28T02:00:00Z" }],
};
const policy = (over: Partial<ApiAlertPolicy> = {}): ApiAlertPolicy => ({
  id: "p1", version: 3, kind: "alert", name: "Stuffy office", unitIds: ["u1", "u2"], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 40, disabledReason: null, customerId: "cust-a",
  recipientMembershipIds: ["owner"], channels: ["inApp"], escalateAfterMinutes: 30, cooldownMinutes: 10, metric: "co2", operator: "gte", threshold: 1200, recoveryThreshold: 1000,
  durationSeconds: 900, activeWindow: { weekdays: [1, 2, 3, 4, 5], startLocal: "08:00", endLocal: "19:00" }, severity: "warning", ...over,
});

describe("customer alert policies", () => {
  it("lists the default rules with this customer's switches (Figma 06e)", () => {
    const d = defaultRuleRows(defaults, "cust-a", "me");
    expect(d.rows.map((r) => [r.name, r.condition, r.type, r.severity, r.enabled, r.version])).toEqual([
      ["Ventilation", "CO₂ ≥ 1000 ppm for 10 min", "Air quality", "warning", true, 0],
      ["Refrigerant low pressure", "Refrigerant pressure ≤ 350 kPa for 2 min", "Fault cause", "critical", true, 0],
      ["AC offline", "No heartbeat for 15 min", "Connection", "warning", false, 2],
    ]);
    expect([d.on, d.notes]).toEqual([2, ["Turned off: “AC offline” — no alerts from this rule on any of your ACs (turned off by you, 28 Sept 2026). HQ still sees device status."]]);
    // in Malay the rule names stay HQ's; the rest follows the language (IR261)
    const ms = defaultRuleRows(defaults, "cust-a", "someone-else", i18nOf({ locale: "ms", timeZone: "Asia/Kuala_Lumpur" }));
    expect([ms.rows[0].name, ms.rows[0].condition, ms.rows[0].type, ms.rows[2].condition, ms.notes[0]]).toEqual(["Ventilation", "CO₂ ≥ 1000 ppm selama 10 min", "Kualiti udara", "Tiada denyutan selama 15 min",
      "Dimatikan: “AC offline” — tiada amaran daripada peraturan ini pada mana-mana AC anda (dimatikan oleh pemilik akaun, 28 Sep 2026). HQ masih melihat status peranti."]);
    expect([durationText(60), durationText(90), durationText(10800), windowText({ weekdays: [6, 7], startLocal: "09:00", endLocal: "12:00" }), windowText({ weekdays: [1, 2, 3, 4, 5, 6, 7], startLocal: "22:00", endLocal: "06:00" })])
      .toEqual(["1 min", "90 s", "3 h", "only 09:00–12:00 on weekends", "only 22:00–06:00"]);
    expect(conditionText({ metric: "airflow_drop", operator: "gte", threshold: 30, durationSeconds: 3600 })).toBe("Airflow drop ≥ 30 % for 1 h");
  });

  it("writes each own policy's When and Then and its attached ACs", () => {
    const [c] = policyCards([policy({ channels: ["inApp", "email"] })], (id) => ({ u1: "Workstations AC 1", u2: "Workstations AC 2" })[id]);
    expect([c.badge, c.when, c.then, c.attachedText]).toEqual(["Air quality", "CO₂ ≥ 1200 ppm for 15 min · recover below 1000 ppm · only 08:00–19:00 on weekdays", "Warning · notify in-app + email", "Attached to 2 ACs: Workstations AC 1, Workstations AC 2"]);
    expect(policyCards([policy({ unitIds: [], metric: "humidity", operator: "lte", threshold: 30, recoveryThreshold: 35, activeWindow: null, severity: "normal" })], () => undefined)[0])
      .toMatchObject({ when: "Humidity ≤ 30 % for 15 min · recover above 35 %", then: "Info · notify in-app", attachedText: "Not attached to any AC yet — attach it from an AC’s page" });
  });

  it("checks the editor, says what it will do and keeps the hidden fields on edit (IR120)", () => {
    const f = policyForm(policy());
    expect([f.group, f.metric, f.duration, f.unit, f.windowOn, f.email]).toEqual(["air", "co2", "15", "min", true, false]);
    expect(summaryText(f)).toBe("Warn me when CO₂ stays ≥ 1200 ppm for 15 min on weekdays 08:00–19:00. Clears below 1000 ppm.");
    expect(summaryText({ ...policyForm(), severity: "critical" })).toBe("Alert me (critical) when room temperature stays ≥ 30 °C for 1 min. Clears below 28 °C.");
    expect(policyErrors(f)).toEqual({});
    expect(policyErrors({ ...f, name: " ", recovery: "1300", duration: "0", windowOn: true, weekdays: [] })).toEqual({
      name: "Name is 1–120 characters.", recovery: "Recovery 1300 must be below the 1200 threshold (direction for ≥)", duration: "Between 1 s and 24 h.", window: "Pick at least one day and two different times.",
    });
    const edit = policyInput({ ...f, threshold: "1100", email: true }, policy(), { customerId: "cust-a", membershipId: "member", timezone: "UTC" });
    expect(edit).toMatchObject({ id: "p1", threshold: 1100, durationSeconds: 900, channels: ["inApp", "email"], recipientMembershipIds: ["owner"], escalateAfterMinutes: 30, cooldownMinutes: 10, timezone: "Asia/Kuala_Lumpur", priority: 40 });
    const fresh = policyInput({ ...policyForm(), name: "Bedroom too hot" }, null, { customerId: "cust-a", membershipId: "member", timezone: "Asia/Kuala_Lumpur" });
    expect(fresh).toEqual({ kind: "alert", name: "Bedroom too hot", customerId: "cust-a", timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 50, recipientMembershipIds: ["member"], escalateAfterMinutes: 60, cooldownMinutes: 5,
      channels: ["inApp"], metric: "temperature", operator: "gte", threshold: 30, recoveryThreshold: 28, durationSeconds: 60, activeWindow: null, severity: "warning" });
    expect(toggledInput(policy({ channels: ["inApp", "whatsapp"] }), false)).toMatchObject({ id: "p1", enabled: false, channels: ["inApp", "whatsapp"], durationSeconds: 900 });
    expect(policyRefusal({ code: "FORBIDDEN", messageKey: "error.ownerOnly", fieldErrors: {} })).toMatch(/account owner/);
    expect(policyRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { recoveryThreshold: "error.recoveryDirection" } })).toMatch(/safe side/);
    // the editor in Malay
    const t = translator("ms");
    expect(summaryText(f, t)).toBe("Ingatkan saya apabila CO₂ kekal ≥ 1200 ppm selama 15 min pada hari bekerja 08:00–19:00. Selesai di bawah 1000 ppm.");
    expect(policyErrors({ ...f, recovery: "1300", duration: "0" }, t)).toEqual({ recovery: "Pemulihan 1300 mesti di bawah ambang 1200 (arah untuk ≥)", duration: "Antara 1 s dan 24 j." });
    expect(policyCards([policy({ channels: ["inApp", "email"] })], () => "AC 1", t)[0]).toMatchObject({ badge: "Kualiti udara", then: "Peringatan · maklumkan melalui dalam aplikasi + e-mel", attachedText: "Dilampirkan pada 2 AC: AC 1, AC 1" });
    expect(policyRefusal({ code: "CONFLICT", messageKey: "", fieldErrors: {} }, t)).toBe("Tidak disimpan: polisi ini telah berubah — versi terkini dipaparkan.");
  });
});
