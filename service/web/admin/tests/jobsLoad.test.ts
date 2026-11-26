import { beforeEach, describe, expect, it, vi } from "vitest";

// The reads of HQ maintenance jobs (FR-A06, DD-A06, DD-A21, DD-A22, FR-C20): the shared reads (organizations, the
// customer → property → unit scope, members, the tab counts), the Jobs tab (pipeline counts, the list with its filters
// and sort, a New job from an alert, the selected job with its events, unit, eligible technicians per preferred time,
// the latest report and its photos), the Plans tab, the Contractors tab, the SLA tab and the Installation requests tab.
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
  return {
    CoreError, coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))), coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["job.read", "job.write", "asset.write"]))),
    coreOp: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreItems: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    orWhenRefused: async <T, F>(read: Promise<T>, fallback: F): Promise<T | F> => {
      try {
        return await read;
      } catch (error) {
        if (error instanceof CoreError && error.error.code !== "UNAVAILABLE") return fallback;
        throw error;
      }
    },
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadContractors, loadInstallations, loadJobs, loadPlans, loadSla } from "../app/admin/jobs/_lib/load";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const later = slot("2026-09-22T02:00:00Z", "2026-09-22T04:00:00Z");
const past = slot("2026-09-10T02:00:00Z", "2026-09-10T04:00:00Z");
const row = (id: string, over: Record<string, unknown> = {}) => ({
  projection: "summary", id, version: 3, unitId: "u-1", type: "reactive", status: "requested", dueAt: later.endAt, requestedSlot: later, scheduledSlot: null, assignmentId: null, origin: "client_request",
  preferredSlots: [later], severity: "normal", displayStatus: "requested", assignmentAcknowledgement: null, technicianMembershipId: null, accessValidFrom: null, accessValidUntil: null,
  createdAt: "2026-09-12T01:00:00Z", followUpOfJobId: null, followUpClass: null, ...over,
});
const job = (over: Record<string, unknown> = {}) => ({
  projection: "detail", id: "job-1", version: 4, unitId: "u-1", type: "reactive", status: "requested", symptom: "Water dripping", contactWindow: null, origin: "client_request", planId: null, occurrenceAt: null,
  requestedSlot: later, preferredSlots: [later, past], preferenceRound: 1, slotProposal: null, partnerSlotProposal: null, scheduledSlot: null, dueAt: later.endAt, startedAt: null, completedAt: null,
  contractorOrgId: null, createdAt: "2026-09-12T01:00:00Z", followUpOfJobId: null, followUpClass: null, assignment: null, offer: null, reportRefs: [], draftReportRef: null, costs: null, ...over,
});
const report = {
  id: "rep-1", version: 2, jobId: "job-1", authorId: "user-tech", reviewAvailability: "available", items: [], measurements: [], parts: [], refrigerant: [], signOff: null, workText: "Cleaned the drain",
  nextAction: { kind: "none" }, submittedAt: "2026-09-13T01:00:00Z", acceptedAt: null, reviewHistory: [],
  attachmentRefs: [
    { id: "a-1", name: "before.jpg", status: "ready", size: 1000, mime: "image/jpeg" }, { id: "a-2", name: "huge.jpg", status: "ready", size: 3_000_000, mime: "image/jpeg" },
    { id: "a-3", name: "pending.jpg", status: "pending", size: 10, mime: "image/jpeg" }, { id: "a-4", name: "gone.jpg", status: "ready", size: 10, mime: "image/jpeg" },
  ],
};
const orgs = [
  { id: "org-hq", version: 1, name: "HQ", kind: "operator", status: "active" }, { id: "org-a", version: 1, name: "Alpha", kind: "customer", status: "active" },
  { id: "org-z", version: 1, name: "Zeta Cool", kind: "contractor", status: "active" }, { id: "org-c", version: 2, name: "Cool Partners", kind: "contractor", status: "active" },
];
const members = [
  { id: "m-int", userId: "user-int", displayName: "Ali", role: "technician", employment: "internal", organizationId: "org-hq", validUntil: null },
  { id: "m-ext", userId: "user-ext", displayName: "Ben", role: "technician", employment: "external", organizationId: "org-c", validUntil: "2027-01-01T00:00:00Z", qualifications: [] },
  { id: "m-hq", userId: "user-hq", displayName: "HQ Ops", role: "admin", employment: null, organizationId: "org-hq", validUntil: null },
];
interface World { rows?: unknown[]; job?: unknown; report?: unknown; plans?: unknown[]; plan?: unknown; profiles?: unknown[]; certs?: unknown[]; cards?: unknown[]; requests?: unknown[]; orgs?: unknown[] }
function core(w: World = {}) {
  const all = new Map<string, unknown>([
    ["organizations.list", w.orgs ?? orgs], ["customers.list", [{ id: "cust-a", name: "Alpha", organizationId: "org-a" }]], ["properties.list", [{ id: "p-1", name: "Tower", customerOrgId: "org-a" }]],
    ["units.list", [{ id: "u-1", displayName: "Lobby AC", customerOrgId: "org-a", propertyId: "p-1", installState: "installed" }, { id: "u-2", displayName: "Annex AC", customerOrgId: "org-a", propertyId: "p-1", installState: "planned" }]],
    ["members.list", members], ["contractors.list", w.profiles ?? [{ organizationId: "org-c", status: "suspended" }]], ["plans.list", w.plans ?? []],
    ["rateCards.list", w.cards ?? []], ["certificates.list", w.certs ?? []], ["installationRequests.list", w.requests ?? []], ["spaces.list", []], ["capabilities.list", [{ id: "m-1", manufacturer: "AC-Co", model: "Split 1" }]],
  ]);
  dal.coreAll.mockImplementation((op) => Promise.resolve(all.get(op) ?? []));
  dal.coreItems.mockImplementation((op, input) => {
    if (op === "jobs.events") return Promise.resolve([{ id: "e-1", action: "jobs.create", occurredAt: "2026-09-12T01:00:00Z", actorUserId: "user-client" }, { id: "e-2", action: "jobs.custom_thing", occurredAt: "2026-09-12T02:00:00Z", actorUserId: null }]);
    return JSON.stringify(input).includes(later.startAt) ? Promise.resolve([{ id: "m-int", displayName: "Ali" }]) : Promise.reject(refused("FORBIDDEN"));
  });
  dal.coreOp.mockImplementation((op, input) => {
    const q = JSON.stringify(input);
    switch (op) {
      case "jobs.list": { return Promise.resolve(q.includes("\"limit\":100") ? { items: w.rows ?? [row("job-1"), row("job-2", { status: "assigned" })], total: 2 } : { items: [], total: q.includes("statuses") ? 7 : 3 }); }
      case "plans.list": { return Promise.resolve({ items: [], total: 4 }); }
      case "jobs.get": { return Error.isError(w.job) ? Promise.reject(w.job) : Promise.resolve(w.job ?? job()); }
      case "units.get": { return Promise.resolve({ displayName: "Lobby AC", location: { pathLabels: ["Tower", "Lobby"] } }); }
      case "reports.get": { return Promise.resolve(w.report ?? report); }
      case "attachments.getContent": { return q.includes("a-4") ? Promise.reject(refused("NOT_FOUND")) : Promise.resolve({ mime: "image/jpeg", bytes: "AAAA" }); }
      case "alerts.get": { return Promise.resolve({ id: "al-12345678", type: "threshold", causeCode: "temperature_high", evidenceText: "28.5 °C for 10 min" }); }
      case "plans.get": { return Error.isError(w.plan) ? Promise.reject(w.plan) : Promise.resolve(w.plan); }
      case "sla.scorecard": { return Promise.resolve(scorecard); }
      default: { return Promise.resolve({}); }
    }
  });
}
const metrics = { responseWithinTarget: 94, arrivalInWindow: 96, firstTimeFix: 86, averageRating: 4.5, ratingCount: 27, openOverdue: 1 };
const scorecard = {
  period: { from: "2026-06-16T01:00:00Z", to: "2026-09-14T01:00:00Z" }, contractorOrgId: null, totals: metrics,
  customers: [{ ...metrics, customerId: "cust-a", planType: "rto", jobCount: 18, status: "on_track" }], breaches: [{ jobId: "job-1", customerId: "cust-a", kind: "overdue", detail: "open past its due time" }],
  targets: [{ planType: "rto", responseHours: 4, arrivalInWindowPercent: 90, firstTimeFixPercent: 85, version: 0, effectiveFrom: null, state: "default" }],
};

beforeEach(() => {
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); dal.coreItems.mockReset();
  core();
});

describe("the Jobs tab", () => {
  it("reads the scope, the stage counts, the sorted list and the first job with its people, history and photos", async () => {
    core({ job: job({ reportRefs: [{ reportId: "rep-0", reportVersion: 1 }, { reportId: "rep-1", reportVersion: 2 }] }) });
    const live = await loadJobs({ customerId: "cust-a", sort: "bogus" });
    expect(live.counts).toStrictEqual({ jobs: 7, plans: 4, contractors: 2 });
    expect(live.stages.every((s) => s.count === 3)).toBe(true);
    expect(live.deliveries.map((d) => d.id)).toStrictEqual(["internal", "org-c", "org-z"]); // contractors by name
    expect(live.contractors).toStrictEqual([{ id: "org-c", name: "Cool Partners", status: "suspended" }, { id: "org-z", name: "Zeta Cool", status: "active" }]);
    expect(live.internalTechs).toStrictEqual([{ id: "m-int", name: "Ali" }]);
    expect(live.units.map((u) => [u.id, u.planned])).toStrictEqual([["u-2", true], ["u-1", false]]); // by customer, then name
    expect([live.rows.map((r) => r.id), live.total, live.requestDraft, live.created, live.periodChip]).toStrictEqual([["job-1", "job-2"], 2, null, null, null]);
    const d = live.detail;
    expect([d?.title, d?.customer, d?.location]).toStrictEqual(["Lobby AC", "Alpha", "Tower › Lobby"]);
    expect(d?.history.map((h) => h.text)).toStrictEqual(["jobs custom thing", expect.stringMatching(/ · client$/)]); // newest first; an unknown action in words
    expect(d?.report?.photos.map((p) => p.id)).toStrictEqual(["a-1"]); // ready, small, readable
    expect(dal.coreItems).toHaveBeenCalledWith("members.eligible", expect.anything());
    expect(dal.coreItems).toHaveBeenCalledTimes(2); // events + the one future preferred time
  });

  it("opens New job from an alert, the job just created, and says when the URL's job is gone", async () => {
    const live = await loadJobs({ new: "u-1", alertId: "al-12345678", jobId: "job-1", created: "book", from: "2026-09-01", to: "2026-09-30" });
    expect(live.requestDraft?.unitId).toBe("u-1");
    expect(live.requestDraft?.symptom).toMatch(/^Alert al-12345: .* — 28\.5 °C for 10 min$/);
    expect(live.created).toStrictEqual({ id: "job-1", book: true });
    expect(live.periodChip).not.toBeNull();
    const plain = await loadJobs({ new: "u-2", created: "nope" });
    expect([plain.requestDraft, plain.created]).toStrictEqual([{ unitId: "u-2", symptom: "" }, null]);
    core({ job: refused("NOT_FOUND") });
    const gone = await loadJobs({ jobId: "job-gone" });
    expect([gone.detail, "missing" in gone ? gone.missing : null]).toStrictEqual([null, "job-gone"]);
    core({ job: refused("UNAVAILABLE") });
    await expect(loadJobs({})).rejects.toThrow("UNAVAILABLE");
    core({ rows: [] });
    const empty = await loadJobs({});
    expect(empty.detail).toBeNull();
  });

  it("names who holds a proposed time and the contractor's offer, and offers an access extension while the job runs", async () => {
    core({
      report: { ...report, attachmentRefs: [] },
      job: job({
        status: "assigned", contractorOrgId: "org-c", reportRefs: [{ reportId: "rep-1", reportVersion: 2 }], draftReportRef: { reportId: "rep-2", reportVersion: 1 }, occurrenceAt: "2026-09-08T02:00:00Z",
        costs: [{ kind: "estimate", label: "Visit", amountMinor: 10_000, currency: "MYR" }],
        slotProposal: { id: "sp-1", source: "hq", slot: later, hold: { kind: "contractor", contractorOrgId: "org-c", technicianMembershipId: null }, message: "Earlier?", replyBy: "2026-09-16T00:00:00Z", status: "declined", decidedAt: "2026-09-13T00:00:00Z", declineReason: null, declineComment: null },
        partnerSlotProposal: { id: "pp-1", offerId: "of-1", slot: later, technicianMembershipId: "m-ext", reason: "Traffic", sentAt: "2026-09-13T02:00:00Z", status: "pending" },
        offer: { id: "of-1", contractorOrgId: "org-c", candidateSlots: [later], offerExpiresAt: "2026-09-15T00:00:00Z", accessValidFrom: "2026-09-14T00:00:00Z", accessValidUntil: "2026-09-30T00:00:00Z", decision: "accept", decidedAt: "2026-09-13T00:00:00Z", declineReason: null },
      }),
    });
    const { detail: d } = await loadJobs({ jobId: "job-1" });
    expect([d?.holdWho, d?.partnerTech, d?.draft, d?.extend]).toStrictEqual(["Cool Partners", "Ben", true, { until: "2026-09-30T00:00:00Z" }]);
    expect(d?.texts.declinedAt).not.toBeNull();
    expect(d?.texts.occurrence).not.toBeNull();
    expect(d?.report?.photos).toStrictEqual([]);
    expect(dal.coreItems).toHaveBeenCalledOnce(); // no eligible technicians outside requested
    core({ job: job({ slotProposal: { id: "sp-2", source: "hq", slot: later, hold: { kind: "internal", membershipId: "m-gone" }, message: "x", replyBy: "2026-09-16T00:00:00Z", status: "pending", decidedAt: null, declineReason: null, declineComment: null }, unitId: "u-gone" }) });
    const { detail: unknown } = await loadJobs({ jobId: "job-1" });
    expect([unknown?.holdWho, unknown?.title, unknown?.customer, unknown?.extend]).toStrictEqual(["HQ technician", "Lobby AC", "customer", null]);
  });

  it("keeps going when the optional reads are refused", async () => {
    core({ job: job({ reportRefs: [{ reportId: "rep-1", reportVersion: 2 }] }) });
    const optionalReads = new Set(["plans.list", "units.get", "reports.get"]);
    const answer = dal.coreOp.getMockImplementation();
    dal.coreOp.mockImplementation((op, input) => (optionalReads.has(op) ? Promise.reject(refused("FORBIDDEN")) : (answer?.(op, input) ?? Promise.resolve({}))));
    dal.coreAll.mockImplementation(() => Promise.reject(refused("FORBIDDEN")));
    dal.coreItems.mockImplementation(() => Promise.reject(refused("FORBIDDEN")));
    const live = await loadJobs({});
    expect([live.counts.contractors, live.counts.plans, live.units, live.detail?.location, live.detail?.report]).toStrictEqual([0, 0, [], "", null]); // no unit place, no report
  });
});

describe("the Plans tab", () => {
  const plan = {
    id: "plan-1", version: 3, unitId: "u-1", recurrence: { kind: "monthly", intervalMonths: 3 }, timezone: "UTC", anchorDay: 8, nextDueAt: "2026-12-08T02:00:00Z",
    generatedOccurrences: [{ occurrenceAt: "2026-06-08T02:00:00Z", jobId: "job-a" }, { occurrenceAt: "2026-09-08T02:00:00Z", jobId: "job-b" }, { occurrenceAt: "2026-03-08T02:00:00Z", jobId: "job-old" }],
    createdAt: "2026-03-01T00:00:00Z", updatedAt: "2026-09-08T00:00:00Z",
  };

  it("lists the plans and opens one with each generated occurrence's job status, newest first", async () => {
    core({ plans: [plan], plan });
    const lists = new Map<string, unknown[]>([
      ["plans.list", [plan]], ["organizations.list", orgs],
      ["jobs.list", [{ projection: "summary", id: "job-a", status: "completed", displayStatus: "completed" }, { projection: "summary", id: "job-b", status: "requested", displayStatus: "time_proposed" }]],
    ]);
    dal.coreAll.mockImplementation((op) => Promise.resolve(lists.get(op) ?? []));
    const live = await loadPlans({});
    expect([live.plansTotal, live.rows.map((r) => r.id)]).toStrictEqual([1, ["plan-1"]]);
    expect(live.detail?.occurrences.map((o) => [o.jobId, o.status])).toStrictEqual([["job-b", "time_proposed"], ["job-a", "completed"], ["job-old", null]]);
    expect(live.detail?.texts.after).toBeTruthy();
  });

  it("says when the URL's plan is gone, reads nothing without plans, and fails on other errors", async () => {
    core({ plans: [plan], plan: refused("NOT_FOUND") });
    const gone = await loadPlans({ planId: "plan-gone" });
    expect([gone.detail, "missing" in gone ? gone.missing : null]).toStrictEqual([null, "plan-gone"]);
    core({ plans: [] });
    const none = await loadPlans({});
    expect(none.detail).toBeNull();
    core({ plans: [plan], plan: refused("UNAVAILABLE") });
    await expect(loadPlans({})).rejects.toThrow("UNAVAILABLE");
  });
});

const card = (id: string, version: number, effectiveFrom: string) => ({ id, version, effectiveFrom, contractorOrgId: "org-c", currency: "MYR", lines: [{ workType: "repair_base", amountMinor: 15_000, note: null }], createdAt: effectiveFrom });

describe("the Contractors tab", () => {
  const kpis = { period: { from: "2026-06-16T01:00:00Z", to: "2026-09-14T01:00:00Z" }, offerAcceptance: 92, arrivalInWindow: 96.2, firstTimeAccepted: 84, averageRating: 4.6, ratingCount: 12, reworkRate: 8 };
  const profile = {
    id: "cp-1", version: 2, organizationId: "org-c", name: "Cool Partners", status: "active", registrationNo: "SSM 1", serviceAreas: ["KL"], contactEmail: "ops@cool.example", insuranceValidUntil: "2027-01-31T00:00:00Z",
    delegation: { from: "2026-04-01T00:00:00Z", to: "2027-01-31T00:00:00Z" }, rateCardId: null, suspendedReason: null, createdAt: "2026-04-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z", kpis,
  };
  const cert = { id: "cert-1", version: 1, membershipId: "m-ext", name: "Refrigerant handling", code: "RH", number: "N-1", issuedAt: "2026-01-01", expiresAt: "2027-01-01", fileName: "cert.pdf", status: "pending_verification", renewalOf: null, organizationId: "org-c", createdAt: "2026-09-10T00:00:00Z" };

  it("lists every contractor, those without a profile, and opens one with its rate card in effect, technicians and certificates", async () => {
    core({ profiles: [profile], cards: [card("rc-new", 3, "2026-10-01T00:00:00Z"), card("rc-now", 2, "2026-08-01T00:00:00Z"), card("rc-old", 1, "2026-01-01T00:00:00Z")], certs: [cert] });
    const live = await loadContractors({ contractorId: "org-c" });
    expect([live.rows.map((r) => r.id), live.withoutProfile, live.canForce]).toStrictEqual([["org-c", "org-z"], [{ id: "org-z", name: "Zeta Cool" }], false]);
    expect(live.detail?.org).toStrictEqual({ id: "org-c", name: "Cool Partners", version: 2 });
    expect(live.detail?.kpis).not.toBeNull();
    expect(live.detail?.techs.map((x) => x.id)).toStrictEqual(["m-ext"]);
    expect(live.detail?.pending).toStrictEqual([expect.objectContaining({ id: "cert-1", technician: "Ben", renewal: false })]);
    expect(live.detail?.rates.current?.title).toMatch(/^Rate card v2 /); // the latest that has started: no profile names one
  });

  it("opens a contractor without a profile, says when the URL's one is gone, and reads nothing without contractors", async () => {
    core({ profiles: [profile] });
    const noProfile = await loadContractors({ contractorId: "org-z" });
    expect([noProfile.detail?.kpis, noProfile.detail?.facts, noProfile.detail?.profile]).toStrictEqual([null, null, null]);
    const gone = await loadContractors({ contractorId: "org-gone" });
    expect([gone.detail, "missing" in gone ? gone.missing : null]).toStrictEqual([null, "org-gone"]);
    core({ orgs: orgs.filter((o) => o.kind !== "contractor") });
    const nobody = await loadContractors({});
    expect(nobody.detail).toBeNull();
  });
});

describe("the SLA and Installation requests tabs", () => {
  it("reads the scorecard for the period and contractor, ignoring an unknown contractor", async () => {
    const live = await loadSla({ period: "30d", contractorId: "org-c" });
    expect(dal.coreOp).toHaveBeenCalledWith("sla.scorecard", expect.objectContaining({ contractorOrgId: "org-c" }));
    expect([live.contractorId, live.rows.length, live.breaches.length, live.contractors.map((c) => c.id)]).toStrictEqual(["org-c", 1, 1, ["org-c", "org-z"]]);
    const any = await loadSla({ contractorId: "org-unknown" });
    expect(any.contractorId).toBe("");
    expect(Object.keys(dal.coreOp.mock.lastCall?.[1] ?? {})).toStrictEqual(["period"]);
  });

  it("lists the submitted requests by default, every one for all, and what HQ may do with them", async () => {
    const request = {
      id: "ir-1", tenantId: "t", version: 1, createdAt: "2026-09-14T01:00:00Z", updatedAt: "2026-09-14T01:00:00Z", customerOrgId: "org-a", propertyId: "p-1", spaceId: null, quantity: 2, modelId: "m-1", withIot: false,
      preferredSlots: [later], contactWindow: null, note: null, status: "submitted", declineReason: null, decidedAt: null, jobIds: [],
    };
    core({ requests: [request, { ...request, id: "ir-2", customerOrgId: "org-unknown" }] });
    const live = await loadInstallations({});
    expect(dal.coreAll).toHaveBeenCalledWith("installationRequests.list", { filters: { status: "submitted" } });
    expect([live.status, live.canApprove, live.canDecline, live.rows.map((r) => r.customer), live.models, live.spaceOf]).toStrictEqual(["submitted", true, true, ["Alpha", "—"], [{ id: "m-1", name: "AC-Co Split 1" }], { "ir-1": null, "ir-2": null }]);
    dal.corePermissions.mockResolvedValueOnce(new Set(["job.write"])).mockResolvedValueOnce(new Set(["job.write"]));
    const all = await loadInstallations({ status: "all" });
    expect(dal.coreAll).toHaveBeenCalledWith("installationRequests.list", { filters: {} });
    expect([all.status, all.canApprove]).toStrictEqual(["all", false]);
    const declined = await loadInstallations({ status: "declined" });
    expect(declined.status).toBe("declined");
  });
});
