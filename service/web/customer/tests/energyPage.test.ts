import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiBaseline, ApiEnergySummary } from "@ac/web/lib/energy";

// Energy & cost of the customer app (FR-C06, FR-C16): the demo outside API mode; the units, baselines and URL selection;
// the highest-use AC as the default when none is chosen (IR815); the daily and per-unit summaries; a VALIDATION refusal
// shown on the page while any other failure reaches the error boundary; and the report export with the monthly e-mail.
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
vi.mock("server-only", () => ({}));
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/energy/_components/energy-demo", () => ({ EnergyDemo: () => null }));
vi.mock("../app/customer/energy/_components/energy-view", () => ({ EnergyView: () => null }));

import { exportReport } from "../app/customer/energy/actions";
import CustomerEnergyPage from "../app/customer/energy/page";
import { EnergyDemo } from "../app/customer/energy/_components/energy-demo";
import { EnergyView } from "../app/customer/energy/_components/energy-view";
import EnergyLoading from "../app/customer/energy/loading";
import EnergyErrorState from "../app/customer/energy/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const NOW = new Date("2026-09-15T01:12:30Z"); // 09:12 in Kuala Lumpur
const units = [{ id: "u-b", displayName: "Bedroom AC" }, { id: "u-a", displayName: "Attic AC" }, { id: "u-c", displayName: "Kitchen AC" }];
const baseline = (id: string, unitIds: string[], kWh: number | null = 70): ApiBaseline => ({
  id, unitIds, version: 1, createdAt: "2026-09-01T00:00:00Z", period: { from: "2026-08-01T00:00:00Z", to: "2026-08-08T00:00:00Z" }, method: "demo_fixed",
  baselineKWh: kWh, quality: { kind: "demo", coverage: 1 }, boundaryId: "ac_input_electricity", boundary: "AC input", assumptions: "demo", source: "demo",
});
const summary = (kWh: number | null, over: Partial<ApiEnergySummary> = {}): ApiEnergySummary => ({
  period: { from: "2026-09-08T16:00:00Z", to: "2026-09-15T01:12:00Z" }, unitIds: ["u-b"],
  totals: { kWh, amountMinor: kWh === null ? null : kWh * 50, savedKWh: null, deltaKWh: null, savingPercentage: null, savedAmountMinor: null, emissionsKg: null, savedEmissionsKg: null },
  currency: "MYR", baselineRef: null, factorRef: null, baselineSnapshot: null, factorSnapshot: null, tariffVersion: "demo-v1", boundaryId: "ac_input_electricity", boundary: "AC input",
  coverage: kWh === null ? null : 1, qualityWarnings: [], ...over,
});
interface SummaryInput { from: string; to: string; unitIds: string[]; baselineId?: string }
const isSummaryInput = (x: unknown): x is SummaryInput => typeof x === "object" && x !== null && "unitIds" in x;
const summaries = () => dal.coreOp.mock.calls.filter((c) => c[0] === "energy.summary").map((c) => c[1]).filter(isSummaryInput);
/** energy.summary per unit set: each AC's kWh on its own, the sum for a set; the period total carries the baseline */
function answer(kWh: Record<string, number | null>, over: (input: SummaryInput) => Partial<ApiEnergySummary> = () => ({})) {
  const known = new Map(Object.entries(kWh));
  dal.coreOp.mockImplementation((op: string, input?: unknown) => {
    if (op === "preferences.get") return Promise.resolve({ locale: "en", timezone: "Asia/Kuala_Lumpur", monthlyReportEmail: false });
    if (!isSummaryInput(input)) return Promise.reject(new Error(`unexpected ${op}`));
    const values = input.unitIds.map((u) => known.get(u) ?? null);
    const total = values.every((v) => v === null) ? null : values.reduce<number>((a, v) => a + (v ?? 0), 0);
    return Promise.resolve(summary(total, { unitIds: input.unitIds, ...over(input) }));
  });
}
const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof EnergyView>>(element) || element.type !== EnergyView) throw new TypeError("not EnergyView");
  return element.props.live;
};
const open = (searchParams: Record<string, string>) => CustomerEnergyPage({ searchParams: Promise.resolve(searchParams), params: Promise.resolve({}) });
const refusal = (code: string) => new dal.CoreError({ code, messageKey: code === "VALIDATION" ? "error.validation" : "error.internal", fieldErrors: code === "VALIDATION" ? { unitIds: "boundary_mismatch" } : {}, retryAfterSeconds: null });

beforeEach(() => {
  vi.resetAllMocks();
  dal.apiMode.mockReturnValue(true);
  dal.coreNow.mockResolvedValue(NOW);
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  const lists = new Map<string, unknown[]>([
    ["units.list", units], ["properties.list", [{ id: "p-1", name: "Home" }]],
    ["baselines.list", [baseline("bl-b", ["u-b"]), baseline("bl-bc", ["u-b", "u-c"]), baseline("bl-c", ["u-c"]), baseline("bl-null", ["u-b"], null)]],
  ]);
  dal.coreAll.mockImplementation((op: string) => Promise.resolve(lists.get(op)));
});

describe("exportReport", () => {
  const input = { month: "2026-08", propertyIds: ["p-1"], sections: ["energy_cost" as const], format: "csv" as const };
  const file = { fileName: "report.csv", mime: "text/csv", size: 2048, generatedAt: "2026-09-15T01:13:00Z", isDemo: true };

  it("returns the demo file; the monthly e-mail choice is stored only when it changed", async () => {
    dal.coreOp.mockResolvedValue(file);
    await expect(exportReport(input, null)).resolves.toStrictEqual({ ok: true, value: file });
    expect(dal.coreOp.mock.calls).toStrictEqual([["energy.exportReport", input]]);
    await exportReport(input, { locale: "ms", timezone: "Asia/Kuala_Lumpur", value: true });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["preferences.update", { locale: "ms", timezone: "Asia/Kuala_Lumpur", monthlyReportEmail: true }, { write: true }]);
  });

  it("a refused export comes back with its fields and stores no preference", async () => {
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { month: "not_ended" }, retryAfterSeconds: null }));
    await expect(exportReport({ ...input, month: "2026-09" }, { locale: "en", timezone: "Asia/Kuala_Lumpur", value: true })).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { month: "not_ended" } });
    expect(dal.coreOp).toHaveBeenCalledOnce();
  });
});

describe("energy page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const page = await open({});
    expect(isValidElement(page) && page.type).toBe(EnergyDemo);
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("with no AC chosen, opens on the one that used the most, with its baseline, the daily chart and the ranking", async () => {
    answer({ "u-b": 10, "u-a": 0, "u-c": 30 }, (x) => (x.baselineId ? { totals: { ...summary(30).totals, savedKWh: 5 }, baselineSnapshot: baseline("bl-c", ["u-c"]) } : {}));
    const live = viewProps(await open({ unitIds: "u-unknown" }));
    expect(live).toMatchObject({ unitIds: ["u-c"], isTopDefault: true, baselineId: "bl-c", baselines: [{ id: "bl-c" }], compare: null, lastMonth: "2026-08" });
    expect(live.units.map((u) => u.name)).toStrictEqual(["Attic AC", "Bedroom AC", "Kitchen AC"]);
    // the ranking leaves out the AC that measured nothing
    expect(live.top.map((r) => [r.unitId, r.kWh])).toStrictEqual([["u-c", 30], ["u-b", 10]]);
    expect(live.query).toBe("unitIds=u-c&period=7d&baselineId=bl-c");
    expect(live.daily).toHaveLength(7);
    expect(live.daily.at(0)?.label).toBe("Wed 9");
    // the baseline as an even daily share: 70 kWh over 6 days and 9 h 12 min, a whole day being 1440 of those minutes
    const minutes = (Date.parse(live.period.to) - Date.parse(live.period.from)) / 60_000;
    expect(live.daily.at(0)?.baseline).toBeCloseTo((70 * 1440) / minutes);
    expect(live.raw).toMatchObject({ kWh: 30, baseline: 70, tariff: "demo-v1", factor: null, coverage: 1 });
    expect(summaries().filter((s) => s.baselineId)).toStrictEqual([{ from: live.period.from, to: live.period.to, unitIds: ["u-c"], baselineId: "bl-c" }]);
  });

  it("an AC chosen in the URL is kept; No baseline is carried to Carbon offsets and draws no baseline", async () => {
    answer({ "u-b": 10, "u-c": 30 });
    const live = viewProps(await open({ unitIds: "u-b", baselineId: "none", period: "today" }));
    expect(live).toMatchObject({ unitIds: ["u-b"], isTopDefault: false, baselineId: null, baselines: [{ id: "bl-b" }] });
    expect(live.query).toBe("unitIds=u-b&period=today&baselineId=none");
    expect(summaries().some((s) => "baselineId" in s)).toBe(false);
    expect(live.raw?.baseline).toBeNull();
    expect(live.daily).toStrictEqual([{ label: "Tue 15", actual: 10, baseline: null }]);
  });

  it("a baseline the savings cannot use (not comparable) is not drawn", async () => {
    answer({ "u-b": 10 }, (x) => (x.baselineId ? { baselineSnapshot: baseline("bl-b", ["u-b"]) } : {}));
    const live = viewProps(await open({ unitIds: "u-b", baselineId: "bl-b" }));
    expect(live.baselineId).toBe("bl-b");
    expect(live.raw?.baseline).toBeNull();
    expect(live.daily.every((d) => d.baseline === null)).toBe(true);
  });

  it("compares units: each with its own baseline, a daily series per unit up to 7 days, the factor named", async () => {
    answer({ "u-b": 10, "u-c": null }, () => ({ factorSnapshot: { id: "f-1", version: 1, region: "MY", year: 2026, kgCO2ePerKWh: 0.5, source: "demo" } }));
    const live = viewProps(await open({ unitIds: "u-b,u-c,u-b" }));
    expect(live.unitIds).toStrictEqual(["u-b", "u-c"]);
    expect(live.baselineId).toBe("bl-bc");
    expect(live.raw?.factor).toBe("MY 2026 · 0.5 kgCO₂e/kWh (demo factor)");
    expect(live.compare?.rows.map((r) => [r.unitId, r.name, r.actual])).toStrictEqual([["u-b", "Bedroom AC", "10.0"], ["u-c", "Kitchen AC", "—"]]);
    expect(live.compare?.series).toStrictEqual([[10, 10, 10, 10, 10, 10, 10], [0, 0, 0, 0, 0, 0, 0]]);
    expect(summaries().filter((s) => s.unitIds.length === 1 && s.baselineId !== undefined).map((s) => [s.unitIds[0], s.baselineId])).toStrictEqual([["u-b", "bl-b"], ["u-c", "bl-c"]]);
  });

  it("over more than 7 days the per-unit daily series is left out; the ranking shares the measured energy", async () => {
    answer({ "u-b": 1, "u-xyz-123456": 2 });
    dal.coreAll.mockImplementation((op: string) => Promise.resolve(op === "units.list" ? [{ id: "u-b", displayName: "Bedroom AC" }, { id: "u-xyz-123456", displayName: "Hall AC" }] : []));
    const live = viewProps(await open({ unitIds: "u-b,u-xyz-123456", period: "30d" }));
    expect(live.daily).toHaveLength(30);
    expect(live.compare?.series).toBeNull();
    expect(live.compare?.rows).toHaveLength(2);
    expect(live.baselines).toStrictEqual([]);
    expect(live.top.map((r) => [r.unitId, r.share])).toStrictEqual([["u-xyz-123456", 2 / 3], ["u-b", 1 / 3]]);
  });

  it("an AC whose summary fails is only left out of the ranking; preferences that fail are null", async () => {
    answer({ "u-b": 10, "u-c": 30 });
    const answered = dal.coreOp.getMockImplementation();
    dal.coreOp.mockImplementation((op: string, input?: unknown) => {
      if (op === "preferences.get") return Promise.reject(new Error("down"));
      if (isSummaryInput(input) && input.unitIds.length === 1 && input.unitIds[0] === "u-a") return Promise.reject(new Error("down"));
      return answered?.(op, input) ?? Promise.reject(new Error("no answer"));
    });
    const live = viewProps(await open({}));
    expect(live.prefs).toBeNull();
    expect(live.unitIds).toStrictEqual(["u-c"]);
    expect(live.top.map((r) => r.unitId)).toStrictEqual(["u-c", "u-b"]);
  });

  it("with no AC that measured anything, falls back to the first by name; with no AC, reads no summary", async () => {
    answer({});
    expect(viewProps(await open({})).unitIds).toStrictEqual(["u-a"]);
    dal.coreOp.mockClear();
    dal.coreAll.mockResolvedValue([]);
    const live = viewProps(await open({}));
    expect(live).toMatchObject({ units: [], unitIds: [], summary: null, top: [], isTopDefault: false });
    expect(summaries()).toStrictEqual([]);
  });

  it("an unusable custom period reads no summary and no ranking", async () => {
    answer({ "u-b": 10 });
    const live = viewProps(await open({ unitIds: "u-b", period: "custom", from: "2026-09-14T10:00", to: "2026-09-14T09:00" }));
    expect(live.period.error).toBe("The end must be after the start");
    expect(live.query).toBe("unitIds=u-b&period=custom&from=2026-09-14T10%3A00&to=2026-09-14T09%3A00&baselineId=bl-b");
    expect(live.summary).toBeNull();
    expect(summaries()).toStrictEqual([]);
  });

  it("a VALIDATION refusal shows on the page; any other failure reaches the error boundary", async () => {
    answer({ "u-b": 10 });
    const answered = dal.coreOp.getMockImplementation();
    let code = "VALIDATION";
    dal.coreOp.mockImplementation((op: string, input?: unknown) => (isSummaryInput(input) && input.baselineId ? Promise.reject(refusal(code)) : answered?.(op, input) ?? Promise.reject(new Error("no answer"))));
    const live = viewProps(await open({ unitIds: "u-b" }));
    expect(live.error).toBe("Unit ids — boundary mismatch");
    expect(live.summary).toBeNull();
    code = "INTERNAL";
    await expect(open({ unitIds: "u-b" })).rejects.toBeInstanceOf(dal.CoreError);
    dal.coreOp.mockImplementation((op: string, input?: unknown) => (isSummaryInput(input) && input.baselineId ? Promise.reject(new Error("socket")) : answered?.(op, input) ?? Promise.reject(new Error("no answer"))));
    await expect(open({ unitIds: "u-b" })).rejects.toThrow("socket");
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = EnergyLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "energy & cost" });
    const failed = EnergyErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Energy & cost", error, retry });
  });
});
