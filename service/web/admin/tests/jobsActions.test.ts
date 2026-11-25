import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ReactNode } from "react";

// The Server Actions of HQ maintenance jobs (FR-A06, DD-A06, FR-C20): each job write at the job's version (a CONFLICT
// re-renders the page; other refusals carry their code and field errors), a job made on a customer's behalf, plans
// (new or at their version, the next occurrence), contractors (a new company, its profile, offers suspended, rate
// cards, certificates, deletion with its accounts when forced), SLA targets and installation requests. The page
// picks its tab, and a photo route answers through the shared photo response.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError, options?: ErrorOptions) {
      super(error.code, options);
      this.name = "CoreError";
      this.error = error;
    }
  }
  return { CoreError, apiMode: vi.fn(() => true), coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>() };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
const force = vi.hoisted(() => ({ deleteAccounts: vi.fn(() => Promise.resolve()) }));
const load = vi.hoisted(() => {
  const fn = (name: string) => vi.fn(() => Promise.resolve({ from: name }));
  return { loadJobs: fn("jobs"), loadPlans: fn("plans"), loadContractors: fn("contractors"), loadSla: fn("sla"), loadInstallations: fn("installations") };
});
const photo = vi.hoisted(() => ({ jobPhotoResponse: vi.fn(() => Promise.resolve(new Response("jpeg"))) }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("@ac/web/lib/forceDelete", () => force);
vi.mock("@ac/web/lib/withAccess", () => ({ withAccess: (_what: string, _perms: string[], fn: () => unknown) => fn() }));
vi.mock("@ac/web/lib/jobPhotoResponse", () => photo);
vi.mock("../app/admin/jobs/_lib/load", () => load);
for (const view of ["jobs-view:JobsView", "plans-view:PlansView", "contractors-view:ContractorsView", "sla-view:SlaView", "installations-view:InstallationsView", "jobs-demo:JobsDemo"]) {
  const [file = "", name = ""] = view.split(":", 2);
  vi.doMock(`../app/admin/jobs/_components/${file}`, () => ({ [name]: () => null }));
}

import {
  approveInstallationRequest, assignInternal, cancelJob, classifyFollowUp, createContractorOrganization, createJob, declineInstallationRequest, deleteContractor, deletePlan, deleteRateCard,
  extendAccess, generateJob, holdJob, offerToContractor, proposeSlot, resolvePartnerSlot, resumeJob, reviewReport, saveContractor, saveCosts, savePlan, saveRateCard, saveSlaTargets,
  setOfferStatus, verifyCertificate, withdrawProposal,
} from "../app/admin/jobs/actions";

const refused = (code: string, fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, fieldErrors, messageKey: `error.${code.toLowerCase()}`, retryAfterSeconds: null });
const v = (n: number) => ({ write: true, expectedVersion: n });
const last = () => dal.coreOp.mock.lastCall;
const slot = { startAt: "2026-09-20T01:00:00Z", endAt: "2026-09-20T03:00:00Z" };

beforeEach(() => {
  dal.coreOp.mockReset(); cache.refresh.mockReset(); force.deleteAccounts.mockClear(); dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockResolvedValue({ id: "x-1", version: 2, name: "Cool Partners" });
});

describe("job writes", () => {
  it("sends each job write at the job's version, trimming its reason", async () => {
    const cases: [() => Promise<unknown>, string, unknown][] = [
      [() => assignInternal("j-1", 3, "m-tech", slot), "jobs.assign", { jobId: "j-1", technicianMembershipId: "m-tech", ...slot }],
      [() => assignInternal("j-1", 3, "m-tech", slot, " Sick leave "), "jobs.assign", { jobId: "j-1", technicianMembershipId: "m-tech", ...slot, reason: "Sick leave" }],
      [() => assignInternal("j-1", 3, "m-tech", slot, "  "), "jobs.assign", { jobId: "j-1", technicianMembershipId: "m-tech", ...slot }],
      [() => offerToContractor("j-1", 3, { contractorOrgId: "org-c", offerExpiresAt: "a", accessValidFrom: "b", accessValidUntil: "c", termsVersion: "t-1" }), "jobs.offer", { jobId: "j-1", contractorOrgId: "org-c", offerExpiresAt: "a", accessValidFrom: "b", accessValidUntil: "c", termsVersion: "t-1" }],
      [() => proposeSlot("j-1", 3, slot, { kind: "internal", membershipId: "m-tech" }, " Earlier? ", "r"), "jobs.proposeSlot", { jobId: "j-1", hold: { kind: "internal", membershipId: "m-tech" }, replyBy: "r", message: "Earlier?", slot }],
      [() => withdrawProposal("j-1", 3, "p-1"), "jobs.withdrawProposal", { jobId: "j-1", proposalId: "p-1" }],
      [() => resolvePartnerSlot("j-1", 3, "p-1", "send_to_client", "r"), "jobs.resolvePartnerSlot", { jobId: "j-1", proposalId: "p-1", decision: "send_to_client", replyBy: "r" }],
      [() => resolvePartnerSlot("j-1", 3, "p-1", "keep", "r"), "jobs.resolvePartnerSlot", { jobId: "j-1", proposalId: "p-1", decision: "keep" }],
      [() => holdJob("j-1", 3, " Parts "), "jobs.hold", { jobId: "j-1", reason: "Parts" }],
      [() => resumeJob("j-1", 3, " In "), "jobs.resumeHold", { jobId: "j-1", reason: "In" }],
      [() => cancelJob("j-1", 3, " Duplicate "), "jobs.cancel", { jobId: "j-1", cancelReason: "Duplicate" }],
      [() => classifyFollowUp("j-1", 3, "rework", " Same fault "), "jobs.classifyFollowUp", { jobId: "j-1", classification: "rework", reason: "Same fault" }],
      [() => reviewReport("j-1", 3, 2, "accept", "normal", " "), "jobs.review", { jobId: "j-1", reportVersion: 2, decision: "accept", reviewMode: "normal" }],
      [() => reviewReport("j-1", 3, 2, "return", "hq_escalation", " Photos missing "), "jobs.review", { jobId: "j-1", reportVersion: 2, decision: "return", reviewMode: "hq_escalation", reason: "Photos missing" }],
      [() => saveCosts("j-1", 3, []), "jobs.saveCost", { jobId: "j-1", costLines: [] }],
      [() => extendAccess("j-1", 3, "2026-09-30T00:00:00Z", " Delay "), "jobs.extendAccess", { jobId: "j-1", accessValidUntil: "2026-09-30T00:00:00Z", reason: "Delay" }],
      [() => declineInstallationRequest("ir-1", 2, " Not serviced "), "installationRequests.decline", { id: "ir-1", reason: "Not serviced" }],
    ];
    for (const [call, op, body] of cases) {
      expect(await call()).toStrictEqual({ ok: true, value: null });
      expect(last()).toStrictEqual([op, body, op === "installationRequests.decline" ? v(2) : v(3)]);
    }
    expect(cache.refresh).toHaveBeenCalledTimes(cases.length);
  });

  it("re-renders after a CONFLICT, keeps a refusal's code and fields, and answers unavailable for anything else", async () => {
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await holdJob("j-1", 3, "x")).toMatchObject({ ok: false, code: "CONFLICT", messageKey: "error.conflict" });
    expect(cache.refresh).toHaveBeenCalledOnce();
    dal.coreOp.mockRejectedValueOnce(refused("VALIDATION", { reason: "too long" }));
    expect(await holdJob("j-1", 3, "x")).toStrictEqual({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { reason: "too long" }, retryAfterSeconds: null });
    expect(cache.refresh).toHaveBeenCalledOnce(); // a validation refusal changes nothing to show
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    expect(await holdJob("j-1", 3, "x")).toMatchObject({ ok: false, code: "UNAVAILABLE" });
  });

  it("creates a job on a customer's behalf with its preferred times, due date and contact window", async () => {
    expect(await createJob({ unitId: "u-1", type: "reactive", symptom: " Noisy ", slots: [slot, { startAt: "b", endAt: "c" }], dueAt: "2026-09-25T00:00:00Z", contactWindow: " After 6 pm " })).toStrictEqual({ ok: true, value: { id: "x-1" } });
    expect(last()).toStrictEqual(["jobs.create", { unitId: "u-1", type: "reactive", symptom: "Noisy", requestedStart: slot.startAt, requestedEnd: slot.endAt, alternativeSlots: [{ startAt: "b", endAt: "c" }], dueAt: "2026-09-25T00:00:00Z", contactWindow: "After 6 pm" }, { write: true }]);
    await createJob({ unitId: "u-1", type: "reactive", symptom: "x", slots: [slot], dueAt: null, contactWindow: " " });
    expect(Object.keys(last()?.[1] ?? {})).not.toContain("contactWindow");
    dal.coreOp.mockRejectedValueOnce(refused("VALIDATION"));
    expect(await createJob({ unitId: "u-1", type: "reactive", symptom: "x", slots: [slot], dueAt: null, contactWindow: "" })).toMatchObject({ ok: false, code: "VALIDATION" });
    await expect(createJob({ unitId: "u-1", type: "reactive", symptom: "x", slots: [], dueAt: null, contactWindow: "" })).rejects.toThrow("a job needs its 1st preferred time");
  });
});

describe("plans, contractors and SLA", () => {
  it("saves a plan new or at its version, and generates its next occurrence", async () => {
    expect(await savePlan({ id: null, version: null, unitId: "u-1", intervalMonths: 3, nextDueAt: "2026-10-08T00:00:00Z" })).toStrictEqual({ ok: true, value: { id: "x-1" } });
    expect(last()).toStrictEqual(["plans.save", { unitId: "u-1", recurrence: { kind: "monthly", intervalMonths: 3 }, nextDueAt: "2026-10-08T00:00:00Z" }, { write: true }]);
    await savePlan({ id: "pl-1", version: 4, unitId: "u-1", intervalMonths: 6, nextDueAt: "x" });
    expect(last()?.[2]).toStrictEqual(v(4));
    expect(await savePlan({ id: "pl-1", version: null, unitId: "u-1", intervalMonths: 6, nextDueAt: "x" })).toMatchObject({ ok: false, code: "UNAVAILABLE" }); // an update needs its version
    expect(await generateJob("pl-1", 4, "2026-10-08")).toStrictEqual({ ok: true, value: { id: "x-1" } });
    expect(last()).toStrictEqual(["plans.generateNext", { id: "pl-1", occurrenceDate: "2026-10-08" }, v(4)]);
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await generateJob("pl-1", 4, "2026-10-08")).toMatchObject({ ok: false, code: "CONFLICT" });
    dal.coreOp.mockRejectedValueOnce(refused("VALIDATION"));
    expect(await savePlan({ id: null, version: null, unitId: "u-1", intervalMonths: 13, nextDueAt: "x" })).toMatchObject({ ok: false, code: "VALIDATION" });
    expect(await deletePlan("pl-1", 4, " Unit gone ")).toStrictEqual({ ok: true, value: null });
    expect(last()).toStrictEqual(["plans.delete", { id: "pl-1", reason: "Unit gone" }, v(4)]);
  });

  it("creates a contractor company and its profile, suspends its offers, and adds a rate card and SLA targets", async () => {
    expect(await createContractorOrganization(" Cool Partners ")).toStrictEqual({ ok: true, value: { id: "x-1", name: "Cool Partners" } });
    expect(last()).toStrictEqual(["organizations.save", { name: "Cool Partners", kind: "contractor", status: "active" }, { write: true }]);
    const profile = { id: null, version: null, organizationId: "org-c", registrationNo: " 1234-X ", serviceAreas: ["KL"], contactEmail: " ops@cool.example ", insuranceValidUntil: null };
    expect(await saveContractor(profile)).toStrictEqual({ ok: true, value: { id: "x-1" } });
    expect(last()).toStrictEqual(["contractors.save", { organizationId: "org-c", registrationNo: "1234-X", serviceAreas: ["KL"], contactEmail: "ops@cool.example", insuranceValidUntil: null }, { write: true }]);
    await saveContractor({ ...profile, id: "cp-1", version: 5 });
    expect(last()?.[2]).toStrictEqual(v(5));
    await setOfferStatus("org-c", 5, "suspended", " Complaints ");
    expect(last()).toStrictEqual(["contractors.setOfferStatus", { contractorOrgId: "org-c", status: "suspended", reason: "Complaints" }, v(5)]);
    expect(await saveRateCard({ contractorOrgId: "org-c", currency: "MYR", effectiveFrom: "2026-10-01", lines: [{ workType: "repair_base", amountMinor: 15_000, note: null }] })).toStrictEqual({ ok: true, value: { id: "x-1", version: 2 } });
    expect(await deleteRateCard("rc-1", " Typo ")).toStrictEqual({ ok: true, value: null });
    expect(last()).toStrictEqual(["rateCards.delete", { id: "rc-1", reason: "Typo" }, { write: true }]);
    expect(await saveSlaTargets({ planType: "rto", responseHours: 24, arrivalInWindowPercent: 90, firstTimeFixPercent: 80, effectiveFrom: "2026-10-01T00:00:00Z" })).toStrictEqual({ ok: true, value: { version: 2 } });
  });

  it("verifies a certificate, with a reason only for a rejection", async () => {
    await verifyCertificate("cert-1", 2, "approve", "ignored");
    expect(last()).toStrictEqual(["certificates.verify", { certificateId: "cert-1", decision: "approve" }, v(2)]);
    await verifyCertificate("cert-1", 2, "reject", " Expired ");
    expect(last()).toStrictEqual(["certificates.verify", { certificateId: "cert-1", decision: "reject", reason: "Expired" }, v(2)]);
  });

  it("deletes a contractor's profile and then its company, with its accounts first when forced", async () => {
    await deleteContractor({ id: "org-c", version: 3 }, { id: "cp-1", version: 5 }, " Mistake ");
    expect(dal.coreOp.mock.calls).toStrictEqual([
      ["contractors.delete", { force: false, id: "cp-1", reason: "Mistake" }, v(5)],
      ["organizations.delete", { id: "org-c", reason: "Mistake" }, v(3)],
    ]);
    expect(force.deleteAccounts).not.toHaveBeenCalled();
    dal.coreOp.mockClear();
    await deleteContractor({ id: "org-c", version: 3 }, null, "Test data", true);
    expect(dal.coreOp.mock.calls.map(([op]) => op)).toStrictEqual(["organizations.delete"]); // no profile to delete
    expect(force.deleteAccounts).toHaveBeenCalledWith("org-c", "Test data");
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await deleteContractor({ id: "org-c", version: 3 }, { id: "cp-1", version: 5 }, "x")).toMatchObject({ ok: false, code: "CONFLICT" });
  });
});

describe("installation requests", () => {
  const request = { requestId: "ir-1", customerOrgId: "org-a", propertyId: "p-1", spaceId: null, modelId: "m-1", names: ["Bedroom AC", "Office AC"], symptom: " New install ", slots: [slot], contactWindow: " Mornings " };

  it("makes a planned unit and an installation job for each name", async () => {
    dal.coreOp.mockImplementation((op) => Promise.resolve(op === "units.save" ? { id: `u-${String(dal.coreOp.mock.calls.length)}` } : {}));
    expect(await approveInstallationRequest(request)).toStrictEqual({ ok: true, value: { made: 2 } });
    const units = dal.coreOp.mock.calls.filter(([op]) => op === "units.save").map(([, input]) => input);
    expect(units).toStrictEqual(["Bedroom AC", "Office AC"].map((displayName) => ({ displayName, customerOrgId: "org-a", propertyId: "p-1", spaceId: null, modelId: "m-1", type: "split", installState: "planned", installedAt: null, serviceScope: ["indoor", "outdoor"] })));
    const jobs = dal.coreOp.mock.calls.filter(([op]) => op === "jobs.create").map(([, input]) => input);
    expect(jobs[0]).toMatchObject({ type: "installation", symptom: "New install", installationRequestId: "ir-1", contactWindow: "Mornings", alternativeSlots: [] });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("keeps the pairs that were made when one is refused, and needs a preferred time", async () => {
    dal.coreOp.mockImplementation((op, input) => (op === "units.save" && JSON.stringify(input).includes("Office") ? Promise.reject(refused("CONFLICT")) : Promise.resolve({ id: "u-ok" })));
    expect(await approveInstallationRequest({ ...request, contactWindow: "" })).toMatchObject({ ok: false, code: "CONFLICT" });
    expect(dal.coreOp.mock.calls.filter(([op]) => op === "jobs.create")).toHaveLength(1); // Bedroom AC's job was made
    expect(await approveInstallationRequest({ ...request, slots: [] })).toMatchObject({ ok: false, code: "UNAVAILABLE" });
  });
});

describe("the page and the photo route", () => {
  it("picks the tab from the URL, the demo outside API mode", async () => {
    const { default: page } = await import("../app/admin/jobs/page");
    const live = async (tab?: string) => {
      const el: ReactNode = await page({ searchParams: Promise.resolve(tab ? { tab } : {}), params: Promise.resolve({}) });
      return isValidElement<{ live?: { from: string }; tab?: string }>(el) ? el.props : null;
    };
    const read = async (tab?: string) => { const p = await live(tab); return p?.live?.from ?? p?.tab; };
    for (const tab of ["plans", "contractors", "sla", "installations"]) expect(await read(tab)).toBe(tab);
    expect(await read("bogus")).toBe("jobs");
    dal.apiMode.mockReturnValue(false);
    expect(await read("plans")).toBe("plans"); // the demo gets the tab
  });

  it("answers a job photo through the shared photo response", async () => {
    const { GET } = await import("../app/admin/jobs/[id]/photos/[attachmentId]/route");
    const res = await GET(new Request("https://hq.example/admin/jobs/j-1/photos/att-1"), { params: Promise.resolve({ id: "j-1", attachmentId: "att-1" }) });
    expect(await res.text()).toBe("jpeg");
    expect(photo.jobPhotoResponse).toHaveBeenCalledWith("j-1", "att-1");
  });
});
