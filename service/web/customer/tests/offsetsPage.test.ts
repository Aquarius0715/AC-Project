import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiEnergySummary } from "@ac/web/lib/energy";
import type { ApiOffsetRecord } from "@ac/web/lib/offsets";

// Carbon offsets of the customer app (FR-C13): the demo outside API mode; the estimate for the selection carried from
// Energy & cost (No baseline included), the way back keeping that selection, the customer's demo records, an unusable
// selection without an estimate; the demo quote and the demo request as Server Actions.
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
  return { coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(), apiMode: vi.fn(), coreAll: vi.fn(), coreDisplay: vi.fn(), coreNow: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/energy/offsets/_components/offsets-demo", () => ({ OffsetsDemo: () => null }));
vi.mock("../app/customer/energy/offsets/_components/offsets-view", () => ({ OffsetsView: () => null }));

import { getQuote, requestOffset } from "../app/customer/energy/offsets/actions";
import CustomerOffsetsPage from "../app/customer/energy/offsets/page";
import { OffsetsDemo } from "../app/customer/energy/offsets/_components/offsets-demo";
import { OffsetsView } from "../app/customer/energy/offsets/_components/offsets-view";
import OffsetsLoading from "../app/customer/energy/offsets/loading";
import OffsetsErrorState from "../app/customer/energy/offsets/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const NOW = new Date("2026-09-15T01:12:30Z");
const units = [{ id: "u-b", displayName: "Bedroom AC" }, { id: "u-a", displayName: "Attic AC" }];
const summary = (over: Partial<ApiEnergySummary> = {}): ApiEnergySummary => ({
  period: { from: "2026-09-08T16:00:00Z", to: "2026-09-15T01:12:00Z" }, unitIds: ["u-b"],
  totals: { kWh: 20, amountMinor: 1000, savedKWh: 4, deltaKWh: -4, savingPercentage: 16.7, savedAmountMinor: 200, emissionsKg: 10, savedEmissionsKg: 2 },
  currency: "MYR", baselineRef: null, factorRef: null, baselineSnapshot: null, factorSnapshot: { id: "f-1", version: 1, region: "MY", year: 2026, kgCO2ePerKWh: 0.5, source: "demo" },
  tariffVersion: "demo-v1", boundaryId: "ac_input_electricity", boundary: "AC input", coverage: 1, qualityWarnings: [], ...over,
});
const offset = (id: string, over: Partial<ApiOffsetRecord> = {}): ApiOffsetRecord => ({
  id, version: 1, createdAt: "2026-09-14T02:00:00Z", quoteId: "q-1", customerId: "org-a", amountKg: 1.5, attempts: [], currentAttemptId: null, state: "demo_requested", previousState: null,
  purchaseRef: null, retirementRef: null, demoCertificateRef: null, eventHistory: [], ...over,
});
const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof OffsetsView>>(element) || element.type !== OffsetsView) throw new TypeError("not OffsetsView");
  return element.props.live;
};
const open = (searchParams: Record<string, string>) => CustomerOffsetsPage({ searchParams: Promise.resolve(searchParams), params: Promise.resolve({}) });
const summaryCalls = () => dal.coreOp.mock.calls.filter((c) => c[0] === "energy.summary").map((c) => c[1]);

beforeEach(() => {
  vi.resetAllMocks();
  dal.apiMode.mockReturnValue(true);
  dal.coreNow.mockResolvedValue(NOW);
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  const records = [offset("off-1-aaaaaaaa", { demoCertificateRef: "DEMO-CERT-1", retirementRef: "RET-1" }), offset("off-2-bbbbbbbb", { retirementRef: "RET-2" }), offset("off-3-cccccccc")];
  dal.coreAll.mockImplementation((op: string) => Promise.resolve(op === "units.list" ? units : records));
  dal.coreOp.mockResolvedValue(summary());
});

describe("offsets actions", () => {
  const input = { purpose: "Demo offset", amountKg: 1, period: { from: "2026-09-08T16:00:00Z", to: "2026-09-15T01:12:00Z" }, unitIds: ["u-b"] };

  it("getQuote asks offsets.preview as a write; a refusal comes back with its fields", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "q-1" });
    await expect(getQuote(input)).resolves.toStrictEqual({ ok: true, value: { id: "q-1" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["offsets.preview", input, { write: true }]);
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { amountKg: "out_of_range" }, retryAfterSeconds: null }));
    await expect(getQuote({ ...input, amountKg: 0 })).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { amountKg: "out_of_range" } });
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("requestOffset records a confirmed demo request with the quote version and re-reads, also after a CONFLICT", async () => {
    dal.coreOp.mockResolvedValueOnce(offset("off-9"));
    await expect(requestOffset("q-1", 2)).resolves.toMatchObject({ ok: true, value: { id: "off-9" } });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["offsets.simulate", { event: "request", demoConfirmed: true, quoteId: "q-1", quoteVersion: 2 }, { write: true }]);
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "CONFLICT", messageKey: "error.conflict", fieldErrors: {}, retryAfterSeconds: null }));
    await expect(requestOffset("q-1", 2)).resolves.toMatchObject({ ok: false, code: "CONFLICT" });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

describe("offsets page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const page = await open({});
    expect(isValidElement(page) && page.type).toBe(OffsetsDemo);
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("estimates the carried selection with its baseline, keeps it for the way back and lists the records newest first", async () => {
    const live = viewProps(await open({ unitIds: "u-b,u-a", period: "custom", from: "2026-09-10T00:00", to: "2026-09-12T00:00", baselineId: "bl-1" }));
    expect(summaryCalls()).toStrictEqual([{ from: "2026-09-09T16:00:00.000Z", to: "2026-09-11T16:00:00.000Z", unitIds: ["u-b", "u-a"], baselineId: "bl-1" }]);
    expect(dal.coreAll).toHaveBeenCalledWith("offsets.list", { sort: { field: "createdAt", direction: "desc" } });
    expect(live).toMatchObject({ unitIds: ["u-b", "u-a"], subject: "Bedroom AC, Attic AC", estimate: { emissions: "10.0 kgCO₂e", savedEmissions: "Reduction 2.0 kgCO₂e", comparable: true, factor: "MY 2026" } });
    expect(live.period).toMatchObject({ from: "2026-09-09T16:00:00.000Z", to: "2026-09-11T16:00:00.000Z" });
    expect(live.back).toBe("unitIds=u-b%2Cu-a&period=custom&from=2026-09-10T00%3A00&to=2026-09-12T00%3A00&baselineId=bl-1");
    // the proof is the demo certificate, else the retirement reference
    expect(live.records.map((r) => [r.id, r.proof])).toStrictEqual([["off-1-aaaaaaaa", "DEMO-CERT-1"], ["off-2-bbbbbbbb", "RET-2"], ["off-3-cccccccc", null]]);
  });

  it("No baseline is estimated without one and kept for the way back; an unknown period is 7 days of the first AC by name", async () => {
    dal.coreOp.mockResolvedValue(summary({ factorSnapshot: null, totals: { ...summary().totals, savedKWh: null, savedEmissionsKg: null } }));
    const live = viewProps(await open({ baselineId: "none", period: "1y" }));
    expect(summaryCalls()).toMatchObject([{ unitIds: ["u-a"] }]);
    expect(summaryCalls().some((s) => typeof s === "object" && s !== null && "baselineId" in s)).toBe(false);
    expect(live.estimate).toMatchObject({ comparable: false, factor: "emission factor missing" });
    expect(live.back).toBe("unitIds=u-a&period=7d&baselineId=none");
  });

  it("a VALIDATION refusal leaves no estimate; any other failure reaches the error boundary", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: {}, retryAfterSeconds: null }));
    expect(viewProps(await open({ unitIds: "u-b" })).estimate).toBeNull();
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "INTERNAL", messageKey: "error.internal", fieldErrors: {}, retryAfterSeconds: null }));
    await expect(open({ unitIds: "u-b" })).rejects.toBeInstanceOf(dal.CoreError);
    dal.coreOp.mockRejectedValueOnce(new Error("socket"));
    await expect(open({ unitIds: "u-b" })).rejects.toThrow("socket");
  });

  it("an unusable period or no AC reads no summary", async () => {
    expect(viewProps(await open({ period: "custom", from: "2026-09-12T00:00", to: "2026-09-10T00:00" })).estimate).toBeNull();
    dal.coreAll.mockResolvedValue([]);
    const live = viewProps(await open({}));
    expect(live).toMatchObject({ unitIds: [], subject: "", estimate: null, records: [] });
    expect(summaryCalls()).toStrictEqual([]);
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = OffsetsLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "carbon offsets" });
    const failed = OffsetsErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Carbon offsets", error, retry });
  });
});
