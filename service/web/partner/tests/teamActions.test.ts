import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement } from "react";

// The Server Actions of Team & capacity (FR-P06, FR-P09): unavailable days set (the conflicts the Core API found) and
// removed from a day on at the row's version (a CONFLICT or NOT_FOUND re-reads the page), a certificate sent with its
// file as base64 and the type of its bytes (a renewal only names what it renews), and a training request at the
// certificate's version with the trimmed note. The page renders the fixture demo outside API mode and the live view
// with single URL keys in API mode.
const dal = vi.hoisted(() => {
  interface DomainError { code: string; messageKey: string; fieldErrors: Record<string, string>; correlationId?: string; retryAfterSeconds: number | null }
  class CoreError extends Error {
    readonly error: DomainError;
    constructor(error: DomainError | string, options?: ErrorOptions) {
      const e = typeof error === "string" ? { code: error, messageKey: `error.${error.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null } : error;
      super(e.code, options);
      this.name = "CoreError";
      this.error = e;
    }
  }
  return { coreOp: vi.fn(), apiMode: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
const load = vi.hoisted(() => ({ loadTeam: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/partner/team/_lib/load", () => load);
vi.mock("../app/partner/team/_components/mock-team", () => ({ MockTeam: () => null }));
vi.mock("../app/partner/team/_components/team-view", () => ({ TeamView: () => null }));

import { clearUnavailability, requestTraining, setUnavailability, submitCertificate } from "../app/partner/team/actions";
import PartnerTeamPage from "../app/partner/team/page";
import { MockTeam } from "../app/partner/team/_components/mock-team";
import { TeamView } from "../app/partner/team/_components/team-view";
import Loading from "../app/partner/team/loading";
import ErrorState from "../app/partner/team/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

beforeEach(() => {
  vi.resetAllMocks();
  dal.coreOp.mockResolvedValue({});
  load.loadTeam.mockResolvedValue({ marker: "live" });
});

describe("setUnavailability", () => {
  it("writes the days, re-reads, and counts the confirmed assignments the Core API found, none when it names none", async () => {
    const input = { membershipId: "m-1", from: "2026-11-25", to: "2026-11-27", type: "annual_leave" as const };
    dal.coreOp.mockResolvedValueOnce({ conflictingAssignmentIds: ["a-1", "a-2"] });
    await expect(setUnavailability(input)).resolves.toStrictEqual({ ok: true, value: { conflicts: 2 } });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("members.setUnavailability", input, { write: true });
    expect(cache.refresh).toHaveBeenCalledOnce();
    await expect(setUnavailability(input)).resolves.toStrictEqual({ ok: true, value: { conflicts: 0 } });
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("VALIDATION"));
    await expect(setUnavailability(input)).resolves.toMatchObject({ ok: false, code: "VALIDATION" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

describe("clearUnavailability", () => {
  it("removes the days from a day on at the row's version, and says whether part of the row is kept", async () => {
    dal.coreOp.mockResolvedValueOnce({ remaining: { id: "un-1", from: "2026-11-25", to: "2026-11-25" } });
    await expect(clearUnavailability("un-1", 3, "2026-11-26")).resolves.toStrictEqual({ ok: true, value: { kept: true } });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("members.clearUnavailability", { id: "un-1", from: "2026-11-26" }, { write: true, expectedVersion: 3 });
    dal.coreOp.mockResolvedValueOnce({ remaining: null });
    await expect(clearUnavailability("un-1", 3, "2026-11-25")).resolves.toStrictEqual({ ok: true, value: { kept: false } });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("re-reads on a CONFLICT or NOT_FOUND only, and reports an unreachable API as unavailable", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("CONFLICT"));
    await expect(clearUnavailability("un-1", 3, "2026-11-26")).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("NOT_FOUND"));
    await expect(clearUnavailability("un-1", 3, "2026-11-26")).resolves.toMatchObject({ ok: false, code: "NOT_FOUND" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("VALIDATION"));
    await expect(clearUnavailability("un-1", 3, "2026-11-26")).resolves.toMatchObject({ ok: false, code: "VALIDATION" });
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(clearUnavailability("un-1", 3, "2026-11-26")).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

/** The certificate form as the dialog sends it, with `extra` fields set or replaced. */
const certForm = (extra: Record<string, string | File> = {}) => {
  const form = new FormData();
  const fields = { membershipId: " m-1 ", code: "demo_outdoor", name: " Outdoor licence ", number: " OUT-2 ", issued: "2026-11-10", expires: "2027-11-09", ...extra };
  for (const [k, v] of Object.entries(fields)) form.set(k, v);
  return form;
};

describe("submitCertificate", () => {
  it("sends the trimmed fields, the Kuala Lumpur days as instants and the file as base64 with the type of its bytes", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "c-9" });
    // a PDF the browser typed by its name as a PNG, the name with a path separator
    const file = new File(["%PDF-1.7"], "scans/out.png", { type: "image/png" });
    await expect(submitCertificate(certForm({ file }))).resolves.toStrictEqual({ ok: true, value: { id: "c-9" } });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("certificates.submit", {
      membershipId: "m-1", code: "demo_outdoor", name: "Outdoor licence", number: "OUT-2", issuedAt: "2026-11-09T16:00:00.000Z", expiresAt: "2027-11-08T16:00:00.000Z",
      file: { name: "scans_out.png", mime: "application/pdf", size: 8, bytes: "JVBERi0xLjc=" }, // the base64 of "%PDF-1.7"
    }, { write: true });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("names the certificate a renewal replaces, and only then", async () => {
    dal.coreOp.mockResolvedValue({ id: "c-9" });
    await submitCertificate(certForm({ file: new File(["%PDF-"], "r.pdf"), renewalOf: " c-1 " }));
    expect(dal.coreOp).toHaveBeenLastCalledWith("certificates.submit", expect.objectContaining({ renewalOf: "c-1" }), { write: true });
    await submitCertificate(certForm({ file: new File(["%PDF-"], "r.pdf"), renewalOf: "  " }));
    expect(dal.coreOp.mock.calls.at(-1)?.[1]).not.toHaveProperty("renewalOf");
  });

  it("refuses without calling the Core API a missing file, a file of no accepted type, and an unknown code, each naming its field", async () => {
    await expect(submitCertificate(certForm())).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { file: "error.invalid" } });
    await expect(submitCertificate(certForm({ file: new File(["hello"], "notes.txt", { type: "text/plain" }) }))).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { file: "error.invalid" } });
    await expect(submitCertificate(certForm({ code: "gas", file: new File(["%PDF-"], "r.pdf") }))).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { code: "error.invalid" } });
    expect(dal.coreOp).not.toHaveBeenCalled();
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("returns the Core API's refusal with its fields", async () => {
    const refused = new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { expiresAt: "error.range" }, retryAfterSeconds: null });
    dal.coreOp.mockRejectedValueOnce(refused);
    await expect(submitCertificate(certForm({ file: new File(["%PDF-"], "r.pdf") }))).resolves.toStrictEqual({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { expiresAt: "error.range" }, retryAfterSeconds: null });
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});

describe("requestTraining", () => {
  it("writes the trimmed note at the certificate's version, and re-reads on success and on a CONFLICT only", async () => {
    await expect(requestTraining("c-1", 4, "  Refresher  ")).resolves.toStrictEqual({ ok: true, value: null });
    expect(dal.coreOp).toHaveBeenCalledExactlyOnceWith("certificates.requestTraining", { certificateId: "c-1", note: "Refresher" }, { write: true, expectedVersion: 4 });
    expect(cache.refresh).toHaveBeenCalledOnce();
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("CONFLICT"));
    await expect(requestTraining("c-1", 4, "Refresher")).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("NOT_FOUND"));
    await expect(requestTraining("c-1", 4, "Refresher")).resolves.toMatchObject({ ok: false, code: "NOT_FOUND" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

type Query = Record<string, string | string[] | undefined>;
const page = (query: Query) => PartnerTeamPage({ params: Promise.resolve({}), searchParams: Promise.resolve(query) });

describe("Partner team page and route states", () => {
  it("renders the demo outside API mode, the view with single URL keys in API mode, and names the team in its states", async () => {
    dal.apiMode.mockReturnValue(false);
    const demo = await page({ date: "2026-11-20" });
    expect(isValidElement(demo) && demo.type).toBe(MockTeam);
    expect(load.loadTeam).not.toHaveBeenCalled();
    dal.apiMode.mockReturnValue(true);
    const view = await page({ date: ["2026-11-20", "2026-11-21"], status: "expired", expiringWithinDays: "30", membershipId: "m-1", orgId: "org-a", qualification: "demo_indoor", activeOnly: "false" });
    expect(JSON.stringify(load.loadTeam.mock.calls[0]?.[0])).toBe(`{"date":"2026-11-20","qualification":"demo_indoor","activeOnly":"false","orgId":"org-a","membershipId":"m-1","status":"expired","expiringWithinDays":"30"}`);
    expect(isValidElement(view) && view.type).toBe(TeamView);
    expect(isValidElement(view) && view.props).toStrictEqual({ live: { marker: "live" } });
    const loading = Loading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "team & capacity" });
    const error = new Error("boom");
    const retry = vi.fn();
    const failed = ErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Team & capacity", error, retry });
  });
});
