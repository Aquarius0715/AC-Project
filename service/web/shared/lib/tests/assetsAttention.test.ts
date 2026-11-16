import { describe, expect, it } from "vitest";
import { contractMatch, type CustomerRow, type Standing } from "@ac/web/lib/assets";

const STANDING: Standing = { plans: [], contracts: 1, contractUnits: 1, overdue: 0, overdueAmount: null, restriction: null };
const row = (alerts: number, standing: Standing | null, status: CustomerRow["status"] = "active"): CustomerRow => ({
  id: "c", version: 1, name: "C", billingName: "C", orgId: "o", orgVersion: 1, orgStatus: status, profile: "general", since: "2026-01-01",
  properties: 1, propertyNames: [], units: 1, operation: "—", running: 0, known: 0, marks: [], status, alerts, standing,
});

describe("contractMatch attention", () => {
  it("matches the customers the Needs attention tile counts", () => {
    expect(contractMatch("attention", row(1, null))).toBe(true);
    expect(contractMatch("attention", row(0, { ...STANDING, overdue: 1 }))).toBe(true);
    expect(contractMatch("attention", row(0, { ...STANDING, restriction: { id: "r", state: "applied", contractId: "k" } }))).toBe(true);
    expect(contractMatch("attention", row(0, STANDING))).toBe(false);
    expect(contractMatch("attention", row(2, null, "inactive"))).toBe(false);
  });
});
