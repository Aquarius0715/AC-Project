import { describe, expect, it } from "vitest";
import { certDates, certificateErrors, certificateRefusal, certKpis, certQuery, certRows, fileSize, impact, type ApiCert, type CertForm, type CertJob } from "@ac/web/lib/partnerCertificates";
import type { ApiTeamMember } from "@ac/web/lib/partnerTeam";
import { i18nOf, translator } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const ms = translator("ms");
const NOW = Date.parse("2026-09-15T01:30:00Z"); // Tue 09:30 in Kuala Lumpur
const KL = (d: string) => `${d}T00:00:00+08:00`;
const member = (id: string, name: string, quals: { code: string; validUntil: string }[], validUntil: string | null = null): ApiTeamMember =>
  ({ id, userId: `u-${id}`, organizationId: "org-a", displayName: name, role: "technician", employment: "external", validFrom: KL("2025-04-01"), validUntil, qualifications: quals.map((q) => ({ ...q, validFrom: KL("2025-04-01"), revokedAt: null })) });
const a = member("m-a", "tech-external-a", [{ code: "demo_outdoor", validUntil: KL("2027-03-31") }, { code: "demo_electrical", validUntil: KL("2026-10-15") }]);
const b = member("m-b", "tech-external-b", [], KL("2026-08-01"));
const cert = (id: string, over: Partial<ApiCert>): ApiCert => ({
  id, version: 1, membershipId: "m-a", organizationId: "org-a", code: "demo_electrical", name: "Electrical basics", number: "EB-2023-1120", issuedAt: KL("2023-10-15"), expiresAt: KL("2026-10-15"),
  fileName: "eb.pdf", status: "valid", renewalOf: null, createdAt: "2023-10-15T00:00:00Z", verifiedAt: "2023-10-16T00:00:00Z", trainingRequestedAt: null, ...over,
});
const certs = [
  cert("c1", { status: "expiring" }),
  cert("c2", { code: "demo_indoor", name: "Split-unit refrigerant handling", number: "RH-2024-0811", issuedAt: KL("2024-03-31"), expiresAt: KL("2027-03-31") }),
  cert("c3", { membershipId: "m-b", code: "demo_outdoor", name: "Refrigerant handling", number: "RH-2022-0310", issuedAt: KL("2022-09-01"), expiresAt: KL("2026-09-01"), status: "expired" }),
];
const renewal = cert("c4", { status: "pending_verification", renewalOf: "c1", number: "EB-2026-0931", issuedAt: KL("2026-09-28"), expiresAt: KL("2029-09-27"), createdAt: "2026-09-14T03:00:00Z" });
const job: CertJob = { jobId: "job-p07-full", short: "job-p07", technicianId: "m-a", slot: { startAt: "2026-10-20T01:00:00Z", endAt: "2026-10-20T04:00:00Z" }, text: "20 Oct, 9:00 am – 12:00 pm MYT", unit: "Server room AC", required: ["demo_electrical"] };

describe("partner certifications", () => {
  it("reads the tab's conditions", () => {
    expect(certQuery({}, ["m-a"])).toEqual({ membershipId: null, status: null, within: 60 });
    expect(certQuery({ membershipId: "m-a", status: "expiring", expiringWithinDays: "30" }, ["m-a"])).toEqual({ membershipId: "m-a", status: "expiring", within: 30 });
    expect(certQuery({ membershipId: "m-x", status: "lost", expiringWithinDays: "45" }, ["m-a"])).toEqual({ membershipId: null, status: null, within: 60 });
  });

  it("builds one row per technician and qualification with what it blocks", () => {
    const rows = certRows([a, b], certs, [job], 60, NOW);
    expect(rows.map((r) => [r.technician, r.name, r.detail, r.expires, r.badge.text, r.blocks.map((x) => x.text), r.actions])).toEqual([
      ["tech-external-a", "Electrical basics", "EB-2023-1120 · issued 15 Oct 2023", "15 Oct 2026", "Expiring · 30 d", ["job-p07 (20 Oct)"], ["view", "renew", "training"]],
      ["tech-external-a", "Outdoor unit work", "granted by HQ · no certificate on file", "31 Mar 2027", "Valid", [], ["add"]],
      ["tech-external-a", "Split-unit refrigerant handling", "RH-2024-0811 · issued 31 Mar 2024", "31 Mar 2027", "Valid", [], ["view"]],
      ["tech-external-b", "Refrigerant handling", "RH-2022-0310 · issued 1 Sept 2022", "1 Sept 2026", "Expired", [], ["view"]], // the membership ended: no new upload
    ]);
    expect(certRows([a], certs, [job], 30, NOW)[0].badge.text).toBe("Expiring · 30 d");
    expect(certRows([a], certs, [job], 30, Date.parse("2026-09-10T00:00:00Z"))[0]).toMatchObject({ status: "valid", badge: { text: "Valid" } }); // 35 days left with a 30-day window
  });

  it("notes a pending renewal, a training request and a rejected first upload", () => {
    const [electrical] = certRows([a], [...certs, renewal], [job], 60, NOW);
    expect([electrical.renewalPending, electrical.actions, electrical.cert]).toEqual([true, ["view", "training"], { id: "c1", version: 1, number: "EB-2023-1120" }]);
    const trained = certRows([a], [cert("c1", { trainingRequestedAt: "2026-09-12T02:00:00Z" })], [], 60, NOW)[0];
    expect([trained.training, trained.actions]).toEqual(["Training requested 12 Sept 2026", ["view", "renew"]]);
    const first = certRows([member("m-c", "tech-external-a2", [])], [cert("c9", { membershipId: "m-c", code: "other", name: "Working at height", status: "rejected", createdAt: "2026-09-01T00:00:00Z" })], [], 60, NOW);
    expect(first.map((r) => [r.name, r.badge.text, r.actions])).toEqual([["Working at height", "Rejected", ["view", "renew"]]]);
    const turnedDown = certRows([a], [cert("c7", { code: "demo_outdoor", name: "Outdoor licence", status: "rejected" })], [], 60, NOW).find((r) => r.code === "demo_outdoor")!;
    expect([turnedDown.detail, turnedDown.badge.text, turnedDown.actions]).toEqual(["granted by HQ · the upload was rejected", "Valid", ["add"]]); // the HQ grant still stands
    const uploaded = certRows([a], [cert("c6", { code: "demo_outdoor", status: "pending_verification" })], [], 60, NOW).find((r) => r.code === "demo_outdoor")!;
    expect([uploaded.detail, uploaded.badge.text, uploaded.renewalPending, uploaded.actions]).toEqual(["granted by HQ · the upload waits for HQ", "Valid", true, []]);
    const waiting = certRows([member("m-c", "tech-external-a2", [])], [cert("c8", { membershipId: "m-c", code: "demo_indoor", status: "pending_verification" })], [], 60, NOW);
    expect(waiting.map((r) => [r.badge.text, r.actions, r.blocks])).toEqual([["Pending HQ verification", ["view"], []]]);
  });

  it("names a qualification a booked job needs that the technician does not hold", () => {
    const a2 = member("m-c", "tech-external-a2", []);
    const gas = { ...job, technicianId: "m-c", required: ["demo_outdoor"] };
    expect(certRows([a2], [], [gas], 60, NOW).map((r) => [r.name, r.detail, r.badge.text, r.blocks.map((x) => x.text), r.actions])).toEqual([
      ["Outdoor unit work", "not held", "Not held", ["job-p07 (20 Oct)"], ["add"]],
    ]);
    expect(impact(certRows([a2], [], [gas], 60, NOW), [gas])?.text).toBe("tech-external-a2 does not hold Outdoor unit work. job-p07 (Server room AC, 20 Oct) needs it — add the certificate or reassign.");
  });

  it("counts the KPIs and states the assignment impact", () => {
    const rows = certRows([a, b], [...certs, renewal], [job], 60, NOW);
    expect(certKpis(rows, [...certs, renewal], 60)).toEqual([
      { label: "Valid", value: 3, sub: "in date", chip: null },
      { label: "Expiring ≤ 60 days", value: 1, sub: null, chip: "tech-external-a · 30 d" },
      { label: "Expired", value: 1, sub: "tech-external-b", chip: null },
      { label: "Pending HQ verification", value: 1, sub: "uploads you sent", chip: null },
    ]);
    expect(impact(rows, [job])).toEqual({
      text: "Electrical basics of tech-external-a expires 15 Oct 2026. job-p07 (Server room AC, 20 Oct) needs it — renew before then or reassign.", jobId: "job-p07-full",
      rows: [["Job", "job-p07 · Server room AC"], ["Slot", "20 Oct, 9:00 am – 12:00 pm MYT"], ["Assigned", "tech-external-a"], ["Requires", "Electrical work"]],
    });
    expect(impact(certRows([a], [cert("c2", { code: "demo_indoor", expiresAt: KL("2027-03-31") })], [job], 60, NOW), [job])?.text) // a grant without a certificate ends too
      .toBe("Electrical work of tech-external-a expires 15 Oct 2026. job-p07 (Server room AC, 20 Oct) needs it — renew before then or reassign.");
    expect(impact(certRows([a], [], [], 60, NOW), [])).toBeNull();
  });

  it("checks the certificate form and reads the refusals", () => {
    const f: CertForm = { membershipId: "m-a", code: "demo_electrical", name: "Electrical basics", number: "EB-2026-0931", issued: "2026-09-28", expires: "2029-09-27", file: { name: "eb.pdf", type: "application/pdf", size: 412_000 } };
    expect(certificateErrors(f)).toEqual({});
    expect(certificateErrors({ ...f, number: " ", expires: "2026-09-28", file: null })).toEqual({ number: "1–64 characters", expires: "The expiry date must be after the issue date", file: "Attach the certificate file" });
    expect(certificateErrors({ ...f, membershipId: "", name: "", issued: "", file: { name: "a.gif", type: "image/gif", size: 10 } })).toEqual({ membershipId: "Choose a technician", name: "1–120 characters", issued: "Choose the issue date", file: "Use a PDF, JPG or PNG file" });
    expect(certificateErrors({ ...f, file: { name: "big.pdf", type: "application/pdf", size: 10_000_001 } }).file).toBe("The file is larger than 10 MB");
    expect(certDates(f)).toEqual({ issuedAt: "2026-09-27T16:00:00.000Z", expiresAt: "2029-09-26T16:00:00.000Z" });
    expect(certificateRefusal({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { expiresAt: "error.range", file: "error.invalidFile" } })).toEqual({
      fields: { expires: "The expiry date must be after the issue date", file: "Use a PDF, JPG or PNG file of at most 10 MB" }, text: null,
    });
    expect(certificateRefusal({ code: "NOT_FOUND", messageKey: "error.notFound", fieldErrors: {} }).text).toBe("That technician or certificate is no longer in your company — reload the page.");
    expect(certificateRefusal({ code: "CONFLICT", messageKey: "error.versionConflict", fieldErrors: {} }).text).toBe("The certificate changed in the meantime — the page shows the latest state.");
    expect([fileSize(412_000), fileSize(2_400_000), fileSize(10)]).toEqual(["412 KB", "2.4 MB", "1 KB"]);
  });

  it("words the certifications in Malay with Kuala Lumpur days (IR277)", () => {
    const rows = certRows([a, b], [...certs, renewal], [job], 60, NOW, MS);
    expect(rows.map((r) => [r.detail, r.expires, r.badge.text])).toEqual([
      ["EB-2023-1120 · dikeluarkan 15 Okt 2023", "15 Okt 2026", "Hampir tamat · 30 hari"],
      ["diberikan oleh HQ · tiada sijil dalam fail", "31 Mac 2027", "Sah"],
      ["RH-2024-0811 · dikeluarkan 31 Mac 2024", "31 Mac 2027", "Sah"],
      ["RH-2022-0310 · dikeluarkan 1 Sep 2022", "1 Sep 2026", "Tamat"],
    ]);
    expect(certKpis(rows, [...certs, renewal], 60, ms).map((k) => k.label)).toEqual(["Sah", "Hampir tamat ≤ 60 hari", "Tamat", "Menunggu pengesahan HQ"]);
    expect(impact(rows, [job], ms)?.text).toBe("Electrical basics bagi tech-external-a akan tamat 15 Okt 2026. job-p07 (Server room AC, 20 Okt) memerlukannya — perbaharui sebelum itu atau tugaskan semula.");
    expect(certificateErrors({ membershipId: "m-a", code: "x", name: "n", number: "1", issued: "2026-09-28", expires: "2026-09-01", file: null }, ms)).toEqual({ expires: "Tarikh tamat tempoh mesti selepas tarikh dikeluarkan", file: "Lampirkan fail sijil" });
  });
});
