import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiClientEvent, ApiClientJob, ApiClientRow } from "@ac/web/lib/customerMaintenance";
import type { ApiFilterSettings, ApiFilterStatus } from "@ac/web/lib/customerFilterCare";
import type { ApiWorkReport } from "@ac/web/lib/partnerReview";

// My requests and Filter care of the customer app (FR-C09, FR-C17, FR-C18): the Server Actions (new requests one by one
// or as a cleaning batch that stops at the first refusal, the answer to a proposal, another plan time, cancel, notes,
// ratings, problem reports with their photos typed by their bytes, Mark cleaned and the reminders) and the loader (the
// rate banner, the selected request with its notes, history, report and photos, a request that is gone, Filter care).
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
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(), coreAll: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
    coreItems: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(), apiMode: vi.fn(), coreDisplay: vi.fn(), coreNow: vi.fn(), corePrincipal: vi.fn(), CoreError,
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/maintenance/_components/maintenance-demo", () => ({ MaintenanceDemo: () => null }));
vi.mock("../app/customer/maintenance/_components/maintenance-view", () => ({ MaintenanceView: () => null }));

import {
  addNote, cancelRequest, createCleaningRequests, createRequest, markCleaned, rateJob, reportProblem, requestReschedule, respondProposal, saveReminders,
} from "../app/customer/maintenance/actions";
import CustomerMaintenancePage from "../app/customer/maintenance/page";
import { loadMaintenance } from "../app/customer/maintenance/_lib/load";
import { MaintenanceDemo } from "../app/customer/maintenance/_components/maintenance-demo";
import { MaintenanceView } from "../app/customer/maintenance/_components/maintenance-view";
import MaintenanceLoading from "../app/customer/maintenance/loading";
import MaintenanceErrorState from "../app/customer/maintenance/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const NOW = new Date("2026-09-15T01:00:00Z");
const ME = "user-me";
const slot = (startAt: string, endAt: string) => ({ startAt, endAt });
const three = [slot("2026-09-17T02:00:00Z", "2026-09-17T04:00:00Z"), slot("2026-09-18T06:00:00Z", "2026-09-18T08:00:00Z"), slot("2026-09-19T02:00:00Z", "2026-09-19T04:00:00Z")];
const refusal = (code: string, messageKey = "error.x", fieldErrors: Record<string, string> = {}) => new dal.CoreError({ code, messageKey, fieldErrors, retryAfterSeconds: null });
const units = [
  { id: "u-bed", displayName: "Bedroom AC", propertyId: "p-home", spaceId: "s-1f", archived: false, connection: "online" },
  { id: "u-old", displayName: "Old AC", propertyId: "p-home", spaceId: null, archived: true, connection: "offline" },
];
const properties = [{ id: "p-home", name: "Home A" }];
const spaces = [{ id: "s-1f", name: "1F", parentSpaceId: null }];
const row = (id: string, over: Partial<ApiClientRow> = {}): ApiClientRow => ({
  projection: "summary", id, version: 1, unitId: "u-bed", type: "reactive", status: "requested", displayStatus: "requested", origin: "client_request", dueAt: "2026-09-20T00:00:00Z",
  requestedSlot: three[0] ?? slot("", ""), scheduledSlot: null, preferredSlots: three, assignmentAcknowledgement: null, ...over,
});
const job = (id: string, over: Partial<ApiClientJob> = {}): ApiClientJob => ({
  projection: "detail", id, version: 3, unitId: "u-bed", type: "reactive", status: "completed", symptom: "Water drips from the indoor unit", contactWindow: null, origin: "client_request",
  planId: null, occurrenceAt: null, requestedSlot: three[0] ?? slot("", ""), preferredSlots: three, preferenceRound: 1, slotProposal: null, scheduledSlot: three[0] ?? null,
  dueAt: "2026-09-20T00:00:00Z", completedAt: "2026-09-14T03:00:00Z", contractorOrgId: null, followUpOfJobId: null, followUpClass: null, rating: null, customerConfirmedAt: null,
  reportRefs: [], assignment: null, ...over,
});
const event = (id: string, occurredAt: string, over: Partial<ApiClientEvent> = {}): ApiClientEvent => ({ id, action: "job.created", occurredAt, actorUserId: null, note: null, ...over });
const report = (over: Partial<ApiWorkReport> = {}): ApiWorkReport => ({
  id: "rep-1", version: 2, jobId: "job-done-1", authorId: "tech-1", reviewAvailability: { allowed: true, reason: null },
  items: [{ id: "it-1", componentGroup: "indoor", componentKey: "filter", result: "attention", reason: "dusty", evidenceIds: [], authorId: "tech-1", observedAt: "2026-09-14T02:00:00Z" },
    { id: "it-2", componentGroup: "outdoor", componentKey: "fan", result: null, reason: null, evidenceIds: [], authorId: "tech-1", observedAt: "2026-09-14T02:00:00Z" }],
  measurements: [{ id: "m-1", metric: "temperature", value: 12.5, unit: "°C", quality: "valid", observedAt: "2026-09-14T02:00:00Z" }],
  parts: [{ name: "Filter", quantity: 2, source: "stock", replacesComponentKey: null }], refrigerant: [], signOff: null, workText: "Cleaned the filter.", nextAction: null,
  attachmentRefs: [
    { id: "a-ready", name: "before.jpg", mime: "image/jpeg", size: 1000, status: "ready" }, { id: "a-busy", name: "busy.jpg", mime: "image/jpeg", size: 1000, status: "processing" },
    { id: "a-big", name: "big.jpg", mime: "image/jpeg", size: 2_000_001, status: "ready" }, { id: "a-gone", name: "gone.jpg", mime: "image/jpeg", size: 1000, status: "ready" },
  ],
  submittedAt: "2026-09-14T03:00:00Z", acceptedAt: "2026-09-14T05:00:00Z", reviewHistory: [], ...over,
});
const settings = (over: Partial<ApiFilterSettings> = {}): ApiFilterSettings => ({ id: "fs-1", version: 0, customerId: "org-a", thresholdHours: null, fallbackDays: 90, recipients: "owners", channels: ["inApp"], ...over });
const filterStatus = (unitId: string, over: Partial<ApiFilterStatus> = {}): ApiFilterStatus => ({
  unitId, runHoursSinceCleaning: 300, thresholdHours: 250, fallbackDays: 90, lastCleanedAt: "2026-07-21T00:00:00Z", lastCleanedBy: "customer", lastCleaningJobId: null, status: "overdue", ...over,
});
/** the Core API answers by operation, `jobs.get` by job id */
let jobs = new Map<string, object>();
let ops = new Map<string, unknown>();
const answer = (op: string, input: unknown): Promise<unknown> => {
  if (op === "jobs.get" && typeof input === "object" && input !== null && "jobId" in input && typeof input.jobId === "string") {
    const j = jobs.get(input.jobId);
    return Error.isError(j) ? Promise.reject(j) : Promise.resolve(j ?? job(input.jobId));
  }
  const v = ops.get(op);
  return Error.isError(v) ? Promise.reject(v) : Promise.resolve(v);
};
const calls = (op: string) => dal.coreOp.mock.calls.filter((c) => c[0] === op).map((c) => c[1]);
const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof MaintenanceView>>(element) || element.type !== MaintenanceView) throw new TypeError("not MaintenanceView");
  return element.props.live;
};

beforeEach(() => {
  vi.resetAllMocks();
  jobs = new Map();
  ops = new Map();
  dal.apiMode.mockReturnValue(true);
  dal.coreNow.mockResolvedValue(NOW);
  dal.corePrincipal.mockResolvedValue({ userId: ME, clientRole: "owner" });
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  const lists = new Map<string, unknown>([["units.list", units], ["properties.list", properties], ["spaces.list", spaces], ["jobs.list", []], ["filterCare.list", []]]);
  dal.coreAll.mockImplementation((op: string) => Promise.resolve(lists.get(op)));
  dal.coreItems.mockResolvedValue([]);
  dal.coreOp.mockImplementation(answer);
});

describe("maintenance actions", () => {
  it("createRequest sends the 1st time as the requested window and the others as alternatives, trimmed, the contact window only when given", async () => {
    ops.set("jobs.create", { id: "job-new-1" });
    await expect(createRequest({ unitId: "u-bed", type: "reactive", symptom: "  Weak cold air  ", slots: three, contactWindow: "  Weekdays 09:00-18:00 " })).resolves.toStrictEqual({ ok: true, value: { id: "job-new-1" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["jobs.create", {
      unitId: "u-bed", type: "reactive", symptom: "Weak cold air", requestedStart: three[0]?.startAt, requestedEnd: three[0]?.endAt, alternativeSlots: three.slice(1), contactWindow: "Weekdays 09:00-18:00",
    }, { write: true }]);
    await createRequest({ unitId: "u-bed", type: "preventive", symptom: "Weak cold air", slots: three, contactWindow: " ".repeat(3) });
    expect(calls("jobs.create").at(-1)).not.toHaveProperty("contactWindow");
    expect(cache.refresh).toHaveBeenCalledTimes(2);
    // no time at all is refused before any call
    await expect(createRequest({ unitId: "u-bed", type: "reactive", symptom: "Weak cold air", slots: [], contactWindow: "" })).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE" });
    expect(calls("jobs.create")).toHaveLength(2);
  });

  it("a refusal keeps its code and fields; only a CONFLICT re-reads the page", async () => {
    ops.set("jobs.create", refusal("VALIDATION", "error.validation", { contactWindow: "errors.contact_details_forbidden" }));
    await expect(createRequest({ unitId: "u-bed", type: "reactive", symptom: "Weak cold air", slots: three, contactWindow: "call 0123456789" }))
      .resolves.toStrictEqual({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { contactWindow: "errors.contact_details_forbidden" }, retryAfterSeconds: null });
    expect(cache.refresh).not.toHaveBeenCalled();
    ops.set("jobs.cancel", refusal("CONFLICT", "error.versionConflict"));
    await expect(cancelRequest("job-1", 4, "Resolved")).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("the cleaning batch sends one preventive request per AC in order and stops at the first refusal, counting the ones sent", async () => {
    const created = ["job-c-1", "job-c-2"];
    dal.coreOp.mockImplementation((op: string) => {
      const id = created.shift();
      return op === "jobs.create" && id ? Promise.resolve({ id }) : Promise.reject(refusal("RATE_LIMITED", "error.rateLimited"));
    });
    const input = { units: [{ unitId: "u-1", symptom: "Filter cleaning: 300 h" }, { unitId: "u-2", symptom: "Filter cleaning: 280 h" }, { unitId: "u-3", symptom: "x" }, { unitId: "u-4", symptom: "y" }], slots: three, contactWindow: "" };
    await expect(createCleaningRequests(input)).resolves.toStrictEqual({ ok: true, value: { ids: ["job-c-1", "job-c-2"], stopped: { code: "RATE_LIMITED", messageKey: "error.rateLimited", fieldErrors: {}, retryAfterSeconds: null } } });
    expect(calls("jobs.create")).toMatchObject([{ unitId: "u-1", type: "preventive", symptom: "Filter cleaning: 300 h" }, { unitId: "u-2" }, { unitId: "u-3" }]);
    dal.coreOp.mockResolvedValue({ id: "job-c-9" });
    await expect(createCleaningRequests({ ...input, units: input.units.slice(0, 1) })).resolves.toStrictEqual({ ok: true, value: { ids: ["job-c-9"], stopped: null } });
  });

  it("respondProposal accepts with the job's version, or declines with the reason, a trimmed comment and the new times only when given", async () => {
    await expect(respondProposal("job-1", 5, "prop-1", { decision: "accept" })).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["jobs.respondProposal", { jobId: "job-1", proposalId: "prop-1", decision: "accept" }, { write: true, expectedVersion: 5 }]);
    await respondProposal("job-1", 5, "prop-1", { decision: "decline", declineReason: "too_late", comment: " Mornings only ", slots: three });
    expect(dal.coreOp.mock.lastCall?.[1]).toStrictEqual({ jobId: "job-1", proposalId: "prop-1", decision: "decline", declineReason: "too_late", comment: "Mornings only", preferredSlots: three });
    await respondProposal("job-1", 5, "prop-1", { decision: "decline", declineReason: "other", comment: "  ", slots: [] });
    expect(dal.coreOp.mock.lastCall?.[1]).toStrictEqual({ jobId: "job-1", proposalId: "prop-1", decision: "decline", declineReason: "other" });
  });

  it("another plan time, cancel, a note and a rating carry the version and trim their text", async () => {
    await requestReschedule("job-p", 2, three, " Away that week ");
    await requestReschedule("job-p", 2, three, "");
    await cancelRequest("job-1", 4, "  Resolved itself ");
    await addNote("job-1", 4, " Gate code 4821 ");
    await rateJob("job-d", 6, 5, ["On time"], " Tidy ");
    await rateJob("job-d", 6, 4, [], "");
    expect(dal.coreOp.mock.calls).toStrictEqual([
      ["jobs.requestReschedule", { jobId: "job-p", preferredSlots: three, comment: "Away that week" }, { write: true, expectedVersion: 2 }],
      ["jobs.requestReschedule", { jobId: "job-p", preferredSlots: three }, { write: true, expectedVersion: 2 }],
      ["jobs.cancel", { jobId: "job-1", cancelReason: "Resolved itself" }, { write: true, expectedVersion: 4 }],
      ["jobs.addNote", { jobId: "job-1", message: "Gate code 4821", visibility: "customer" }, { write: true, expectedVersion: 4 }],
      ["jobs.rate", { jobId: "job-d", stars: 5, tags: ["On time"], comment: "Tidy" }, { write: true, expectedVersion: 6 }],
      ["jobs.rate", { jobId: "job-d", stars: 4, tags: [] }, { write: true, expectedVersion: 6 }],
    ]);
  });

  it("reportProblem sends the photos typed by their bytes with clean names, skips empty files and keeps a preferred visit only with both ends", async () => {
    ops.set("jobs.reportProblem", { id: "job-follow-1" });
    const png = new Uint8Array([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 1, 2]);
    const form = new FormData();
    form.set("jobId", "job-d"); form.set("version", "6"); form.set("reasonCode", "same_problem"); form.set("details", "  Dripping again since Monday  ");
    form.append("photos", new File([png], "a/b.jpg", { type: "image/jpeg" }));
    form.append("photos", new File([], "empty.png", { type: "image/png" }));
    form.set("visitStart", three[0]?.startAt ?? ""); form.set("visitEnd", three[0]?.endAt ?? "");
    await expect(reportProblem(form)).resolves.toStrictEqual({ ok: true, value: { id: "job-follow-1" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["jobs.reportProblem", {
      jobId: "job-d", reasonCode: "same_problem", details: "Dripping again since Monday", preferredSlot: three[0],
      photos: [{ name: "a_b.jpg", mime: "image/png", size: 10, bytes: png.toBase64() }],
    }, { write: true, expectedVersion: 6 }]);
    form.delete("visitEnd");
    form.delete("photos");
    await reportProblem(form);
    expect(calls("jobs.reportProblem").at(-1)).toMatchObject({ preferredSlot: null, photos: [] });
  });

  it("reportProblem refuses a photo that is no JPEG or PNG and an unknown reason before calling the API", async () => {
    const form = new FormData();
    form.set("jobId", "job-d"); form.set("version", "6"); form.set("reasonCode", "same_problem"); form.set("details", "Dripping again since Monday");
    form.append("photos", new File(["%PDF-1.7"], "scan.png", { type: "image/png" }));
    await expect(reportProblem(form)).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { photos: "error.invalid" } });
    form.delete("photos");
    form.set("reasonCode", "angry");
    await expect(reportProblem(form)).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { reasonCode: "error.invalid" } });
    expect(calls("jobs.reportProblem")).toStrictEqual([]);
  });

  it("Mark cleaned and the reminder settings are writes without a version", async () => {
    const input = { thresholdHours: 300, fallbackDays: 60, recipients: "all_users" as const, channels: ["inApp" as const, "email" as const] };
    await expect(markCleaned("u-bed")).resolves.toStrictEqual({ ok: true, value: null });
    await expect(saveReminders(input)).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp.mock.calls).toStrictEqual([["filterCare.markCleaned", { unitId: "u-bed" }, { write: true }], ["filterCare.saveSettings", input, { write: true }]]);
    dal.coreOp.mockRejectedValueOnce(refusal("FORBIDDEN", "error.forbidden"));
    await expect(saveReminders(input)).resolves.toMatchObject({ ok: false, code: "FORBIDDEN" });
  });
});

describe("loadMaintenance", () => {
  it("lists the requests by status with the rate banner for the newest unrated completed job, and New request only for an active AC", async () => {
    const list = [
      row("job-done-old", { status: "completed", displayStatus: "completed", scheduledSlot: slot("2026-09-01T02:00:00Z", "2026-09-01T04:00:00Z") }),
      row("job-req-1"), row("job-done-new", { status: "completed", displayStatus: "completed", scheduledSlot: null, dueAt: "2026-09-13T00:00:00Z" }),
      row("job-done-mid", { status: "completed", displayStatus: "completed", scheduledSlot: slot("2026-09-10T02:00:00Z", "2026-09-10T04:00:00Z") }),
      row("job-done-4th", { status: "completed", displayStatus: "completed", scheduledSlot: slot("2026-08-01T02:00:00Z", "2026-08-01T04:00:00Z"), unitId: "u-missing" }),
    ];
    dal.coreAll.mockImplementation((op: string) => Promise.resolve(new Map<string, unknown>([["units.list", units], ["properties.list", properties], ["spaces.list", spaces], ["jobs.list", list]]).get(op)));
    // the newest is rated already, the next one is gone: the third is the one to rate
    jobs.set("job-done-new", job("job-done-new", { rating: { stars: 5, tags: [], comment: null, ratedAt: "2026-09-14T00:00:00Z", editableUntil: "2026-09-21T00:00:00Z" } }));
    jobs.set("job-done-mid", refusal("NOT_FOUND"));
    jobs.set("job-done-old", job("job-done-old", { completedAt: "2026-09-12T03:00:00Z" }));
    const live = await loadMaintenance({ new: "u-bed" });
    expect(dal.coreAll).toHaveBeenCalledWith("jobs.list", { sort: { field: "status", direction: "asc" } });
    expect(calls("jobs.get")).toStrictEqual([{ jobId: "job-done-new" }, { jobId: "job-done-mid" }, { jobId: "job-done-old" }]);
    expect(live.rateBanner).toStrictEqual({ jobId: "job-done-old", text: "Work finished — job-done · Bedroom AC · 12 Sept 2026." });
    expect(live).toMatchObject({ page: "requests", owner: true, requestUnitId: "u-bed", filters: null, detail: null, now: NOW.toISOString() });
    expect(live.units).toStrictEqual([{ id: "u-bed", name: "Bedroom AC", place: "Home A › 1F" }]);
    expect(live.rows.map((r) => [r.id, r.unit])).toStrictEqual([["job-req-1", "Bedroom AC"], ["job-done-new", "Bedroom AC"], ["job-done-old", "Bedroom AC"], ["job-done-mid", "Bedroom AC"], ["job-done-4th", "AC no longer available"]]);
    // an archived AC or an unknown one opens no New request
    const archived = await loadMaintenance({ new: "u-old" });
    expect(archived.requestUnitId).toBeNull();
    const unknown = await loadMaintenance({ new: ["u-x", "u-bed"] });
    expect(unknown.requestUnitId).toBeNull();
  });

  it("a completed job without its completion time gets the short banner; a job older than 7 days or not a detail gets none", async () => {
    dal.corePrincipal.mockResolvedValue({ userId: ME, clientRole: "member" });
    const done = [row("job-a", { status: "completed", displayStatus: "completed" })];
    dal.coreAll.mockImplementation((op: string) => Promise.resolve(new Map<string, unknown>([["units.list", units], ["properties.list", []], ["spaces.list", []], ["jobs.list", done]]).get(op)));
    jobs.set("job-a", job("job-a", { completedAt: null }));
    const live = await loadMaintenance({});
    expect(live.rateBanner?.text).toBe("Work finished — job-a · Bedroom AC.");
    expect(live.owner).toBe(false);
    jobs.set("job-a", job("job-a", { completedAt: "2026-09-01T00:00:00Z" }));
    const old = await loadMaintenance({});
    expect(old.rateBanner).toBeNull();
    jobs.set("job-a", { ...job("job-a"), projection: "history" });
    const history = await loadMaintenance({});
    expect(history.rateBanner).toBeNull();
    jobs.set("job-a", refusal("INTERNAL"));
    await expect(loadMaintenance({})).rejects.toBeInstanceOf(dal.CoreError);
  });

  it("the selected request has its facts, notes, the 8 newest history entries and the accepted report with up to four ready photos", async () => {
    jobs.set("job-done-1", job("job-done-1", { reportRefs: [{ reportId: "rep-0", reportVersion: 1 }, { reportId: "rep-1", reportVersion: 2 }] }));
    const events = [
      event("e-note-me", "2026-09-14T04:00:00Z", { action: "job.note_added", actorUserId: ME, note: { visibility: "customer", message: "Gate code 4821" } }),
      event("e-note-hq", "2026-09-14T05:00:00Z", { action: "job.note_added", actorUserId: "hq-1", note: { visibility: "customer", message: "Noted" } }),
      ...Array.from({ length: 8 }, (_, k) => event(`e-${String(k)}`, `2026-09-0${String(k + 1)}T00:00:00Z`, { action: "job.rated" })),
    ];
    dal.coreItems.mockResolvedValue(events);
    ops.set("reports.get", report());
    dal.coreOp.mockImplementation((op: string, input?: unknown) => {
      if (op !== "attachments.getContent") return answer(op, input);
      const id = typeof input === "object" && input !== null && "attachmentId" in input ? input.attachmentId : null;
      return id === "a-gone" ? Promise.reject(refusal("NOT_FOUND")) : Promise.resolve({ mime: "image/jpeg", bytes: "QUJD" });
    });
    const live = await loadMaintenance({ jobId: "job-done-1" });
    expect(dal.coreItems).toHaveBeenCalledWith("jobs.events", { jobId: "job-done-1", query: { limit: 100 } });
    expect(calls("reports.get")).toStrictEqual([{ jobId: "job-done-1", reportId: "rep-1", reportVersion: 2 }]);
    expect(calls("attachments.getContent")).toStrictEqual([
      { jobId: "job-done-1", reportId: "rep-1", reportVersion: 2, attachmentId: "a-ready" }, { jobId: "job-done-1", reportId: "rep-1", reportVersion: 2, attachmentId: "a-gone" },
    ]);
    const d = live.detail;
    expect(d?.unit).toStrictEqual({ name: "Bedroom AC", place: "Home A › 1F" });
    expect(d?.notes.map((n) => [n.who, n.text])).toStrictEqual([["You", "Gate code 4821"], ["Coordinator", "Noted"]]);
    expect(d?.history.map((h) => h.title)).toStrictEqual(["Note from the coordinator", "You added a note", ...Array.from({ length: 6 }, () => "You rated the job")]);
    expect(d?.history.map((h) => h.id).slice(2)).toStrictEqual(["e-7", "e-6", "e-5", "e-4", "e-3", "e-2"]);
    expect(d?.report).toStrictEqual({
      version: 2, accepted: "14 Sept 2026, 1:00 pm MYT",
      items: [{ id: "it-1", label: "Indoor unit — filter condition", result: "Attention", tone: "warn", reason: "dusty" }, { id: "it-2", label: "Outdoor unit — fan", result: "—", tone: "muted", reason: null }],
      readings: [["Supply air temperature", "12.5 °C · recorded"]], parts: ["Filter × 2"], workText: "Cleaned the filter.", photos: [{ id: "a-ready", name: "before.jpg", url: "data:image/jpeg;base64,QUJD" }],
    });
    expect(d?.feedback.kind).toBe("rate");
  });

  it("a request without a report reads none; a report or events that are gone are left out; a report not yet accepted has no time", async () => {
    jobs.set("job-r", job("job-r", { status: "requested", completedAt: null }));
    const none = await loadMaintenance({ jobId: "job-r" });
    expect(calls("reports.get")).toStrictEqual([]);
    expect(none.detail?.report).toBeNull();
    jobs.set("job-r", job("job-r", { reportRefs: [{ reportId: "rep-1", reportVersion: 1 }] }));
    dal.coreItems.mockRejectedValue(refusal("FORBIDDEN"));
    ops.set("reports.get", refusal("NOT_FOUND"));
    const gone = await loadMaintenance({ jobId: "job-r" });
    expect(gone.detail?.history).toStrictEqual([]);
    expect(gone.detail?.report).toBeNull();
    ops.set("reports.get", report({ acceptedAt: null, attachmentRefs: [] }));
    dal.coreItems.mockResolvedValue([]);
    const pending = await loadMaintenance({ jobId: "job-r" });
    expect(pending.detail?.report).toMatchObject({ accepted: null, photos: [] });
  });

  it("a request that is gone, or that is not the client's detail, is named as missing", async () => {
    jobs.set("job-x", refusal("NOT_FOUND"));
    expect(await loadMaintenance({ jobId: "job-x" })).toMatchObject({ detail: null, missing: "job-x" });
    jobs.set("job-y", { ...job("job-y"), projection: "summary" });
    expect(await loadMaintenance({ jobId: ["job-y"] })).toMatchObject({ detail: null, missing: "job-y" });
    expect(await loadMaintenance({ jobId: "" })).not.toHaveProperty("missing");
  });

  it("Filter care reads the status per AC and the reminder settings, and no requests", async () => {
    const lists = new Map<string, unknown>([["units.list", units], ["properties.list", properties], ["spaces.list", spaces], ["filterCare.list", [filterStatus("u-bed"), filterStatus("u-old", { status: "ok", runHoursSinceCleaning: null })]]]);
    dal.coreAll.mockImplementation((op: string) => Promise.resolve(lists.get(op)));
    ops.set("filterCare.getSettings", settings({ version: 2, thresholdHours: 300, channels: ["inApp", "email"] }));
    for (const tab of ["filter-care", "filters"]) {
      const live = await loadMaintenance({ tab });
      expect(live).toMatchObject({ page: "filters", rows: [], rateBanner: null, detail: null });
      expect(live.filters?.lines.map((l) => (l.kind === "unit" ? [l.row.unit, l.row.state, l.row.runNote] : null))).toStrictEqual([["Bedroom AC", "overdue", null], ["Old AC", "ok", "offline"]]);
      expect(live.filters?.reminders).toContainEqual(["Send by", "App + email"]);
      expect(live.filters?.form).toStrictEqual({ useDefault: false, hours: "300", days: "90", recipients: "owners", email: true });
    }
    expect(dal.coreAll).not.toHaveBeenCalledWith("jobs.list", expect.anything());
  });
});

describe("maintenance page", () => {
  it("renders the demo outside API mode with the job and tab of the URL, without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const page = await CustomerMaintenancePage({ searchParams: Promise.resolve({ jobId: ["job-c02", "job-c03"], tab: "filter-care" }), params: Promise.resolve({}) });
    expect(isValidElement(page) && page.type).toBe(MaintenanceDemo);
    expect(isValidElement<{ jobId?: string; initialTab?: string }>(page) && [page.props.jobId, page.props.initialTab]).toStrictEqual(["job-c02", "filter-care"]);
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("renders the view with what the loader read in API mode", async () => {
    const live = viewProps(await CustomerMaintenancePage({ searchParams: Promise.resolve({}), params: Promise.resolve({}) }));
    expect(live).toMatchObject({ page: "requests", rows: [], detail: null });
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = MaintenanceLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "maintenance requests" });
    const failed = MaintenanceErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Maintenance", error, retry });
  });
});
