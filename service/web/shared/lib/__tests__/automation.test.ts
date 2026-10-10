import { describe, expect, it } from "vitest";
import { translator } from "@ac/web/lib/i18n";
import {
  actionText, autoDraft, autoErrors, autoInput, conditionText, decisionWord, disabledReasonWord, evaluationInput, policyGroups, reasonWord,
  type ApiAutoPolicy, type AutoDraft,
} from "@ac/web/lib/automation";

const policy = (over: Partial<ApiAutoPolicy>): ApiAutoPolicy => ({
  id: "p1", version: 2, kind: "automation", name: "Peak saver", unitIds: ["u1"], ownerMembershipId: "m-hq", timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 60,
  disabledReason: null, condition: { type: "tariff", operator: "gt", value: 0.6, unit: "MYR_per_kWh" }, action: { kind: "set_temperature", celsius: 26 }, ...over,
});
const customerOf = new Map([["u1", "Demo Customer A"], ["u2", "Demo Customer B"]]);

describe("HQ automation policies (FR-A11, DD-A11)", () => {
  it("words every condition and action as one sentence", () => {
    expect([
      conditionText({ type: "occupancy", occupied: true }), conditionText({ type: "peak", active: false }), conditionText({ type: "tariff", operator: "gte", value: 0.6, unit: "MYR_per_kWh" }),
      conditionText({ type: "solar", operator: "lt", value: 2, unit: "kW" }), conditionText({ type: "battery", operator: "lte", value: 1.5, unit: "kW" }),
    ]).toEqual(["When the room is occupied", "When no peak period is active", "When the electricity tariff is ≥ 0.6 MYR/kWh", "When solar output is < 2 kW", "When battery output is ≤ 1.5 kW"]);
    expect([
      actionText({ kind: "set_power", power: false }), actionText({ kind: "set_temperature", celsius: 26 }), actionText({ kind: "set_mode", mode: "dry" }),
      actionText({ kind: "set_fan", fanLevel: "high" }), actionText({ kind: "ventilate", level: "low" }),
    ]).toEqual(["turn the AC off", "set the temperature to 26 °C", "switch to dry mode", "set the fan to high", "ventilate at low"]);
  });

  it("groups policies by customer, policies across customers first, by priority then ID", () => {
    const groups = policyGroups([
      policy({ id: "p2", priority: 40 }), policy({}), policy({ id: "p3", unitIds: ["u1", "u2"], enabled: false, disabledReason: "consent_revoked" }), policy({ id: "p4", unitIds: ["u9"] }),
    ], customerOf);
    expect(groups.map((g) => [g.label, g.rows.map((r) => r.id)])).toEqual([["Across customers", ["p3"]], ["Demo Customer A", ["p1", "p2"]], ["unknown customer", ["p4"]]]);
    expect(groups[0].rows[0]).toMatchObject({ sentence: "When the electricity tariff is > 0.6 MYR/kWh → set the temperature to 26 °C", enabled: false, disabledReason: "consent revoked" });
  });

  it("checks a draft and builds the policies.save input", () => {
    const d = (over: Partial<AutoDraft>): AutoDraft => ({ ...autoDraft(), name: "Peak saver", unitIds: ["u1"], ...over });
    expect(autoErrors(d({}), { min: 16, max: 30 })).toEqual({});
    expect(autoErrors(d({ name: " ", priority: "101", timezone: "", unitIds: [], value: "-1", celsius: "35" }), { min: 16, max: 30 })).toEqual({
      name: "1–120 characters", priority: "An integer 0–100", timezone: "Required", unitIds: "Choose at least one unit", value: "A value ≥ 0", celsius: "Within 16–30 °C on every target unit",
    });
    expect(autoErrors(d({ celsius: "x" }), null).celsius).toBe("A temperature");
    expect(autoInput(d({ type: "peak", flag: true, actionKind: "ventilate", level: "mid" }), "p1")).toEqual({
      id: "p1", kind: "automation", name: "Peak saver", unitIds: ["u1"], timezone: "Asia/Kuala_Lumpur", enabled: false, priority: 50, condition: { type: "peak", active: true }, action: { kind: "ventilate", level: "mid" },
    });
    expect(autoDraft(policy({ condition: { type: "occupancy", occupied: false }, action: { kind: "set_fan", fanLevel: "high" } }))).toMatchObject({ type: "occupancy", flag: false, actionKind: "set_fan", level: "high", priority: "60" });
  });

  it("builds the evaluation input at the current minute; an empty value is a null fact", () => {
    const input = evaluationInput("occupancy", [{ unitId: "u1", value: "true", quality: "valid" }, { unitId: "u2", value: "", quality: "missing" }], new Date("2026-09-15T01:00:42Z"), "e1");
    expect(input).toEqual({
      eventId: "e1", occurredAt: "2026-09-15T01:00:00.000Z", phase: "condition", unitIds: ["u1", "u2"],
      facts: [
        { unitId: "u1", metric: "occupied", unit: "boolean", observedAt: "2026-09-15T01:00:00.000Z", quality: "valid", value: true },
        { unitId: "u2", metric: "occupied", unit: "boolean", observedAt: "2026-09-15T01:00:00.000Z", quality: "missing", value: null },
      ],
    });
    expect(evaluationInput("tariff", [{ unitId: "u1", value: "0.7", quality: "valid" }], new Date("2026-09-15T01:00:00Z"), "e2").facts[0]).toMatchObject({ metric: "tariff", unit: "MYR_per_kWh", value: 0.7 });
    expect([reasonWord("missing_data"), reasonWord("a_new_reason"), decisionWord("suppressed"), decisionWord("other"), disabledReasonWord("other")]).toEqual(["missing data — skipped", "a_new_reason", "suppressed", "other", "other"]);
  });
});

describe("HQ automation policies in Malay (IR303)", () => {
  it("words the sentences, groups, checks, reasons and decisions", () => {
    const t = translator("ms");
    expect(`${conditionText({ type: "tariff", operator: "gt", value: 0.6, unit: "MYR_per_kWh" }, t)} → ${actionText({ kind: "set_mode", mode: "cool" }, t)}`).toBe("Apabila tarif elektrik > 0.6 MYR/kWh → tukar ke mod sejuk");
    expect([actionText({ kind: "set_power", power: true }, t), actionText({ kind: "set_fan", fanLevel: "mid" }, t)]).toEqual(["hidupkan AC", "tetapkan kipas kepada sederhana"]);
    expect(policyGroups([policy({ unitIds: ["u1", "u2"] })], customerOf, t)[0].label).toBe("Merentas pelanggan");
    expect(autoErrors({ ...autoDraft(), name: "", unitIds: [] }, null, t)).toEqual({ name: "1–120 aksara", unitIds: "Pilih sekurang-kurangnya satu unit" });
    expect([reasonWord("restricted", t), decisionWord("selected", t), decisionWord("requested", t), disabledReasonWord("consent_revoked", t)]).toEqual(["disekat oleh sekatan aktif", "dipilih", "diminta", "persetujuan ditarik balik"]);
  });
});
