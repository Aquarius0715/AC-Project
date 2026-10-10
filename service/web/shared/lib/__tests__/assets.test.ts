import { describe, expect, it } from "vitest";
import {
  changedFields, claimCandidate, conditionText, coverageCsv, coverageKpis, coverageRows, customerRows, defaultRules, errorReportCsv, importMessage, placeOptions, policyLines,
  registerKpis, standingMarks, unitDraft, unitErrors, unitRows, type ApiCoverage, type ApiCustomerPolicy, type ApiImportRow, type ApiUnitRow, type Standing,
} from "@ac/web/lib/assets";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-14T01:00:00Z"); // 09:00 in Kuala Lumpur
const unit = (over: Partial<ApiUnitRow>): ApiUnitRow => ({
  id: "u1", version: 3, customerOrgId: "org-a", propertyId: "p1", spaceId: "s1", displayName: "Bedroom AC", modelId: "m1", type: "split", installedAt: "2026-01-09T16:00:00Z",
  serviceScope: ["indoor", "outdoor"], alertPolicyIds: [], archived: false, capabilityVersion: 2, connection: "online", lastSeenAt: null, warrantyEndsAt: "2027-01-09T16:00:00Z",
  effectivePowerState: "on", activeAlertCount: 0, ...over,
});
const standing = (over: Partial<Standing>): Standing => ({ plans: ["rto"], contracts: 1, contractUnits: 2, overdue: 0, overdueAmount: null, restriction: null, ...over });
const policy = (over: Partial<ApiCustomerPolicy>): ApiCustomerPolicy => ({
  id: "pol-1", version: 1, kind: "alert", name: "Bedroom too hot", unitIds: ["u1"], enabled: true, ownerMembershipId: "m-own", metric: "temperature", operator: "gte", threshold: 30, durationSeconds: 60, severity: "warning", ...over,
});

describe("HQ customers & units", () => {
  it("marks each customer's standing and counts the register", () => {
    expect([standingMarks("inactive", null), standingMarks("active", standing({ overdue: 1, restriction: { id: "r", state: "applied", contractId: "k" } })), standingMarks("active", standing({})), standingMarks("active", standing({ contracts: 0 }))].map((m) => m.map((x) => x.label)))
      .toEqual([["Inactive"], ["‼ Overdue", "Restriction applied"], ["Good standing"], ["No contract"]]);
    const rows = customerRows([{ id: "c1", version: 1, name: "Demo Customer A", status: "active", organizationId: "org-a", serviceProfile: "rto", createdAt: "2026-04-01T00:00:00Z" }],
      [{ id: "org-a", version: 2, name: "Customer A Sdn Bhd", kind: "customer", status: "active" }], [], [unit({}), unit({ id: "u2", effectivePowerState: "off" })], () => null);
    expect([rows[0].since, rows[0].operation, rows[0].billingName]).toEqual(["Apr 2026", "50.0%", "Customer A Sdn Bhd"]);
    expect(registerKpis(rows, [unit({}), unit({ id: "u2", effectivePowerState: "unknown" })])).toMatchObject({ customers: 1, propertyNames: "none yet", power: "Running 1 · Stopped 0 · unknown 1" });
  });

  it("lists units and checks the unit form", () => {
    const rows = unitRows([unit({ spaceId: null, alertPolicyIds: ["p1", "p2"] })], [], new Map([["m1", "ventilation-demo"]]));
    expect(rows[0]).toMatchObject({ location: "Unassigned", policies: "Default + 2", model: "ventilation-demo v2", power: "running" });
    const d = unitDraft(unit({}));
    expect([d.installedAt, d.warrantyEnd]).toEqual(["2026-01-10", "2027-01-10"]); // Kuala Lumpur days
    expect(unitErrors({ ...d, displayName: " ", installedAt: "2026-09-15", serviceScope: [] }, "2026-09-14")).toEqual({
      displayName: "1–120 characters", installedAt: "An installation date cannot be in the future (IR44)", serviceScope: "Choose at least one inspection group",
    });
    expect(changedFields(unit({}), { ...d, displayName: "Bedroom AC 2", spaceId: "" })).toEqual(["Name", "Location"]);
    expect(placeOptions([{ id: "p1", version: 1, kind: "home", name: "Home A", address: null, accessInstructions: null, updatedAt: "", units: 0, unassigned: 0, floors: 0, rooms: 0, spaces: [] }], [])[0].label).toBe("Home A (no space)");
  });

  it("words the policies' conditions and the default rules", () => {
    expect(conditionText(policy({ activeWindow: { weekdays: [1, 2, 3, 4, 5], startLocal: "08:00", endLocal: "19:00" } }))).toBe("Temperature ≥ 30 °C for 60 s, weekdays 08:00–19:00 → Warning");
    expect(conditionText(policy({ metric: "heartbeat_gap", threshold: 10, severity: undefined, activeWindow: { weekdays: [1, 3], startLocal: "22:00", endLocal: "06:00" } }))).toBe("No heartbeat for 10 min, Mon, Wed 22:00–06:00");
    const def = policy({ id: "def", kind: "default_alert", name: "Default policy", rules: [{ ruleKey: "co2", name: "CO₂ high", metric: "co2", operator: "gte", threshold: 1200, durationSeconds: 900, category: "ventilation" }],
      ruleSettings: [{ ruleKey: "co2", customerId: "c1", enabled: false, version: 2, reason: "office closed", updatedAt: "2026-09-10T02:00:00Z" }] });
    expect(defaultRules(def)[0]).toMatchObject({ condition: "CO₂ (ppm) ≥ 1200 ppm for 15 min", enabled: false, note: "Off since 10 Sept 2026 · office closed" });
    expect(policyLines([policy({}), def], ["u1"], () => "customer-a").map((p) => [p.name, p.type, p.condition, p.madeBy])).toEqual([
      ["Default policy", "Default", "1 rule · 0 on for this customer", "HQ (template)"], ["Bedroom too hot", "Temperature", "Temperature ≥ 30 °C for 60 s → Warning", "customer-a"],
    ]);
  });

  it("shows coverage with Kuala Lumpur warranty days and the import messages", () => {
    const cov = (over: Partial<ApiCoverage>): ApiCoverage => ({ unitId: "u1", customerId: "c1", modelId: "m1", warrantyEndsAt: "2026-10-09T16:00:00Z", contractIds: [], status: "expiring", claimableJobIds: [], ...over });
    const x = { unit: () => ({ name: "Bedroom AC", place: "Home A" }), customer: () => "Demo Customer A", model: () => "Daikin X", contract: (id: string) => `RTO until ${id}` };
    const rows = coverageRows([cov({}), cov({ unitId: "u2", status: "no_coverage", warrantyEndsAt: null })], x, NOW);
    expect(rows.map((r) => [r.ends, r.endsDate, r.statusText])).toEqual([["10 Oct 2026", "2026-10-10", "Ends in 26 d · no contract"], ["—", "", "No warranty · no contract"]]);
    expect(coverageCsv(rows).split("\n")[1]).toBe("Bedroom AC,u1,Demo Customer A,Daikin X,2026-10-10,,Ends in 26 d · no contract");
    expect(coverageKpis(rows)).toMatchObject({ underWarranty: 1, within30: 1, noCoverage: 1, contractNames: "none" });
    const row = (over: Partial<ApiImportRow>): ApiImportRow => ({ rowNumber: 2, propertyName: "Office A", floorName: "2F", roomName: null, unitName: "AC #2", modelCode: "SPL-100", serial: "SN-1", installedOn: null, warrantyEnd: null, result: "error", messageKey: "error.serialBound", ...over });
    expect([importMessage(row({})), importMessage(row({ messageKey: "warning.importCreatesSpace", result: "warning" })), importMessage(row({ messageKey: null, result: "ready" }))])
      .toEqual(["Serial SN-1 is already bound to another unit.", "“2F” does not exist — it will be created.", "Ready"]);
    expect(errorReportCsv([row({}), row({ rowNumber: 3, result: "ready", messageKey: null })]).split("\n")).toEqual(["row,property,floor,room,unit_name,model_code,serial,result,message", "2,Office A,2F,,AC #2,SPL-100,SN-1,error,Serial SN-1 is already bound to another unit.", ""]);
    expect(claimCandidate({ id: "job-1-aaaa", version: 2, type: "reactive", completedAt: "2026-09-01T03:00:00Z", costs: [] }, [], "Bedroom AC")).toMatchObject({ completed: "1 Sept 2026", parts: "parts listed in the report", amountMinor: null });
  });
});

describe("HQ customers & units in Malay with the display time zone (IR293)", () => {
  it("words the register, units, conditions and coverage", () => {
    expect(standingMarks("active", standing({ restriction: { id: "r", state: "requested", contractId: "k" } }), MS.t).map((m) => m.label)).toEqual(["Sekatan diminta"]);
    expect(registerKpis([], [], MS.t)).toMatchObject({ propertyNames: "belum ada", power: "Berjalan 0 · Berhenti 0 · tidak diketahui 0" });
    expect(unitRows([unit({ spaceId: null })], [], new Map(), MS.t)[0]).toMatchObject({ location: "Belum ditugaskan", policies: "Lalai" });
    expect(conditionText(policy({ activeWindow: { weekdays: [6, 7], startLocal: "08:00", endLocal: "12:00" } }), MS)).toBe("Suhu ≥ 30 °C selama 60 s, hujung minggu 08:00–12:00 → Peringatan");
    expect(conditionText(policy({ severity: undefined, durationSeconds: 900, activeWindow: { weekdays: [1, 3], startLocal: "22:00", endLocal: "06:00" } }), MS)).toMatch(/^Suhu ≥ 30 °C selama 15 min, Isn, Rab 22:00–06:00$/);
    const rows = coverageRows([{ unitId: "u1", customerId: "c1", modelId: "m1", warrantyEndsAt: "2026-10-09T16:00:00Z", contractIds: [], status: "expiring", claimableJobIds: [] }],
      { unit: () => undefined, customer: () => "A", model: () => "M", contract: () => "" }, NOW, MS);
    expect([rows[0].ends, rows[0].statusText]).toEqual(["10 Okt 2026", "Tamat dalam 26 h · tiada kontrak"]); // the Kuala Lumpur day, in Malay
    expect(unitErrors({ ...unitDraft(unit({})), modelId: "" }, "2026-09-14", MS.t)).toEqual({ modelId: "Pilih model" });
    expect(changedFields(unit({}), { ...unitDraft(unit({})), warrantyEnd: "" }, MS.t)).toEqual(["Tamat waranti"]);
  });
});
