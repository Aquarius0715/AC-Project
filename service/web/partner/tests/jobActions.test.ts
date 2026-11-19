import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement } from "react";

// The Server Actions of a contractor's job (FR-P02, FR-P05): accept, decline, propose another time and withdraw at the
// job version (a decline does not re-read, as the job leaves the company; a CONFLICT always re-reads), and the quality
// review at the report and job versions (it re-reads whatever happens). Both pages render the fixture demo outside API
// mode, the live view in API mode and not found for another company's job; their route states name the screen.
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
const detailLoad = vi.hoisted(() => ({ loadJobDetail: vi.fn() }));
const reviewLoad = vi.hoisted(() => ({ loadReview: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/partner/jobs/[id]/(detail)/_lib/load", () => detailLoad);
vi.mock("../app/partner/jobs/[id]/(detail)/_components/job-demo", () => ({ JobDemo: () => null }));
vi.mock("../app/partner/jobs/[id]/(detail)/_components/job-view", () => ({ JobDetailView: () => null }));
vi.mock("../app/partner/jobs/[id]/review/_lib/load", () => reviewLoad);
vi.mock("../app/partner/jobs/[id]/review/_components/review-demo", () => ({ ReviewDemo: () => null }));
vi.mock("../app/partner/jobs/[id]/review/_components/review-view", () => ({ ReviewView: () => null }));

import { redirect } from "next/navigation";
import { acceptOffer, declineOffer, proposeSlot, withdrawProposal } from "../app/partner/jobs/[id]/(detail)/actions";
import { reviewReport } from "../app/partner/jobs/[id]/review/actions";
import PartnerJobPage from "../app/partner/jobs/[id]/(detail)/page";
import PartnerReviewPage from "../app/partner/jobs/[id]/review/page";
import { JobDemo } from "../app/partner/jobs/[id]/(detail)/_components/job-demo";
import { JobDetailView } from "../app/partner/jobs/[id]/(detail)/_components/job-view";
import { ReviewDemo } from "../app/partner/jobs/[id]/review/_components/review-demo";
import { ReviewView } from "../app/partner/jobs/[id]/review/_components/review-view";
import JobLoading from "../app/partner/jobs/[id]/(detail)/loading";
import JobErrorState from "../app/partner/jobs/[id]/(detail)/error";
import ReviewLoading from "../app/partner/jobs/[id]/review/loading";
import ReviewErrorState from "../app/partner/jobs/[id]/review/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

beforeEach(() => {
  vi.resetAllMocks();
  dal.coreOp.mockResolvedValue({ id: "job-p02", status: "accepted" });
});

const slot = { startAt: "2026-11-25T01:00:00Z", endAt: "2026-11-25T03:00:00Z" };

describe("offer actions", () => {
  it("send each operation at the job version, and re-read after all but a decline", async () => {
    await expect(acceptOffer("job-p02", "of-1", "terms-3", 7)).resolves.toStrictEqual({ ok: true, value: { id: "job-p02", status: "accepted" } });
    expect(dal.coreOp).toHaveBeenLastCalledWith("jobs.accept", { jobId: "job-p02", offerId: "of-1", termsVersion: "terms-3" }, { write: true, expectedVersion: 7 });
    expect(cache.refresh).toHaveBeenCalledOnce();
    await expect(declineOffer("job-p02", "of-1", 7, "No technician that week")).resolves.toMatchObject({ ok: true });
    expect(dal.coreOp).toHaveBeenLastCalledWith("jobs.decline", { jobId: "job-p02", offerId: "of-1", reason: "No technician that week" }, { write: true, expectedVersion: 7 });
    expect(cache.refresh).toHaveBeenCalledOnce();
    await expect(proposeSlot("job-p02", "of-1", 7, slot, "m-1", "Earlier is better")).resolves.toMatchObject({ ok: true });
    expect(dal.coreOp).toHaveBeenLastCalledWith("jobs.proposePartnerSlot", { jobId: "job-p02", offerId: "of-1", technicianMembershipId: "m-1", reason: "Earlier is better", slot }, { write: true, expectedVersion: 7 });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
    await expect(withdrawProposal("job-p02", "pp-1", 8)).resolves.toMatchObject({ ok: true });
    expect(dal.coreOp).toHaveBeenLastCalledWith("jobs.withdrawPartnerSlot", { jobId: "job-p02", proposalId: "pp-1" }, { write: true, expectedVersion: 8 });
    expect(cache.refresh).toHaveBeenCalledTimes(3);
  });

  it("return a refusal with its code, key, fields and wait, re-reading on a CONFLICT only, even for a decline", async () => {
    const refused = new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { reason: "error.length" }, retryAfterSeconds: null, correlationId: "c-1" });
    dal.coreOp.mockRejectedValueOnce(refused);
    await expect(declineOffer("job-p02", "of-1", 7, "")).resolves.toStrictEqual({ ok: false, code: "VALIDATION", messageKey: "error.validation", fieldErrors: { reason: "error.length" }, retryAfterSeconds: null });
    expect(cache.refresh).not.toHaveBeenCalled();
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("CONFLICT"));
    await expect(declineOffer("job-p02", "of-1", 7, "Busy")).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledOnce();
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("FORBIDDEN"));
    await expect(acceptOffer("job-p02", "of-1", "terms-3", 7)).resolves.toMatchObject({ ok: false, code: "FORBIDDEN" });
    expect(cache.refresh).toHaveBeenCalledOnce();
  });

  it("report an unreachable Core API as unavailable, and let the DAL's redirect to sign-in through", async () => {
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(acceptOffer("job-p02", "of-1", "terms-3", 7)).resolves.toStrictEqual({ ok: false, code: "UNAVAILABLE", messageKey: "error.unavailable", fieldErrors: {} });
    dal.coreOp.mockImplementationOnce(() => { redirect("/bff/auth/login"); });
    await expect(withdrawProposal("job-p02", "pp-1", 8)).rejects.toMatchObject({ digest: expect.stringMatching(/^NEXT_REDIRECT/) as unknown });
    expect(cache.refresh).not.toHaveBeenCalled();
  });
});

describe("reviewReport", () => {
  it("accepts without a reason and returns with one, at the report and job versions, and re-reads each time", async () => {
    await expect(reviewReport("job-p02", 9, 2, "accept", null)).resolves.toStrictEqual({ ok: true, value: { status: "accepted" } });
    expect(dal.coreOp).toHaveBeenLastCalledWith("jobs.review", { jobId: "job-p02", reportVersion: 2, decision: "accept", reviewMode: "normal" }, { write: true, expectedVersion: 9 });
    dal.coreOp.mockResolvedValueOnce({ status: "in_progress" });
    await expect(reviewReport("job-p02", 9, 2, "return", "Photo of the filter missing")).resolves.toStrictEqual({ ok: true, value: { status: "in_progress" } });
    expect(dal.coreOp).toHaveBeenLastCalledWith("jobs.review", { jobId: "job-p02", reportVersion: 2, decision: "return", reviewMode: "normal", reason: "Photo of the filter missing" }, { write: true, expectedVersion: 9 });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });

  it("re-reads after a refusal, an outage and the redirect to sign-in too", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError("CONFLICT"));
    await expect(reviewReport("job-p02", 9, 2, "accept", null)).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    dal.coreOp.mockRejectedValueOnce(new TypeError("fetch failed"));
    await expect(reviewReport("job-p02", 9, 2, "accept", null)).resolves.toMatchObject({ ok: false, code: "UNAVAILABLE" });
    dal.coreOp.mockImplementationOnce(() => { redirect("/bff/auth/login"); });
    await expect(reviewReport("job-p02", 9, 2, "accept", null)).rejects.toMatchObject({ digest: expect.stringMatching(/^NEXT_REDIRECT/) as unknown });
    expect(cache.refresh).toHaveBeenCalledTimes(3);
  });
});

const notFoundDigest = { digest: "NEXT_HTTP_ERROR_FALLBACK;404" };
const jobPage = (id: string) => PartnerJobPage({ params: Promise.resolve({ id }), searchParams: Promise.resolve({}) });
const reviewPage = (id: string) => PartnerReviewPage({ params: Promise.resolve({ id }), searchParams: Promise.resolve({}) });

describe("job detail page", () => {
  it("renders the demo outside API mode, the live view in API mode, and not found when the loader finds nothing", async () => {
    dal.apiMode.mockReturnValue(false);
    const demo = await jobPage("job-p02");
    expect(isValidElement(demo) && demo.type).toBe(JobDemo);
    expect(isValidElement(demo) && demo.props).toStrictEqual({ id: "job-p02" });
    expect(detailLoad.loadJobDetail).not.toHaveBeenCalled();
    dal.apiMode.mockReturnValue(true);
    detailLoad.loadJobDetail.mockResolvedValueOnce({ marker: "live" });
    const view = await jobPage("job-p02");
    expect(detailLoad.loadJobDetail).toHaveBeenCalledExactlyOnceWith("job-p02");
    expect(isValidElement(view) && view.type).toBe(JobDetailView);
    expect(isValidElement(view) && view.props).toStrictEqual({ live: { marker: "live" } });
    detailLoad.loadJobDetail.mockResolvedValueOnce(null);
    await expect(jobPage("job-x")).rejects.toMatchObject(notFoundDigest);
  });
});

describe("quality review page", () => {
  it("renders the demo outside API mode and the live view in API mode", async () => {
    dal.apiMode.mockReturnValue(false);
    const demo = await reviewPage("job-p02");
    expect(isValidElement(demo) && demo.type).toBe(ReviewDemo);
    expect(isValidElement(demo) && demo.props).toStrictEqual({ id: "job-p02" });
    expect(reviewLoad.loadReview).not.toHaveBeenCalled();
    dal.apiMode.mockReturnValue(true);
    reviewLoad.loadReview.mockResolvedValueOnce({ marker: "live" });
    const view = await reviewPage("job-p02");
    expect(reviewLoad.loadReview).toHaveBeenCalledExactlyOnceWith("job-p02");
    expect(isValidElement(view) && view.type).toBe(ReviewView);
    expect(isValidElement(view) && view.props).toStrictEqual({ live: { marker: "live" } });
  });

  it("is not found for a job outside the delegation (not found, forbidden, a refused job id), and fails on any other error", async () => {
    dal.apiMode.mockReturnValue(true);
    const outside = [
      new dal.CoreError("NOT_FOUND"),
      new dal.CoreError("FORBIDDEN"),
      new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { jobId: "error.invalid" }, retryAfterSeconds: null }),
    ];
    for (const error of outside) {
      reviewLoad.loadReview.mockRejectedValueOnce(error);
      await expect(reviewPage("job-x")).rejects.toMatchObject(notFoundDigest);
    }
    const other = new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { reportVersion: "error.invalid" }, retryAfterSeconds: null });
    reviewLoad.loadReview.mockRejectedValueOnce(other);
    await expect(reviewPage("job-p02")).rejects.toBe(other);
    const outage = new TypeError("fetch failed");
    reviewLoad.loadReview.mockRejectedValueOnce(outage);
    await expect(reviewPage("job-p02")).rejects.toBe(outage);
  });
});

describe("route states", () => {
  it("name the job and the quality review while loading and on an error", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const states = [
      [JobLoading(), JobErrorState({ error, retry }), "the job", "The job"],
      [ReviewLoading(), ReviewErrorState({ error, retry }), "quality review", "Quality review"],
    ] as const;
    for (const [loading, failed, what, title] of states) {
      expect(loading.type).toBe(RouteLoading);
      expect(loading.props).toStrictEqual({ what, variant: "detail" });
      expect(failed.type).toBe(RouteError);
      expect(failed.props).toStrictEqual({ what: title, error, retry });
    }
  });
});
