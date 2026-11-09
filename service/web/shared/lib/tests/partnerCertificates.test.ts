import { describe, expect, it } from "vitest";
import { certDates, certificateErrors, certificateRefusal, certKpis, certQuery, certRows, fileSize, impact, type ApiCert, type CertForm, type CertJob } from "@ac/web/lib/partnerCertificates";
import type { ApiTeamMember } from "@ac/web/lib/partnerTeam";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import { must } from "./must";

const TECH_EXTERNAL_A = "tech-external-a";
const DEMO_OUTDOOR = "demo_outdoor";
const TEXT_2027_03_31 = "2027-03-31";
const DEMO_ELECTRICAL = "demo_electrical";
const TECH_EXTERNAL_B = "tech-external-b";
const ELECTRICAL_BASICS = "Electrical basics";
const DEMO_INDOOR = "demo_indoor";
const PENDING_VERIFICATION = "pending_verification";
const TEXT_2026_09_28 = "2026-09-28";
const TECH_EXTERNAL_A2 = "tech-external-a2";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const ms = translator("ms");
const NOW = Date.parse("2026-09-15T01:30:00Z"); // Tue 09:30 in Kuala Lumpur
const KL = (d: string) => `${d}T00:00:00+08:00`;
const member = (id: string, name: string, quals: { code: string; validUntil: string }[], validUntil: string | null = null): ApiTeamMember =>
  ({ id, validUntil, userId: `u-${id}`, organizationId: "org-a", displayName: name, role: "technician", employment: "external", validFrom: KL("2025-04-01"), qualifications: quals.map((q) => ({ ...q, validFrom: KL("2025-04-01"), revokedAt: null })) });
const a = member("m-a", TECH_EXTERNAL_A, [{ code: DEMO_OUTDOOR, validUntil: KL(TEXT_2027_03_31) }, { code: DEMO_ELECTRICAL, validUntil: KL("2026-10-15") }]);
const b = member("m-b", TECH_EXTERNAL_B, [], KL("2026-08-01"));
const cert = (id: string, over: Partial<ApiCert>): ApiCert => {
  return {
    id, version: 1, membershipId: "m-a", organizationId: "org-a", code: DEMO_ELECTRICAL, name: ELECTRICAL_BASICS, number: "EB-2023-1120", issuedAt: KL("2023-10-15"), expiresAt: KL("2026-10-15"),
    fileName: "eb.pdf", status: "valid", renewalOf: null, createdAt: "2023-10-15T00:00:00Z", verifiedAt: "2023-10-16T00:00:00Z", trainingRequestedAt: null, ...over,
  };
};
const certs = [
  cert("c1", { status: "expiring" }),
  cert("c2", { code: DEMO_INDOOR, name: "Split-unit refrigerant handling", number: "RH-2024-0811", issuedAt: KL("2024-03-31"), expiresAt: KL(TEXT_2027_03_31) }),
  cert("c3", { membershipId: "m-b", code: DEMO_OUTDOOR, name: "Refrigerant handling", number: "RH-2022-0310", issuedAt: KL("2022-09-01"), expiresAt: KL("2026-09-01"), status: "expired" }),
];
const renewal = cert("c4", { status: PENDING_VERIFICATION, renewalOf: "c1", number: "EB-2026-0931", issuedAt: KL(TEXT_2026_09_28), expiresAt: KL("2029-09-27"), createdAt: "2026-09-14T03:00:00Z" });
const height = (id: string, over: Partial<ApiCert>) => cert(id, { membershipId: "m-c", code: "other", name: "Working at height", ...over });
const job: CertJob = { jobId: "job-p07-full", short: "job-p07", technicianId: "m-a", slot: { startAt: "2026-10-20T01:00:00Z", endAt: "2026-10-20T04:00:00Z" }, text: "20 Oct, 9:00 am – 12:00 pm MYT", unit: "Server room AC", required: [DEMO_ELECTRICAL] };

describe("partner certifications", () => {
  it("reads the tab's conditions", () => {
    expect(certQuery({}, ["m-a"])).toStrictEqual({ membershipId: null, status: null, within: 60 });
    expect(certQuery({ membershipId: "m-a", status: "expiring", expiringWithinDays: "30" }, ["m-a"])).toStrictEqual({ membershipId: "m-a", status: "expiring", within: 30 });
    expect(certQuery({ membershipId: "m-x", status: "lost", expiringWithinDays: "45" }, ["m-a"])).toStrictEqual({ membershipId: null, status: null, within: 60 });
  });

  it("builds one row per technician and qualification with what it blocks", () => {
    const rows = certRows([a, b], certs, [job], 60, NOW);
    expect(rows.map((r) => [r.technician, r.name, r.detail, r.expires, r.badge.text, r.blocks.map((x) => x.text), r.actions])).toStrictEqual([
      [TECH_EXTERNAL_A, ELECTRICAL_BASICS, "EB-2023-1120 · issued 15 Oct 2023", "15 Oct 2026", "Expiring · 30 d", ["job-p07 (20 Oct)"], ["view", "renew", "training"]],
      [TECH_EXTERNAL_A, "Outdoor unit work", "granted by HQ · no certificate on file", "31 Mar 2027", "Valid", [], ["add"]],
      [TECH_EXTERNAL_A, "Split-unit refrigerant handling", "RH-2024-0811 · issued 31 Mar 2024", "31 Mar 2027", "Valid", [], ["view"]],
      [TECH_EXTERNAL_B, "Refrigerant handling", "RH-2022-0310 · issued 1 Sept 2022", "1 Sept 2026", "Expired", [], ["view"]], // the membership ended: no new upload
    ]);
    expect(certRows([a], certs, [job], 30, NOW)[0]?.badge.text).toBe("Expiring · 30 d");
    expect(certRows([a], certs, [job], 30, Date.parse("2026-09-10T00:00:00Z"))[0]).toMatchObject({ status: "valid", badge: { text: "Valid" } }); // 35 days left with a 30-day window
  });

  it("notes a pending renewal, a training request and a rejected first upload", () => {
    const [electrical] = certRows([a], [...certs, renewal], [job], 60, NOW);
    expect([electrical?.renewalPending, electrical?.actions, electrical?.cert]).toStrictEqual([true, ["view", "training"], { id: "c1", version: 1, number: "EB-2023-1120", fileName: "eb.pdf" }]); // the document opens from View (IR436)
    const trained = must(certRows([a], [cert("c1", { trainingRequestedAt: "2026-09-12T02:00:00Z" })], [], 60, NOW)[0]);
    expect([trained.training, trained.actions]).toStrictEqual(["Training requested 12 Sept 2026", ["view", "renew"]]);
    const first = certRows([member("m-c", TECH_EXTERNAL_A2, [])], [cert("c9", { membershipId: "m-c", code: "other", name: "Working at height", status: "rejected", createdAt: "2026-09-01T00:00:00Z" })], [], 60, NOW);
    expect(first.map((r) => [r.name, r.badge.text, r.actions])).toStrictEqual([["Working at height", "Rejected", ["view", "renew"]]]);
    const turnedDown = must(certRows([a], [cert("c7", { code: DEMO_OUTDOOR, name: "Outdoor licence", status: "rejected" })], [], 60, NOW).find((r) => r.code === DEMO_OUTDOOR));
    expect([turnedDown.detail, turnedDown.badge.text, turnedDown.actions]).toStrictEqual(["granted by HQ · the upload was rejected", "Valid", ["add"]]); // the HQ grant still stands
    const uploaded = must(certRows([a], [cert("c6", { code: DEMO_OUTDOOR, status: PENDING_VERIFICATION })], [], 60, NOW).find((r) => r.code === DEMO_OUTDOOR));
    expect([uploaded.detail, uploaded.badge.text, uploaded.renewalPending, uploaded.actions]).toStrictEqual(["granted by HQ · the upload waits for HQ", "Valid", true, []]);
    const waiting = certRows([member("m-c", TECH_EXTERNAL_A2, [])], [cert("c8", { membershipId: "m-c", code: DEMO_INDOOR, status: PENDING_VERIFICATION })], [], 60, NOW);
    expect(waiting.map((r) => [r.badge.text, r.actions, r.blocks])).toStrictEqual([["Pending HQ verification", ["view"], []]]);
  });

  it("keeps a twice-renewed other certificate in one row, whatever the list order (IR706)", () => {
    // certificates.list answers by expiry: the original, its renewal, then the renewal of that renewal
    const chain = [
      height("o1", { status: "expired", expiresAt: KL("2026-09-01") }),
      height("o2", { renewalOf: "o1", expiresAt: KL("2027-09-01") }),
      height("o3", { renewalOf: "o2", status: PENDING_VERIFICATION, expiresAt: KL("2028-09-01") }),
    ];
    for (const list of [chain, chain.toReversed()]) {
      const rows = certRows([member("m-c", TECH_EXTERNAL_A2, [])], list, [], 60, NOW).filter((r) => r.name === "Working at height");
      expect(rows).toHaveLength(1);
    }
  });

  it("names a qualification a booked job needs that the technician does not hold", () => {
    const a2 = member("m-c", TECH_EXTERNAL_A2, []);
    const gas = { ...job, technicianId: "m-c", required: [DEMO_OUTDOOR] };
    expect(certRows([a2], [], [gas], 60, NOW).map((r) => [r.name, r.detail, r.badge.text, r.blocks.map((x) => x.text), r.actions])).toStrictEqual([
      ["Outdoor unit work", "not held", "Not held", ["job-p07 (20 Oct)"], ["add"]],
    ]);
    expect(impact(certRows([a2], [], [gas], 60, NOW), [gas])?.text).toBe("tech-external-a2 does not hold Outdoor unit work. job-p07 (Server room AC, 20 Oct) needs it — add the certificate or reassign.");
  });

  it("counts the KPIs and states the assignment impact", () => {
    const rows = certRows([a, b], [...certs, renewal], [job], 60, NOW);
    expect(certKpis(rows, [...certs, renewal], 60)).toStrictEqual([
      { label: "Valid", value: 3, sub: "in date", chip: null },
      { label: "Expiring ≤ 60 days", value: 1, sub: null, chip: "tech-external-a · 30 d" },
      { label: "Expired", value: 1, sub: TECH_EXTERNAL_B, chip: null },
      { label: "Pending HQ verification", value: 1, sub: "uploads you sent", chip: null },
    ]);
    expect(impact(rows, [job])).toStrictEqual({
      text: "Electrical basics of tech-external-a expires 15 Oct 2026. job-p07 (Server room AC, 20 Oct) needs it — renew before then or reassign.", jobId: "job-p07-full",
      rows: [["Job", "job-p07 · Server room AC"], ["Slot", "20 Oct, 9:00 am – 12:00 pm MYT"], ["Assigned", TECH_EXTERNAL_A], ["Requires", "Electrical work"]],
    });
    expect(impact(certRows([a], [cert("c2", { code: DEMO_INDOOR, expiresAt: KL(TEXT_2027_03_31) })], [job], 60, NOW), [job])?.text) // a grant without a certificate ends too
      .toBe("Electrical work of tech-external-a expires 15 Oct 2026. job-p07 (Server room AC, 20 Oct) needs it — renew before then or reassign.");
    expect(impact(certRows([a], [], [], 60, NOW), [])).toBeNull();
  });

  it("checks the certificate form and reads the refusals", () => {
    const f: CertForm = { membershipId: "m-a", code: DEMO_ELECTRICAL, name: ELECTRICAL_BASICS, number: "EB-2026-0931", issued: TEXT_2026_09_28, expires: "2029-09-27", file: { name: "eb.pdf", type: "application/pdf", size: 412_000 } };
    expect(certificateErrors(f)).toStrictEqual({});
    expect(certificateErrors({ ...f, number: " ", expires: TEXT_2026_09_28, file: null })).toStrictEqual({ number: "1–64 characters", expires: "The expiry date must be after the issue date", file: "Attach the certificate file" });
    expect(certificateErrors({ ...f, membershipId: "", name: "", issued: "", file: { name: "a.gif", type: "image/gif", size: 10 } })).toStrictEqual({ membershipId: "Choose a technician", name: "1–120 characters", issued: "Choose the issue date", file: "Use a PDF, JPG or PNG file" });
    expect(certificateErrors({ ...f, file: { name: "big.pdf", type: "application/pdf", size: 10_000_001 } }).file).toBe("The file is larger than 10 MB");
    expect(certDates(f)).toStrictEqual({ issuedAt: "2026-09-27T16:00:00.000Z", expiresAt: "2029-09-26T16:00:00.000Z" });
    expect(certificateRefusal({ code: "VALIDATION", fieldErrors: { expiresAt: "error.range", file: "error.invalidFile" } })).toStrictEqual({
      fields: { expires: "The expiry date must be after the issue date", file: "Use a PDF, JPG or PNG file of at most 10 MB" }, text: null,
    });
    expect(certificateRefusal({ code: "NOT_FOUND", fieldErrors: {} }).text).toBe("That technician or certificate is no longer in your company — reload the page.");
    expect(certificateRefusal({ code: "CONFLICT", fieldErrors: {} }).text).toBe("The certificate changed in the meantime — the page shows the latest state.");
    expect([fileSize(412_000), fileSize(2_400_000), fileSize(10)]).toStrictEqual(["412 KB", "2.4 MB", "1 KB"]);
  });

  it("words the certifications in Malay with Kuala Lumpur days (IR277)", () => {
    const rows = certRows([a, b], [...certs, renewal], [job], 60, NOW, MS);
    expect(rows.map((r) => [r.detail, r.expires, r.badge.text])).toStrictEqual([
      ["EB-2023-1120 · dikeluarkan 15 Okt 2023", "15 Okt 2026", "Hampir tamat · 30 hari"],
      ["diberikan oleh HQ · tiada sijil dalam fail", "31 Mac 2027", "Sah"],
      ["RH-2024-0811 · dikeluarkan 31 Mac 2024", "31 Mac 2027", "Sah"],
      ["RH-2022-0310 · dikeluarkan 1 Sep 2022", "1 Sep 2026", "Tamat"],
    ]);
    expect(certKpis(rows, [...certs, renewal], 60, ms).map((k) => k.label)).toStrictEqual(["Sah", "Hampir tamat ≤ 60 hari", "Tamat", "Menunggu pengesahan HQ"]);
    expect(impact(rows, [job], ms)?.text).toBe("Electrical basics bagi tech-external-a akan tamat 15 Okt 2026. job-p07 (Server room AC, 20 Okt) memerlukannya — perbaharui sebelum itu atau tugaskan semula.");
    expect(certificateErrors({ membershipId: "m-a", code: "x", name: "n", number: "1", issued: TEXT_2026_09_28, expires: "2026-09-01", file: null }, ms)).toStrictEqual({ expires: "Tarikh tamat tempoh mesti selepas tarikh dikeluarkan", file: "Lampirkan fail sijil" });
  });
});
