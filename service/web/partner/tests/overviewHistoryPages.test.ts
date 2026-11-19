// The partner Overview (/partner, FR-P01) and Job history (/partner/history, FR-P07) pages: each renders the fixture
// demo outside API mode and, in API mode, its view with what the loader read for the URL's filters (the first value of a
// repeated one); the route states name the screen.
import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement } from "react";

const dal = vi.hoisted(() => ({ apiMode: vi.fn() }));
const loaders = vi.hoisted(() => ({ loadOverview: vi.fn(), loadHistory: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/partner/(overview)/_lib/load", () => ({ loadOverview: loaders.loadOverview }));
vi.mock("../app/partner/history/_lib/load", () => ({ loadHistory: loaders.loadHistory }));
vi.mock("../app/partner/(overview)/_components/overview-demo", () => ({ OverviewDemo: () => null }));
vi.mock("../app/partner/(overview)/_components/overview-view", () => ({ OverviewView: () => null }));
vi.mock("../app/partner/history/_components/history-demo", () => ({ HistoryDemo: () => null }));
vi.mock("../app/partner/history/_components/history-view", () => ({ HistoryView: () => null }));

import PartnerOverviewPage from "../app/partner/(overview)/page";
import OverviewErrorState from "../app/partner/(overview)/error";
import OverviewLoading from "../app/partner/(overview)/loading";
import { OverviewDemo } from "../app/partner/(overview)/_components/overview-demo";
import { OverviewView } from "../app/partner/(overview)/_components/overview-view";
import PartnerHistoryPage from "../app/partner/history/page";
import HistoryErrorState from "../app/partner/history/error";
import HistoryLoading from "../app/partner/history/loading";
import { HistoryDemo } from "../app/partner/history/_components/history-demo";
import { HistoryView } from "../app/partner/history/_components/history-view";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

type Query = Record<string, string | string[]>;
const overview = (query: Query) => PartnerOverviewPage({ params: Promise.resolve({}), searchParams: Promise.resolve(query) });
const history = (query: Query) => PartnerHistoryPage({ params: Promise.resolve({}), searchParams: Promise.resolve(query) });
const shape = (element: unknown) => (isValidElement(element) ? { type: element.type, props: element.props } : null);

beforeEach(() => { vi.clearAllMocks(); });

describe("overview page", () => {
  it("renders the demo outside API mode without reading", async () => {
    dal.apiMode.mockReturnValue(false);
    expect(shape(await overview({ from: "2026-11-02" }))).toStrictEqual({ type: OverviewDemo, props: {} });
    expect(loaders.loadOverview).not.toHaveBeenCalled();
  });

  it("reads the URL's period (none for a repeated or empty value, so the loader takes this week) and renders the view", async () => {
    dal.apiMode.mockReturnValue(true);
    const live = { period: "week" };
    loaders.loadOverview.mockResolvedValue(live);
    expect(shape(await overview({ from: "2026-11-02", to: "2026-11-08" }))).toStrictEqual({ type: OverviewView, props: { live } });
    expect(loaders.loadOverview).toHaveBeenLastCalledWith({ from: "2026-11-02", to: "2026-11-08" });
    await overview({ from: ["2026-11-02", "2026-12-01"], to: "" });
    expect(loaders.loadOverview).toHaveBeenLastCalledWith({});
  });
});

describe("history page", () => {
  it("renders the demo outside API mode without reading", async () => {
    dal.apiMode.mockReturnValue(false);
    expect(shape(await history({ jobId: "job-p02" }))).toStrictEqual({ type: HistoryDemo, props: {} });
    expect(loaders.loadHistory).not.toHaveBeenCalled();
  });

  it("reads the URL's job, tab, sort and period (the first of a repeated value) and renders the view", async () => {
    dal.apiMode.mockReturnValue(true);
    const live = { rows: [] };
    loaders.loadHistory.mockResolvedValue(live);
    const page = await history({ jobId: ["job-p02", "job-p03"], tab: "internal", sort: "oldest", period: "30d" });
    expect(shape(page)).toStrictEqual({ type: HistoryView, props: { live } });
    expect(loaders.loadHistory).toHaveBeenLastCalledWith({ jobId: "job-p02", tab: "internal", sort: "oldest", period: "30d" });
    await history({});
    expect(loaders.loadHistory).toHaveBeenLastCalledWith({});
  });
});

describe("route states", () => {
  it("name the overview and the job history while loading and on an error", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    expect(shape(OverviewLoading())).toStrictEqual({ type: RouteLoading, props: { what: "the overview", variant: "dashboard" } });
    expect(shape(OverviewErrorState({ error, retry }))).toStrictEqual({ type: RouteError, props: { what: "The overview", error, retry } });
    expect(shape(HistoryLoading())).toStrictEqual({ type: RouteLoading, props: { what: "the job history" } });
    expect(shape(HistoryErrorState({ error, retry }))).toStrictEqual({ type: RouteError, props: { what: "The job history", error, retry } });
  });
});
