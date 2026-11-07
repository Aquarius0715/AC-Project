import { describe, expect, it } from "vitest";
import { alertChoices, alertView, durationText, refusal, ruleWords, selectAlert, type ApiTechAlert } from "@ac/web/lib/techAlerts";
import { EN, i18nOf } from "@ac/web/lib/i18n";
import { unitDetail } from "./unitFixture";

const BEDROOM_TOO_HOT = "Bedroom too hot";
const ALERT_TEMP_0 = "alert-temp-0";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = Date.parse("2026-09-14T01:35:00Z"); // 09:35 in Kuala Lumpur
const rule = { name: BEDROOM_TOO_HOT, metric: "temperature", operator: "gte" as const, threshold: 30, recoveryThreshold: 28, durationSeconds: 60 };
const base: ApiTechAlert = {
  id: "alert-temp-a", version: 1, unitId: "u1", type: "sensor", severity: "warning", status: "open", causeCode: "unknown", evidenceKind: "demo_observation",
  evidenceText: "temperature 30.4 °C", observedAt: "2026-09-14T01:12:00Z", detectedAt: "2026-09-14T01:12:00Z", acknowledgedAt: null, resolvedAt: null, resolutionReason: null,
  previousAlertId: ALERT_TEMP_0, policyId: "p1", rule,
};
const earlier: ApiTechAlert = { ...base, id: ALERT_TEMP_0, status: "resolved", detectedAt: "2026-09-02T02:00:00Z", resolvedAt: "2026-09-02T03:00:00Z", previousAlertId: null };
const unit = unitDetail({
  id: "u1", displayName: "Bedroom AC", location: { pathLabels: ["Home A", "1F", "Bedroom"], address: null, accessInstructions: null },
  latestMeasurements: [{ id: "m1", unitId: "u1", sensorId: "s", metric: "temperature", value: 30.4, unit: "°C", observedAt: "2026-09-14T01:35:00Z", origin: "measured", quality: "valid", qualityReason: null }],
});
const ctx = { alerts: [base, earlier], job: { id: "job-t07-0000", type: "preventive" }, canResolve: false, unit };

describe("technician alert evidence", () => {
  it("words the rule that raised the alert (IR284, D08)", () => {
    expect([durationText(60, EN.t), durationText(120, EN.t), durationText(600, EN.t), durationText(3600, EN.t), durationText(10_800, EN.t)]).toStrictEqual(["60 s", "2 min", "10 min", "60 min", "3 h"]);
    expect(ruleWords(rule, EN.t)).toStrictEqual({ policy: "Policy: Temperature ≥ 30.0 °C, recovery < 28.0 °C, duration 60 s", recovery: "< 28.0 °C for 60 s", raise: "Temperature ≥ 30.0 °C" });
    expect(ruleWords({ name: "Refrigerant low pressure", metric: "refrigerant_pressure", operator: "lte", threshold: 350, recoveryThreshold: 380, durationSeconds: 120 }, EN.t).policy)
      .toBe("Policy: Refrigerant pressure ≤ 350 kPa, recovery > 380 kPa, duration 2 min");
  });

  it("shows an open policy alert to a technician without alert.resolve", () => {
    const v = alertView(base, ctx, NOW);
    expect(v.banner).toStrictEqual({ title: "alert-te · Bedroom too hot", sub: "Policy: Temperature ≥ 30.0 °C, recovery < 28.0 °C, duration 60 s", tone: "crit", state: null });
    expect(v.cards).toStrictEqual([
      { kind: "Observed (demo)", title: "Evidence at detection", value: "temperature 30.4 °C", sub: "observed today 9:12 am MYT" },
      { kind: "Measured", title: "Latest reading", value: "30.4 °C", sub: "observed today 9:35 am MYT · good" },
    ]);
    expect(v.resolution).toStrictEqual({
      info: "Automatic resolution needs a remeasurement < 28.0 °C for 60 s — or a reason below with alert.resolve.", done: null,
      note: "If it recurs after it is resolved, a new alert is created that links back to this one.",
      forbidden: "Not resolved — you don’t have alert.resolve. The alert resolves automatically once a remeasurement stays < 28.0 °C for 60 s, or ask a permitted user.",
    });
    expect(v.history).toStrictEqual([{ id: "raised", time: "today 9:12 am MYT", title: "Raised", sub: "temperature 30.4 °C · Temperature ≥ 30.0 °C for 60 s", badge: { text: "Observed (demo)", tone: "primary" } }]);
    expect(v.related).toStrictEqual([
      ["Job", "job-t07- · Preventive maintenance"], ["Unit", "Bedroom AC · Home A › 1F › Bedroom"], ["Policy", BEDROOM_TOO_HOT], ["Recovery rule", "< 28.0 °C for 60 s"],
      ["Previous alert", "2 Sept 2026 · resolved"], ["You can resolve", "with a remeasurement only"],
    ]);
  });

  it("shows a resolved alert without a policy (IR66)", () => {
    const done: ApiTechAlert = { ...base, id: "alert-win-1", rule: null, policyId: null, evidenceKind: "inferred", status: "resolved", acknowledgedAt: "2026-09-14T01:25:00Z", resolvedAt: "2026-09-14T01:30:00Z", resolutionReason: "Window closed", previousAlertId: null };
    const v = alertView(done, { ...ctx, job: null, unit: null, canResolve: true }, NOW);
    expect([v.banner.sub, v.banner.tone, v.banner.state]).toStrictEqual(["Suspected · no policy — it resolves only with a reason", "ok", "Resolved · today 9:30 am MYT"]);
    expect(v.cards).toHaveLength(1); // no rule: no latest reading of its metric
    expect(v.resolution).toMatchObject({ info: null, done: "Resolved today 9:30 am MYT — Window closed. Acknowledged today 9:25 am MYT.", note: "Completing the job alone never resolves an alert. If the condition returns, a new alert is created that links back to alert alert-wi." });
    expect(v.history.map((h) => h.title)).toStrictEqual(["Resolved", "Acknowledged", "Raised"]);
    expect(v.related.map((r) => r[1])).toStrictEqual(["—", "u1", "No policy", "—", "none", "done today 9:30 am MYT"]);
    expect(alertView({ ...base, rule: null }, { ...ctx, canResolve: false }, NOW).related[5]?.[1]).toBe("no — ask a user with alert.resolve");
    expect(alertView(base, { ...ctx, canResolve: true }, NOW).related[5]?.[1]).toBe("with a reason, or by a remeasurement");
    expect(alertView({ ...base, rule: null }, { ...ctx, canResolve: true }, NOW).related[5]?.[1]).toBe("with a reason");
  });

  it("opens the URL's alert, else the worst unresolved, and words refusals", () => {
    const crit: ApiTechAlert = { ...base, id: "crit", severity: "critical", detectedAt: "2026-09-13T01:00:00Z" };
    expect([selectAlert([base, crit, earlier], ALERT_TEMP_0)?.id, selectAlert([base, crit, earlier])?.id, selectAlert([earlier])?.id, selectAlert([])]).toStrictEqual([ALERT_TEMP_0, "crit", ALERT_TEMP_0, null]);
    expect(alertChoices([earlier, base], NOW).map((c) => [c.id, c.title, c.sub])).toStrictEqual([["alert-temp-a", BEDROOM_TOO_HOT, "Open · today 9:12 am MYT"], [ALERT_TEMP_0, BEDROOM_TOO_HOT, "Resolved · 2 Sept 2026, 10:00 am MYT"]]);
    const forbidden = alertView(base, ctx, NOW).resolution.forbidden;
    expect(refusal({ code: "FORBIDDEN", messageKey: "error.forbidden" }, forbidden, EN.t)).toBe(forbidden);
    expect(refusal({ code: "FORBIDDEN", messageKey: "errors.assignment_ended" }, forbidden, EN.t)).toBe("Your work window on this unit has ended.");
    expect(refusal({ code: "VALIDATION", messageKey: "error.validation" }, forbidden, EN.t)).toBe("A resolution reason is required (1–1000 characters).");
    expect(refusal({ code: "CONFLICT", messageKey: "error.weird" }, forbidden, EN.t)).toBe("CONFLICT: Weird"); // IR380: the shared wording
  });

  it("words the evidence in Malay with the display time zone (IR284)", () => {
    expect(ruleWords(rule, MS.t).policy).toBe("Dasar: Suhu ≥ 30.0 °C, pemulihan < 28.0 °C, tempoh 60 s");
    const v = alertView(base, ctx, NOW, MS);
    expect(v.related.map((r) => r[0])).toStrictEqual(["Kerja", "Unit", "Dasar", "Peraturan pemulihan", "Amaran sebelumnya", "Anda boleh menyelesaikan"]);
    expect(v.cards[1]).toStrictEqual({ kind: "Diukur", title: "Bacaan terkini", value: "30.4 °C", sub: "diperhatikan hari ini 10:35 PG GMT+9 · baik" });
  });
});
