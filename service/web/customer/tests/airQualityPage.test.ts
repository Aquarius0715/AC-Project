import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps } from "react";
import type { ApiPropertyRow, ApiSpaceRow, ApiUnitRow } from "@ac/web/lib/assets";
import type { ApiUnitDetail } from "@ac/web/lib/units";

// The air-quality page of the customer app (FR-C07): the demo outside API mode; no units → the empty view; otherwise the
// selected unit's series per metric over the window (5-minute slots for a day, hourly for 7 days, IR605/IR607), the
// room's ventilation history only for a unit in a room, and the fresh-air guidance only for a unit with low ventilation.
// logVentilation re-reads the page whether the Core API takes it or refuses it (IR110).
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
  return { coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(), apiMode: vi.fn(), coreAll: vi.fn(), corePrincipal: vi.fn(), coreDisplay: vi.fn(), coreNow: vi.fn(), CoreError };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/customer/air-quality/_components/air-quality-demo", () => ({ AirQualityDemo: () => null }));
vi.mock("../app/customer/air-quality/_components/air-quality-view", () => ({ AirQualityView: () => null }));

import { logVentilation } from "../app/customer/air-quality/actions";
import CustomerAirQualityPage from "../app/customer/air-quality/page";
import { AirQualityDemo } from "../app/customer/air-quality/_components/air-quality-demo";
import { AirQualityView } from "../app/customer/air-quality/_components/air-quality-view";
import AirQualityLoading from "../app/customer/air-quality/loading";
import AirQualityErrorState from "../app/customer/air-quality/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";

const NOW = new Date("2026-09-15T01:12:30Z");
const property: ApiPropertyRow = {
  id: "p-1", name: "Home", version: 1, customerOrgId: "org-a", kind: "home", address: null, accessInstructions: null, location: null, geofenceRadiusMeters: null, archived: false, updatedAt: "2026-09-01T00:00:00Z",
};
const bedroom: ApiSpaceRow = { id: "s-bed", propertyId: "p-1", name: "Bedroom", version: 1, parentSpaceId: null, kind: "room", archived: false };
const unit = (id: string, displayName: string, spaceId: string | null): ApiUnitRow => ({
  id, displayName, spaceId, version: 1, customerOrgId: "org-a", propertyId: "p-1", modelId: "cap-1", type: "split", installState: "installed", installedAt: null, serviceScope: [],
  alertPolicyIds: [], archived: false, capabilityVersion: 1, connection: "online", lastSeenAt: null, warrantyEndsAt: null, effectivePowerState: "on", activeAlertCount: 0,
});
const reading = (metric: "co2" | "pm25", value: number): ApiUnitDetail["latestMeasurements"][number] => ({
  metric, value, id: `m-${metric}`, unitId: "u-1", sensorId: "sen-1", unit: metric === "co2" ? "ppm" : "µg/m³", observedAt: "2026-09-15T01:12:00Z", origin: "measured", quality: "valid", qualityReason: null,
});
const detail = (id: string, displayName: string, over: Partial<ApiUnitDetail> = {}): ApiUnitDetail => ({
  id, displayName, version: 1, spaceId: null, alertPolicyIds: [], connection: "online", effectivePowerState: "on",
  observedState: { power: true, celsius: null, mode: null, fanLevel: null, observedAt: null }, lastSeenAt: null, latestMeasurements: [],
  capabilities: { manufacturer: "Daikin", model: "FTKF", control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [] },
  iotModel: null, effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: [], address: null, accessInstructions: null }, installedAt: null, installState: "installed", modelId: "cap-1", capabilityVersion: 1, serviceScope: [], components: [], ...over,
});
const series = (buckets: { start: string; value: number | null; count: number }[] | null, allergen: unknown = null) => ({ items: [], nextCursor: null, total: 7, allergenObservation: allergen, buckets });
const vent = { id: "v-1", spaceId: "s-bed", unitId: "u-1", method: "window_opened", durationMinutes: 15, co2AtLog: null, loggedByMembershipId: "m-me", loggedAt: "2026-09-14T10:10:00Z" };

/** the unit detail, the series and the ventilation list the page reads, keyed by op */
function answer(unitDetail: ApiUnitDetail, buckets: Parameters<typeof series>[0]) {
  dal.coreOp.mockImplementation((op: string) => {
    if (op === "units.get") return Promise.resolve(unitDetail);
    if (op === "telemetry.series") return Promise.resolve(series(buckets));
    return Promise.resolve({ items: [vent], total: 1 });
  });
}
const viewProps = (element: unknown) => {
  if (!isValidElement<ComponentProps<typeof AirQualityView>>(element) || element.type !== AirQualityView) throw new TypeError("not AirQualityView");
  return element.props.live;
};
const open = (searchParams: Record<string, string>) => CustomerAirQualityPage({ searchParams: Promise.resolve(searchParams), params: Promise.resolve({}) });
const calls = (op: string) => dal.coreOp.mock.calls.filter((c) => c[0] === op).map((c) => c[1]);

beforeEach(() => {
  vi.resetAllMocks();
  dal.apiMode.mockReturnValue(true);
  dal.corePrincipal.mockResolvedValue({ membershipId: "m-me" });
  dal.coreNow.mockResolvedValue(NOW);
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  const lists = new Map<string, unknown[]>([["properties.list", [property]], ["spaces.list", [bedroom]], ["units.list", [unit("u-1", "Bedroom AC", "s-bed"), unit("u-2", "Porch AC", null)]]]);
  dal.coreAll.mockImplementation((op: string) => Promise.resolve(lists.get(op)));
});

describe("logVentilation", () => {
  it("records at the Core API and re-reads; a refusal comes back with its fields, also after a re-read", async () => {
    const input = { spaceId: "s-bed", unitId: "u-1", method: "window_opened" as const, durationMinutes: 15 };
    dal.coreOp.mockResolvedValueOnce(vent);
    await expect(logVentilation(input)).resolves.toStrictEqual({ ok: true, value: vent });
    expect(dal.coreOp.mock.lastCall).toStrictEqual(["ventilation.log", input, { write: true }]);
    dal.coreOp.mockRejectedValueOnce(new dal.CoreError({ code: "VALIDATION", messageKey: "error.validation", fieldErrors: { durationMinutes: "out_of_range" }, retryAfterSeconds: null }));
    await expect(logVentilation({ ...input, durationMinutes: 241 })).resolves.toMatchObject({ ok: false, code: "VALIDATION", fieldErrors: { durationMinutes: "out_of_range" } });
    expect(cache.refresh).toHaveBeenCalledTimes(2);
  });
});

describe("air-quality page", () => {
  it("renders the demo outside API mode without reading the Core API", async () => {
    dal.apiMode.mockReturnValue(false);
    const page = await open({});
    expect(isValidElement(page) && page.type).toBe(AirQualityDemo);
    expect(dal.coreAll).not.toHaveBeenCalled();
  });

  it("without a unit, renders the empty view and reads no unit", async () => {
    dal.coreAll.mockResolvedValue([]);
    expect(viewProps(await open({ unitId: "u-1" }))).toBeNull();
    expect(dal.coreOp).not.toHaveBeenCalled();
  });

  it("a unit in a room over 7 days: hourly slots per metric, the room's history and the fresh-air guidance", async () => {
    answer(detail("u-1", "Bedroom AC", {
      latestMeasurements: [reading("co2", 1200), reading("pm25", 10)],
      capabilities: { manufacturer: "Daikin", model: "FTKF", control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: true, ventilationLevels: ["low"] },
      iotModel: { id: "iot-1", manufacturer: "Lab", model: "Node", sensors: [{ metric: "co2", unit: "ppm", staleAfterSeconds: 900 }], firmwareCandidates: [] },
    }), [{ start: "2026-09-15T00:00:00Z", value: 900, count: 12 }, { start: "2026-09-15T01:00:00Z", value: null, count: 0 }]);
    const live = viewProps(await open({ unitId: "u-1", metric: "co2,pm25", period: "7d" }));
    const asked = calls("telemetry.series");
    expect(asked).toMatchObject([{ metric: "co2", bucketMinutes: 60, unitIds: ["u-1"] }, { metric: "pm25", bucketMinutes: 60, unitIds: ["u-1"] }]);
    expect(calls("ventilation.list")).toStrictEqual([{ filters: { spaceId: "s-bed" }, limit: 10 }]);
    expect(live).toMatchObject({ roomKey: "s-bed", spaceId: "s-bed", unitId: "u-1", unitName: "Bedroom AC", path: "Home › Bedroom", freshAir: true, metric: "co2", metrics: ["co2", "pm25"], period: "7d" });
    expect(live?.chart.title).toBe("CO2 · PM2.5 — last 7 days");
    expect(live?.chart.series.map((s) => [s.metric, s.hasSensor, s.points])).toStrictEqual([["co2", true, [900, null]], ["pm25", false, [900, null]]]);
    expect(live?.chart.gaps).toBe(true);
    expect(live?.chart.times).toHaveLength(2);
    expect(live?.tables.map((t) => [t.metric, t.rows.length])).toStrictEqual([["co2", 2], ["pm25", 2]]);
    expect(live?.cards.map((c) => c.metric)).toStrictEqual(["co2", "pm25", "temperature", "humidity"]);
    expect(live?.history).toMatchObject({ total: 1, rows: [{ id: "v-1", by: "by you" }] });
    expect(live?.co2Now).toContain("1200");
    expect(live?.strip.text).toContain("Bedroom");
  });

  it("a unit outside a room over an unknown period: 5-minute slots of 24 hours, no history, named after its property", async () => {
    answer(detail("u-2", "Porch AC"), null);
    const live = viewProps(await open({ unitId: "u-2", period: "1y" }));
    expect(calls("telemetry.series")).toMatchObject([{ metric: "co2", bucketMinutes: 5 }]);
    expect(calls("ventilation.list")).toStrictEqual([]);
    expect(live).toMatchObject({ roomKey: "unassigned:p-1", spaceId: null, path: "Home", freshAir: false, metrics: ["co2"], period: "24h", history: null });
    expect(live?.chart).toMatchObject({ title: "CO2 — last 24 hours", gaps: false, times: [] });
    expect(live?.chart.sub).toContain("Home");
    expect(live?.tables).toMatchObject([{ metric: "co2", rows: [] }]);
  });

  it("names the screen in its loading and error states", () => {
    const error = new Error("boom");
    const retry = vi.fn();
    const loading = AirQualityLoading();
    expect(loading.type).toBe(RouteLoading);
    expect(loading.props).toStrictEqual({ what: "air quality" });
    const failed = AirQualityErrorState({ error, retry });
    expect(failed.type).toBe(RouteError);
    expect(failed.props).toStrictEqual({ what: "Air quality", error, retry });
  });
});
