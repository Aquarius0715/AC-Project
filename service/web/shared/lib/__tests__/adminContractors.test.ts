import { describe, expect, it } from "vitest";
import {
  contractorRefusal, contractorRows, kpiTiles, nextMonthStart, pendingCertificates, profileErrors, profileFacts, rateAmount, rateCardErrors, rateCardView, technicianRows,
  type ApiCertificate, type ApiProfile, type ApiRateCard, type ApiTechnician,
} from "@ac/web/lib/adminContractors";

const NOW = Date.parse("2026-09-14T01:00:00Z"); // Mon 09:00 KL
const kpis = { period: { from: "2026-06-16T01:00:00Z", to: "2026-09-14T01:00:00Z" }, offerAcceptance: 92, arrivalInWindow: 96.2, firstTimeAccepted: 84, averageRating: 4.6, ratingCount: 12, reworkRate: 8 };
const profile = (over: Partial<ApiProfile>): ApiProfile => ({
  id: "p-a", version: 2, organizationId: "org-a", name: "Demo Contractor A", status: "active", registrationNo: "SSM 202301012345", serviceAreas: ["Kuala Lumpur", "Selangor"],
  contactEmail: "ops@contractor-a.example", insuranceValidUntil: "2027-01-31T00:00:00Z", delegation: { from: "2026-04-01T00:00:00Z", to: "2027-01-31T00:00:00Z" }, rateCardId: "rc-3",
  suspendedReason: null, kpis, createdAt: "2026-04-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z", ...over,
});
const tech = (over: Partial<ApiTechnician>): ApiTechnician => ({ id: "m-a", displayName: "tech-external-a", organizationId: "org-a", role: "technician", validUntil: null,
  qualifications: [{ code: "demo_indoor", validFrom: "2026-09-01T00:00:00Z", validUntil: "2027-09-01T00:00:00Z", revokedAt: null }], ...over });
const cert = (over: Partial<ApiCertificate>): ApiCertificate => ({ id: "c1", version: 1, membershipId: "m-a", organizationId: "org-a", code: "demo_indoor", name: "Indoor", number: "N-1",
  issuedAt: "2025-10-15T00:00:00Z", expiresAt: "2026-10-15T00:00:00Z", fileName: "cert.pdf", status: "expiring", renewalOf: null, createdAt: "2026-09-01T00:00:00Z", ...over });

describe("HQ contractor register", () => {
  it("lists every contractor organization, with or without a profile", () => {
    const rows = contractorRows([{ id: "org-b", name: "Demo Contractor B" }, { id: "org-a", name: "Demo Contractor A" }], [profile({}), profile({ id: "p-c", organizationId: "org-c", name: "C", status: "suspended" })], [tech({}), tech({ id: "m-a2" })]);
    expect(rows).toEqual([
      { id: "org-a", name: "Demo Contractor A", sub: "Kuala Lumpur, Selangor · 2 technicians", badge: { label: "Active", tone: "ok" } },
      { id: "org-b", name: "Demo Contractor B", sub: "no contractor profile yet · 0 technicians", badge: { label: "No profile", tone: "muted" } },
    ]);
    expect(contractorRows([{ id: "org-c", name: "C" }], [profile({ organizationId: "org-c", status: "suspended" })], [])[0]).toMatchObject({ sub: "Kuala Lumpur, Selangor · offers suspended", badge: { label: "Suspended", tone: "crit" } });
  });

  it("shows the KPI tiles and the profile facts", () => {
    expect(kpiTiles(kpis).map((t) => [t.label, t.value])).toEqual([["Offer acceptance", "92 %"], ["Arrival in window", "96 %"], ["Report accepted first time", "84 %"], ["Customer rating", "4.6 ★"], ["Rework rate", "8 %"]]);
    const empty = kpiTiles({ ...kpis, offerAcceptance: null, averageRating: null, ratingCount: 0, reworkRate: 12.5 });
    expect([empty[0].value, empty[0].sub, empty[3].value, empty[3].sub, empty[4].tone]).toEqual(["—", "no answered offers · 90 d", "—", "0 ratings (Client)", "warn"]);
    expect(profileFacts(profile({}), NOW)).toEqual([["Registration", "SSM 202301012345"], ["Service areas", "Kuala Lumpur, Selangor"], ["Delegation period", "2026-04-01 – 2027-01-31"], ["Contact", "ops@contractor-a.example"], ["Insurance", "valid to 2027-01-31"]]);
    expect(profileFacts(profile({ insuranceValidUntil: "2026-08-01T00:00:00Z" }), NOW)[4][1]).toBe("expired 2026-08-01");
    expect(profileFacts(profile({ insuranceValidUntil: null }), NOW)[4][1]).toBe("not recorded — the delegation runs a year");
  });

  it("shows the rate card in effect and the scheduled ones", () => {
    const cards: ApiRateCard[] = [
      { id: "rc-4", version: 4, contractorOrgId: "org-a", effectiveFrom: "2026-10-01T00:00:00Z", currency: "MYR", lines: [{ workType: "repair_base", amountMinor: 47000, note: null }] },
      { id: "rc-3", version: 3, contractorOrgId: "org-a", effectiveFrom: "2026-06-01T00:00:00Z", currency: "MYR", lines: [{ workType: "rework_deduction", amountMinor: 12000, note: "2nd return" }, { workType: "periodic_inspection", amountMinor: 38000, note: "per unit visit" }] },
    ];
    const v = rateCardView(cards, "rc-3", NOW);
    expect(v.current).toEqual({ title: "Rate card v3 (from 2026-06-01)", currency: "MYR", lines: [{ label: "Periodic inspection (per unit)", amount: "380.00", note: "per unit visit" }, { label: "Rework deduction (2nd return)", amount: "− 120.00", note: "2nd return" }] });
    expect(v.scheduled.map((s) => s.title)).toEqual(["v4 from 2026-10-01 · scheduled"]);
    expect(v.nextVersion).toBe(5);
    expect(rateCardView([], null, NOW)).toEqual({ current: null, scheduled: [], nextVersion: 1 });
    expect(rateAmount({ workType: "emergency", amountMinor: 65000 })).toBe("650.00");
  });

  it("names each technician's certificate state", () => {
    const rows = technicianRows([
      tech({}), tech({ id: "m-a2", displayName: "tech-external-a2" }), tech({ id: "m-b", displayName: "tech-external-b", validUntil: "2026-09-01T00:00:00Z" }),
      tech({ id: "m-c", displayName: "tech-c", qualifications: [{ code: "demo_indoor", validFrom: "2026-01-01T00:00:00Z", validUntil: "2026-10-01T00:00:00Z", revokedAt: null }] }),
      tech({ id: "m-d", displayName: "tech-d" }),
    ], [cert({}), cert({ id: "c2", membershipId: "m-d", status: "pending_verification" })], NOW);
    expect(rows.map((r) => [r.name, r.sub, r.badge.label])).toEqual([
      ["tech-external-a", "Indoor", "1 expiring · 10-15"], ["tech-external-a2", "Indoor", "Valid"], ["tech-external-b", "membership ended 09-01", "Expired"],
      ["tech-c", "Indoor", "expiring · 10-01"], ["tech-d", "Indoor", "1 awaiting verification"],
    ]);
    expect(pendingCertificates([cert({}), cert({ id: "c2", status: "pending_verification" })]).map((c) => c.id)).toEqual(["c2"]);
  });

  it("checks the profile and rate card forms", () => {
    expect(profileErrors({ registrationNo: "SSM 1", serviceAreas: "Penang, Kedah", contactEmail: "ops@b.example" })).toEqual({});
    expect(profileErrors({ registrationNo: " ", serviceAreas: "Penang, penang", contactEmail: "nope" })).toEqual({ registrationNo: "1–64 characters.", serviceAreas: "1–20 different areas, comma-separated.", contactEmail: "A valid e-mail address." });
    const lines = [{ workType: "repair_base", amount: "450", note: "" }];
    expect(rateCardErrors({ effectiveFrom: "2026-10-01T00:00:00Z", lines }, NOW)).toEqual({});
    expect(rateCardErrors({ effectiveFrom: "2026-09-01T00:00:00Z", lines: [...lines, ...lines] }, NOW)).toEqual({ effectiveFrom: "Effective from a future date.", lines: "Each work type once." });
    expect(rateCardErrors({ effectiveFrom: "2026-10-01T00:00:00Z", lines: [{ workType: "emergency", amount: "-1", note: "" }] }, NOW).lines).toMatch(/non-negative/);
    expect(nextMonthStart(NOW)).toBe("2026-09-30T16:00:00.000Z"); // 2026-10-01 00:00 KL
    expect(contractorRefusal({ code: "CONFLICT", messageKey: "errors.profile_exists", fieldErrors: {} })).toBe("Not saved (CONFLICT): this organization already has a contractor profile.");
  });
});
