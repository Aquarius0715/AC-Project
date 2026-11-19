// The partner Jobs list (/partner/jobs, FR-P01) and AC detail (/partner/units/[id], FR-P04) pages: each renders the
// fixture demo outside API mode and, in API mode, its view with what the loader read for the URL; the jobs list maps the
// overview's status=… links to its tab, and an AC outside the access window is not found. The route states name the
// screen.
import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement } from "react";

const dal = vi.hoisted(() => ({ apiMode: vi.fn() }));
const loaders = vi.hoisted(() => ({ loadJobs: vi.fn(), loadUnit: vi.fn() }));
const navigation = vi.hoisted(() => ({ notFound: vi.fn(() => { throw new Error("NEXT_NOT_FOUND"); }) }));
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("next/navigation", () => navigation);
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/partner/jobs/(list)/_lib/load", () => ({ loadJobs: loaders.loadJobs }));
vi.mock("../app/partner/units/[id]/_lib/load", () => ({ loadUnit: loaders.loadUnit }));
vi.mock("../app/partner/jobs/(list)/_components/jobs-demo", () => ({ JobsDemo: () => null }));
vi.mock("../app/partner/jobs/(list)/_components/jobs-view", () => ({ JobsView: () => null }));
vi.mock("../app/partner/units/[id]/_components/mock-unit", () => ({ MockPartnerUnit: () => null }));
vi.mock("../app/partner/units/[id]/_components/unit-detail", () => ({ PartnerUnitDetail: () => null }));

import PartnerJobsPage from "../app/partner/jobs/(list)/page";
import JobsErrorState from "../app/partner/jobs/(list)/error";
import JobsLoading from "../app/partner/jobs/(list)/loading";
import { JobsDemo } from "../app/partner/jobs/(list)/_components/jobs-demo";
import { JobsView } from "../app/partner/jobs/(list)/_components/jobs-view";
import PartnerUnitPage from "../app/partner/units/[id]/page";
import UnitErrorState from "../app/partner/units/[id]/error";
import UnitLoading from "../app/partner/units/[id]/loading";
import { MockPartnerUnit } from "../app/partner/units/[id]/_components/mock-unit";
import { PartnerUnitDetail } from "../app/partner/units/[id]/_components/unit-detail";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

type Query = Record<string, string | string[]>;
const jobs = (query: Query) => PartnerJobsPage({ params: Promise.resolve({}), searchParams: Promise.resolve(query) });
const unit = (id: string, query: Query) => PartnerUnitPage({ params: Promise.resolve({ id }), searchParams: Promise.resolve(query) });
const shape = (element: unknown) => (isValidElement(element) ? { type: element.type, props: element.props } : null);
// toHaveBeenCalledWith treats a key set to undefined as missing, so {} means "no filter read from the URL"
const noFilters = {};

beforeEach(() => { vi.clearAllMocks(); });

describe("jobs list page", () => {
  it("renders the demo with the URL's status outside API mode, without reading", async () => {
    dal.apiMode.mockReturnValue(false);
    expect(shape(await jobs({ status: "offered", tab: "active" }))).toStrictEqual({ type: JobsDemo, props: { status: "offered" } });
    expect(loaders.loadJobs).not.toHaveBeenCalled();
  });

  it("reads the URL's tab, sort, period and page and renders the view", async () => {
    dal.apiMode.mockReturnValue(true);
    const live = { rows: [] };
    loaders.loadJobs.mockResolvedValue(live);
    const page = await jobs({ tab: "review", sort: "oldest", from: "2026-11-02", to: "2026-11-08", cursor: "c2", page: "2" });
    expect(shape(page)).toStrictEqual({ type: JobsView, props: { live } });
    expect(loaders.loadJobs).toHaveBeenCalledExactlyOnceWith({ tab: "review", sort: "oldest", from: "2026-11-02", to: "2026-11-08", cursor: "c2", page: "2" });
  });

  it("drops a repeated or empty value", async () => {
    dal.apiMode.mockReturnValue(true);
    await jobs({ tab: ["review", "all"], sort: "", from: ["2026-11-02", "2026-12-01"] });
    expect(loaders.loadJobs).toHaveBeenLastCalledWith(noFilters);
  });

  it.each([
    ["offered", "offered"],
    ["active", "active"],
    ["submitted", "review"],
    ["completed", "completed"],
  ])("maps the overview's status=%s to the %s tab", async (status, tab) => {
    dal.apiMode.mockReturnValue(true);
    await jobs({ status });
    expect(loaders.loadJobs).toHaveBeenLastCalledWith({ ...noFilters, tab });
  });

  it("prefers tab over status, and passes no tab for an unknown status or an object key", async () => {
    dal.apiMode.mockReturnValue(true);
    await jobs({ tab: "all", status: "offered" });
    expect(loaders.loadJobs).toHaveBeenLastCalledWith({ ...noFilters, tab: "all" });
    for (const status of ["cancelled", "constructor", "toString", "__proto__"]) {
      await jobs({ status });
      expect(loaders.loadJobs).toHaveBeenLastCalledWith(noFilters);
    }
  });
});

describe("unit page", () => {
  it("renders the fixture unit outside API mode, without reading", async () => {
    dal.apiMode.mockReturnValue(false);
    expect(shape(await unit("ac-a01", { jobId: "job-p02" }))).toStrictEqual({ type: MockPartnerUnit, props: { id: "ac-a01" } });
    expect(loaders.loadUnit).not.toHaveBeenCalled();
  });

  it("reads the unit with the URL's job and renders the detail", async () => {
    dal.apiMode.mockReturnValue(true);
    const live = { unit: { id: "ac-a01" } };
    loaders.loadUnit.mockResolvedValue(live);
    expect(shape(await unit("ac-a01", { jobId: "job-p02" }))).toStrictEqual({ type: PartnerUnitDetail, props: { live } });
    expect(loaders.loadUnit).toHaveBeenCalledExactlyOnceWith("ac-a01", "job-p02");
  });

  it("passes no job for a missing, empty or repeated jobId", async () => {
    dal.apiMode.mockReturnValue(true);
    loaders.loadUnit.mockResolvedValue({ unit: { id: "ac-a01" } });
    const queries: Query[] = [{}, { jobId: "" }, { jobId: ["job-p02", "job-p03"] }];
    for (const query of queries) {
      await unit("ac-a01", query);
      expect(loaders.loadUnit).toHaveBeenLastCalledWith("ac-a01", undefined);
    }
    expect(navigation.notFound).not.toHaveBeenCalled();
  });

  it("is not found when the loader finds no unit (outside the access window)", async () => {
    dal.apiMode.mockReturnValue(true);
    loaders.loadUnit.mockResolvedValue(null);
    await expect(unit("ac-b01", {})).rejects.toThrow("NEXT_NOT_FOUND");
    expect(navigation.notFound).toHaveBeenCalledOnce();
  });
});

describe("route states", () => {
  it("name the jobs and the AC while loading and on an error", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    expect(shape(JobsLoading())).toStrictEqual({ type: RouteLoading, props: { what: "jobs" } });
    expect(shape(JobsErrorState({ error, retry }))).toStrictEqual({ type: RouteError, props: { what: "Jobs", error, retry } });
    expect(shape(UnitLoading())).toStrictEqual({ type: RouteLoading, props: { what: "this AC", variant: "detail" } });
    expect(shape(UnitErrorState({ error, retry }))).toStrictEqual({ type: RouteError, props: { what: "This AC", error, retry } });
  });
});
