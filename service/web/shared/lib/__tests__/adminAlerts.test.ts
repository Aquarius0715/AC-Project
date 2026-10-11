import { describe, expect, it } from "vitest";
import { adminAlertRows, alertPolicyInput, HQ_METRICS, newPolicyDraft, policyChanges, policyErrors, policyRows, recoveryError, type ApiAdminAlert, type ApiPolicy } from "@ac/web/lib/adminAlerts";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = Date.parse("2026-09-14T01:00:00Z"); // 09:00 in Kuala Lumpur
const alert = (over: Partial<ApiAdminAlert>): ApiAdminAlert => ({
  id: "alert-1", version: 1, unitId: "u1", policyId: "pol-1", type: "temperature", severity: "warning", status: "open", causeCode: "window_open", evidenceKind: "inferred",
  evidenceText: "cooling load rose", observedAt: "2026-09-14T00:50:00Z", evidenceIds: [], detectedAt: "2026-09-14T00:55:00Z", acknowledgedAt: null, resolvedAt: null, resolutionReason: null, ...over,
});
const units = [{ id: "u1", displayName: "Bedroom AC", customerOrgId: "org-a" }];
const customers = [{ id: "c-a", name: "Demo Customer A", organizationId: "org-a" }];
const policy = (over: Partial<ApiPolicy>): ApiPolicy => ({
  id: "pol-1", version: 2, kind: "alert", name: "Bedroom too hot", unitIds: ["u1", "u2"], timezone: "Asia/Kuala_Lumpur", enabled: true, priority: 50, customerId: "c-a",
  recipientMembershipIds: ["m-1"], channels: ["inApp"], escalateAfterMinutes: 60, cooldownMinutes: 5, metric: "temperature", operator: "gte", threshold: 30, recoveryThreshold: 28, durationSeconds: 60, severity: "warning", ...over,
});

describe("HQ alerts and alert policies", () => {
  it("orders the alerts unresolved first and tells each one's story", () => {
    const rows = adminAlertRows([alert({ id: "a-res", status: "resolved", resolvedAt: "2026-09-14T00:58:00Z", resolutionReason: "window closed" }), alert({ id: "a-crit", severity: "critical", causeCode: "unknown" }), alert({})],
      units, customers, [policy({})], NOW);
    expect(rows.map((r) => [r.id, r.state, r.st])).toEqual([["a-crit", "open", "Open"], ["alert-1", "open", "Open"], ["a-res", "resolved", "Resolved"]]);
    const r = rows[1];
    expect([r.title, r.time, r.meta, r.unit, r.evidence, r.cause, r.observed, r.detected, r.evidenceRecords]).toEqual([
      "Possible open window", "today 8:55 am MYT", "Demo Customer A · Bedroom AC · Bedroom too hot", "Bedroom AC · u1", "inferred — “cooling load rose”", "window_open (suspected)",
      "14 Sept 2026, 8:50 am MYT", "14 Sept 2026, 8:55 am MYT", "none attached",
    ]);
    expect(r.timeline.map((x) => x.title)).toEqual(["Alert detected", "Waiting for acknowledgement"]);
    expect([rows[0].cause, rows[2].timeline.at(-1)]).toEqual(["not determined", { time: "today 8:58 am MYT", title: "Resolved", detail: "window closed", tone: "ok" }]);
    const ack = adminAlertRows([alert({ status: "acknowledged", acknowledgedAt: "2026-09-14T00:57:00Z", evidenceIds: ["e1", "e2"], policyId: null })], [], [], [], NOW)[0];
    expect([ack.meta, ack.evidenceRecords, ack.timeline.map((x) => x.title)]).toEqual(["unknown customer · u1 · no policy", "2 attached", ["Alert detected", "Acknowledged", "Acknowledged — waiting for resolution"]]);
    // the links of the detail: the unit and its customer (customers.id), and whether a policy or an inference raised it (IR318)
    expect([r.unitId, r.customerId, r.policyless, r.inferred]).toEqual(["u1", "c-a", false, true]);
    expect([ack.customerId, ack.policyless]).toEqual([null, true]);
    expect(adminAlertRows([alert({ evidenceKind: "inspection" })], units, customers, [], NOW)[0].inferred).toBe(false);
  });

  it("lists the policies, the default first, with their condition in words", () => {
    const rows = policyRows([policy({}), policy({ id: "def", kind: "default_alert", name: "Default policy", customerId: null, rules: [{ ruleKey: "co2", name: "CO₂" }] }), policy({ id: "auto", kind: "automation" })],
      customers, [{ id: "m-1", displayName: "customer-a", role: "client" }]);
    expect(rows.map((r) => [r.name, r.group, r.sub, r.units])).toEqual([
      ["Default policy", "DEFAULT · ON EVERY UNIT", "1 rule · ventilation (CO₂, PM2.5) + fault causes", "every unit"], ["Bedroom too hot", "DEMO CUSTOMER A", "Temperature ≥ 30 °C for 60 s", "2 units"],
    ]);
    expect(rows[1].recipients).toEqual([{ id: "m-1", label: "customer-a", role: "client" }]);
    expect(alertPolicyInput(rows[1])).toMatchObject({ id: "pol-1", kind: "alert", recoveryThreshold: 28, durationSeconds: 60, recipientMembershipIds: ["m-1"] });
  });

  it("checks the policy form and offers only the metrics the Core API accepts", () => {
    const p = policyRows([policy({})], customers, [])[0];
    expect(policyErrors(p)).toEqual({});
    expect(policyErrors({ ...p, name: " ", duration: 0, cooldown: 2000, escalate: 0, recovery: 31 })).toEqual({
      name: "1–120 characters", duration: "Duration must be 1–86400 s", cooldown: "1–1440", escalate: "1–1440", recovery: "Recovery 31 must be below the 30 threshold (direction for ≥)",
    });
    expect([recoveryError("lte", 350, 380), recoveryError("lt", 350, 300)]).toEqual([undefined, "Recovery 300 must be above the 350 threshold (direction for <)"]);
    expect(HQ_METRICS).toEqual(["temperature", "humidity", "co2", "pm25", "refrigerant_pressure", "vibration", "power"]); // IR66 item 3
    // IR120 item 3: a whole priority 0–100 and 1–20 recipients
    expect(policyErrors({ ...p, priority: 50.5, recipients: [] })).toEqual({ priority: "A whole number 0–100", recipients: "Add at least one recipient" });
    expect(policyErrors({ ...p, priority: 101, recipients: Array.from({ length: 21 }, (_, n) => ({ id: `m-${n}`, label: "", role: "" })) })).toEqual({ priority: "A whole number 0–100", recipients: "At most 20 recipients" });
  });

  it("names who made a policy and the units it is attached to (Figma 38:5, IR345)", () => {
    const [r] = policyRows([policy({ ownerMembershipId: "m-hq" })], customers, [{ id: "m-hq", displayName: "hq-operator", role: "admin" }], undefined, [{ id: "u1", displayName: "Bedroom AC" }]);
    expect([r.madeBy, r.unitIds, r.unitNames]).toEqual(["hq-operator", ["u1", "u2"], ["Bedroom AC", "u2"]]); // a unit the list does not hold keeps its ID
    expect(policyRows([policy({ ownerMembershipId: "m-gone" })], customers, [])[0].madeBy).toBe(""); // members unreadable
  });

  it("starts a new policy as DD-A05 says and creates it without an id (IR345)", () => {
    const d = newPolicyDraft({ id: "c-a", name: "Demo Customer A" }, "Asia/Tokyo");
    expect([d.id, d.version, d.kind, d.customerId, d.group, d.timezone, d.on, d.priority, d.channels, d.recipients, d.unitIds]).toEqual(["", 0, "alert", "c-a", "DEMO CUSTOMER A", "Asia/Tokyo", false, 50, ["inApp"], [], []]);
    expect(policyErrors(d)).toEqual({ name: "1–120 characters", recipients: "Add at least one recipient" });
    const input = alertPolicyInput({ ...d, name: "Too hot", recipients: [{ id: "m-1", label: "customer-a", role: "client" }] });
    expect(input).not.toHaveProperty("id");
    expect(input).toEqual({
      kind: "alert", name: "Too hot", timezone: "Asia/Tokyo", enabled: false, priority: 50, customerId: "c-a", metric: "temperature", operator: "gte", threshold: 30, recoveryThreshold: 28,
      durationSeconds: 60, activeWindow: null, severity: "warning", recipientMembershipIds: ["m-1"], channels: ["inApp"], escalateAfterMinutes: 60, cooldownMinutes: 5,
    });
  });

  it("names what a draft changes against the saved policy", () => {
    const p = policyRows([policy({})], customers, [{ id: "m-1", displayName: "customer-a", role: "client" }])[0];
    expect(policyChanges(p, p)).toEqual([]);
    expect(policyChanges({ ...p, channels: ["inApp"] }, { ...p, channels: ["inApp"] })).toEqual([]);
    expect(policyChanges({ ...p, on: false, threshold: 31, recipients: [], channels: ["email", "inApp"], escalate: 30 }, p)).toEqual(["Enabled", "Condition", "Recipients", "Channels", "Escalation"]);
    expect(policyChanges({ ...p, channels: ["email", "inApp"] }, { ...p, channels: ["inApp", "email"] })).toEqual([]); // the order of the channels does not matter
    expect(policyChanges({ ...p, name: "x", cooldown: 9 }, p, MS.t)).toEqual(["Nama", "Tempoh bertenang"]);
  });
});

describe("HQ alerts in Malay with the display time zone (IR292)", () => {
  it("words the alerts and policies, with the times in GMT+9", () => {
    const r = adminAlertRows([alert({ status: "acknowledged", acknowledgedAt: "2026-09-14T00:57:00Z" })], units, customers, [policy({})], NOW, MS)[0];
    expect([r.title, r.st, r.cause, r.evidenceRecords]).toEqual(["Kemungkinan tingkap terbuka", "Diakui", "window_open (disyaki)", "tiada dilampirkan"]);
    expect([r.time, r.detected]).toEqual(["hari ini 9:55 PG GMT+9", "14 Sep 2026, 9:55 PG GMT+9"]);
    expect(r.timeline.map((x) => x.title)).toEqual(["Amaran dikesan", "Diakui", "Diakui — menunggu penyelesaian"]);
    const p = policyRows([policy({})], customers, [{ id: "m-1", displayName: "customer-a", role: "admin" }], MS.t)[0];
    expect([p.sub, p.units, p.recipients[0].role]).toEqual(["Suhu ≥ 30 °C selama 60 s", "2 unit", "HQ"]);
    expect(policyErrors({ ...p, duration: 0 }, MS.t).duration).toBe("Tempoh mesti 1–86400 s");
  });
});
