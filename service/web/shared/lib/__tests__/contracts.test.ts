import { describe, expect, it } from "vitest";
import { contractRows, dayInstant, draftErrors, klDay, planWord, priceMinor, unitOptions, type ApiContractFull, type ContractDraft } from "@ac/web/lib/contracts";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const contract = (over: Partial<ApiContractFull>): ApiContractFull => ({
  id: "k1234567-aaaa", version: 2, activeRestrictionIds: [], hasUnresolvedRecovery: false, customerId: "c1", customerOrgId: "org-a", unitIds: ["u1", "u2"],
  planType: "rto", startAt: "2025-12-31T16:00:00Z", endAt: "2026-12-31T16:00:00Z", priceMinor: 12050, currency: "MYR", restrictionEligible: true, rulesVersion: "demo-v1", ...over,
});
const customers = [{ id: "c1", name: "Demo Customer A", organizationId: "org-a", status: "active" }];
const draft = (over: Partial<ContractDraft>): ContractDraft => ({ customerId: "c1", unitIds: ["u1"], planType: "rto", start: "2026-01-01", end: "2027-01-01", price: "120.50", currency: "MYR", restrictionEligible: true, rulesVersion: "demo-v1", ...over });

describe("HQ contracts (FR-A07, DD-A07)", () => {
  it("rows contracts with their Kuala Lumpur days for the inputs and as the user reads them", () => {
    const [a, b] = contractRows([contract({}), contract({ id: "k2", customerId: "cx", unitIds: ["u3"], planType: "general", restrictionEligible: false, rulesVersion: null })], customers);
    expect(a).toMatchObject({ plan: "RTO", cust: "Demo Customer A", units: "2 units", price: "120.50 MYR", start: "2026-01-01", end: "2027-01-01", period: "1 Jan 2026 → 1 Jan 2027" });
    expect([b.plan, b.cust, b.units]).toEqual(["General", "customer", "1 unit"]);
    expect(unitOptions(
      [{ id: "u1", customerOrgId: "org-a", propertyId: "p1", displayName: "Lobby AC", archived: false }, { id: "u9", customerOrgId: "org-a", propertyId: "p1", displayName: "Old", archived: true }, { id: "u5", customerOrgId: "org-x", propertyId: "p1", displayName: "Other", archived: false }],
      customers, [{ id: "p1", customerOrgId: "org-a", name: "Office A", archived: false }], [contract({})],
    )).toEqual([{ id: "u1", customerId: "c1", label: "Lobby AC", where: "Office A", otherContracts: ["k1234567-aaaa"] }]); // archived and other tenants' units left out
  });

  it("checks a draft before contracts.save and keeps an unchanged day's stored instant", () => {
    expect(draftErrors(draft({}))).toEqual({});
    expect(draftErrors(draft({ customerId: "", unitIds: [], start: "", end: "", price: "1.234", planType: "general", rulesVersion: "" }))).toEqual({
      customerId: "Choose a customer", unitIds: "Include at least one unit", start: "Required", end: "End must be after start", price: "A price ≥ 0 with at most 2 decimals",
      restrictionEligible: "Only RTO contracts can be restriction eligible", rulesVersion: "Required when eligible (1–64 characters)",
    });
    expect([klDay("2025-12-31T16:00:00Z"), dayInstant("2026-01-01", "2025-12-31T16:00:00Z"), dayInstant("2026-01-02", "2025-12-31T16:00:00Z"), dayInstant("2026-01-02")])
      .toEqual(["2026-01-01", "2025-12-31T16:00:00Z", "2026-01-02T00:00:00+08:00", "2026-01-02T00:00:00+08:00"]);
    expect([priceMinor("120.5"), priceMinor(" 0 "), priceMinor("19.99")]).toEqual([12050, 0, 1999]);
  });
});

describe("HQ contracts in Malay with the display time zone (IR300)", () => {
  it("words plans, rows and checks; the days stay Kuala Lumpur days", () => {
    const t = translator("ms");
    const [r] = contractRows([contract({ planType: "environment" })], [], MS);
    expect([r.plan, r.cust, r.units, r.start, r.period]).toEqual(["Alam sekitar", "pelanggan", "2 unit", "2026-01-01", "1 Jan 2026 → 1 Jan 2027"]); // not Tokyo's days
    expect([planWord("rto", t), planWord("general", t), planWord("energy", t)]).toEqual(["RTO", "Umum", "Tenaga"]);
    expect(draftErrors(draft({ unitIds: [], end: "2025-01-01" }), t)).toEqual({ unitIds: "Masukkan sekurang-kurangnya satu unit", end: "Tamat mesti selepas mula" });
  });
});
