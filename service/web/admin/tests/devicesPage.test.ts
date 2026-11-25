import { beforeEach, describe, expect, it, vi } from "vitest";
import { isValidElement, type ComponentProps, type ReactNode } from "react";
import type { ApiCampaign, ApiDevice, ApiIoTModel } from "@ac/web/lib/devices";

// HQ devices and models (FR-A04, FR-A20, SCR-A04): the Server Actions (models, IoT models, registration with its bind,
// node IDs, bind / unbind / delete, checks, calibrations, firmware and campaigns, each at its version) and the Server
// Component (the tab, the registry page and the selected device with its history, the firmware campaigns).
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
  const settle = async <T, F>(read: Promise<T>, fallback: F, codes: string[]): Promise<T | F> => {
    try {
      return await read;
    } catch (error) {
      if (error instanceof CoreError && codes.includes(error.error.code)) return fallback;
      throw error;
    }
  };
  return {
    CoreError, apiMode: vi.fn(() => true), coreNow: vi.fn(() => Promise.resolve(new Date("2026-09-14T01:00:00Z"))),
    coreDisplay: vi.fn(() => Promise.resolve({ locale: "en", timeZone: "Asia/Kuala_Lumpur" })),
    corePermissions: vi.fn(() => Promise.resolve(new Set(["device.read", "device.write", "audit.read"]))),
    orWhenForbidden: <T, F>(read: Promise<T>, fallback: F) => settle(read, fallback, ["FORBIDDEN"]),
    orAbsent: <T>(read: Promise<T>) => settle(read, null, ["FORBIDDEN", "NOT_FOUND"]),
    coreOp: vi.fn<(op: string, input?: unknown, options?: unknown) => Promise<unknown>>(),
    coreAll: vi.fn<(op: string, input?: unknown, limit?: number) => Promise<unknown>>(),
    coreAllPage: vi.fn<(op: string, input?: unknown) => Promise<unknown>>(),
  };
});
const cache = vi.hoisted(() => ({ refresh: vi.fn() }));
const pickers = vi.hoisted(() => ({ unitOptions: vi.fn<(...args: unknown[]) => Promise<unknown>>(), deviceOptions: vi.fn<(...args: unknown[]) => Promise<unknown>>() }));
const registry = vi.hoisted(() => ({ registryPage: vi.fn<(...args: unknown[]) => Promise<unknown>>() }));
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => cache);
vi.mock("next/server", () => ({ connection: () => Promise.resolve() }));
vi.mock("@ac/web/lib/dal", () => dal);
vi.mock("../app/admin/_lib/pickers", () => pickers);
vi.mock("../app/admin/devices/_lib/registry", () => registry);
vi.mock("../app/admin/devices/_components/devices-view", () => ({ DevicesView: () => null }));
vi.mock("../app/admin/devices/_components/devices-demo", () => ({ DevicesDemo: () => null }));

import {
  bindDevice, calibrateSensor, capabilityHistory, checkDevice, controlCampaign, deleteCampaign, deleteCapability, deleteDevice, deleteIotModel, deviceOnUnit, registerAndBind,
  saveCapability, saveIotModel, scheduleCampaign, searchDevices, searchUnits, setMqttNode, unbindDevice, updateFirmware,
} from "../app/admin/devices/actions";
import AdminDevicesPage from "../app/admin/devices/page";
import { DevicesView } from "../app/admin/devices/_components/devices-view";
import { DevicesDemo } from "../app/admin/devices/_components/devices-demo";
import DevicesLoading from "../app/admin/devices/loading";
import DevicesErrorState from "../app/admin/devices/error";
import { RouteError, RouteLoading } from "@ac/web/components/RouteStates";
import { unitDetail } from "@ac/web/lib/tests/unitFixture";

const refused = (code: string) => new dal.CoreError({ code, messageKey: `error.${code.toLowerCase()}`, fieldErrors: {}, retryAfterSeconds: null });
const lastCall = () => dal.coreOp.mock.lastCall;
const v = (n: number) => ({ write: true, expectedVersion: n });

beforeEach(() => {
  dal.apiMode.mockReturnValue(true);
  dal.coreOp.mockReset(); dal.coreAll.mockReset(); dal.coreAllPage.mockReset(); cache.refresh.mockReset();
  pickers.unitOptions.mockReset(); pickers.deviceOptions.mockReset(); registry.registryPage.mockReset();
});

describe("device Server Actions", () => {
  it("saves models and IoT models new without a version or at the version read", async () => {
    dal.coreOp.mockResolvedValue({ id: "m-1", version: 3 });
    const capability = { manufacturer: "AC-Co", model: "m1", control: true, modeControl: true, fanControl: true, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [], reason: "x" };
    expect(await saveCapability(capability)).toStrictEqual({ ok: true, value: { id: "m-1", version: 3 } });
    expect(JSON.stringify(lastCall()?.[2])).toBe('{"write":true}');
    expect(await saveCapability({ ...capability, id: "m-1" }, 2)).toMatchObject({ ok: true });
    expect(lastCall()?.[2]).toStrictEqual({ write: true, expectedVersion: 2 });
    const iot = { manufacturer: "IoT-Co", model: "n1", sensors: [], firmwareCandidates: [], reason: "x" };
    expect(await saveIotModel(iot)).toMatchObject({ ok: true });
    expect(JSON.stringify(lastCall()?.[2])).toBe('{"write":true}');
    expect(await saveIotModel({ ...iot, id: "i-1" }, 4)).toMatchObject({ ok: true });
    expect(lastCall()?.[2]).toStrictEqual({ write: true, expectedVersion: 4 });
  });

  it("lists a model's saved versions from the audit log of the last year, with the fields each one changed", async () => {
    dal.coreOp.mockResolvedValue({ items: [
      { occurredAt: "2026-09-01T00:00:00Z", actorId: "user-op", previousVersion: 1, nextVersion: 2, reason: "Wider range", maskedBefore: { min: "16", max: "30" }, maskedAfter: { min: "16", max: "32", fan: "auto" } },
      { occurredAt: "2026-08-01T00:00:00Z", actorId: "user-op", previousVersion: null, nextVersion: 1, reason: null, maskedBefore: null, maskedAfter: { min: "16" } },
      { occurredAt: "2026-07-01T00:00:00Z", actorId: "user-op", previousVersion: 1, nextVersion: null, reason: "Deleted", maskedBefore: { min: "16" }, maskedAfter: null },
      { occurredAt: "2026-06-01T00:00:00Z", actorId: "user-op", previousVersion: 1, nextVersion: 1, reason: null, maskedBefore: null, maskedAfter: null },
    ] });
    expect(await capabilityHistory("m-1")).toStrictEqual({ ok: true, value: [
      { at: "2026-09-01T00:00:00Z", actor: "user-op", from: 1, to: 2, reason: "Wider range", changes: ["max", "fan"] },
      { at: "2026-08-01T00:00:00Z", actor: "user-op", from: null, to: 1, reason: null, changes: ["min"] }, // created
      { at: "2026-07-01T00:00:00Z", actor: "user-op", from: 1, to: null, reason: "Deleted", changes: ["min"] }, // deleted
      { at: "2026-06-01T00:00:00Z", actor: "user-op", from: 1, to: 1, reason: null, changes: [] },
    ] });
    expect(lastCall()).toStrictEqual(["audit.list", { limit: 100, filters: { targetId: "m-1", from: "2025-09-14T01:00:00.000Z", to: "2026-09-15T01:00:00.000Z" } }]);
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("registers a device and binds it to the chosen unit, replacing the unit's device, or keeps it unbound when the bind is refused", async () => {
    dal.coreOp.mockResolvedValueOnce({ id: "d-new", version: 1 }).mockResolvedValueOnce({});
    expect(await registerAndBind({ serial: "AC-1", sensorTypes: ["temperature"], mqttNode: "node_9", iotModelId: "i-1", unitId: "u-1", replaceDeviceId: "d-old", reason: "Swap" })).toStrictEqual({ ok: true, value: { id: "d-new", bindError: null } });
    expect(dal.coreOp.mock.calls).toStrictEqual([
      ["devices.register", { serial: "AC-1", iotModelId: "i-1", sensorTypes: ["temperature"], unitId: "u-1", mqttNode: "node_9" }, { write: true }],
      ["devices.bind", { deviceId: "d-new", unitId: "u-1", reason: "Swap", replaceDeviceId: "d-old" }, { write: true, expectedVersion: 1 }],
    ]);
    dal.coreOp.mockReset();
    dal.coreOp.mockResolvedValueOnce({ id: "d-2", version: 1 }).mockRejectedValueOnce(refused("CONFLICT"));
    expect(await registerAndBind({ serial: "AC-2", sensorTypes: null, mqttNode: null, iotModelId: "i-1", unitId: "u-1" })).toStrictEqual({ ok: true, value: { id: "d-2", bindError: "error.conflict" } });
    expect(dal.coreOp.mock.calls[1]?.[1]).toStrictEqual({ deviceId: "d-2", unitId: "u-1", reason: "Registered with the unit" });
    dal.coreOp.mockReset();
    dal.coreOp.mockResolvedValueOnce({ id: "d-3", version: 1 });
    expect(await registerAndBind({ serial: "AC-3", sensorTypes: null, mqttNode: null, iotModelId: "i-1", unitId: null })).toStrictEqual({ ok: true, value: { id: "d-3", bindError: null } });
    expect(dal.coreOp).toHaveBeenCalledOnce(); // no unit, no bind
    dal.coreOp.mockReset();
    dal.coreOp.mockResolvedValueOnce({ id: "d-4", version: 1 }).mockRejectedValueOnce(new TypeError("Failed to fetch"));
    expect(await registerAndBind({ serial: "AC-4", sensorTypes: null, mqttNode: null, iotModelId: "i-1", unitId: "u-1" })).toMatchObject({ ok: false, code: "UNAVAILABLE" });
  });

  it("searches units and devices for the pickers, and finds the device on a unit, without re-rendering", async () => {
    pickers.unitOptions.mockResolvedValue([{ id: "u-1" }]);
    pickers.deviceOptions.mockResolvedValue([{ id: "d-1" }]);
    expect(await searchUnits("lob", true)).toStrictEqual({ ok: true, value: [{ id: "u-1" }] });
    expect(pickers.unitOptions).toHaveBeenCalledWith("lob", true);
    expect(await searchDevices("AC", false, true)).toStrictEqual({ ok: true, value: [{ id: "d-1" }] });
    expect(pickers.deviceOptions).toHaveBeenCalledWith("AC", false, true);
    dal.coreOp.mockResolvedValueOnce({ items: [{ id: "d-1", serial: "AC-1" }] }).mockResolvedValueOnce({ items: [] });
    expect(await deviceOnUnit("u-1")).toStrictEqual({ ok: true, value: { id: "d-1", serial: "AC-1" } });
    expect(lastCall()).toStrictEqual(["devices.list", { limit: 1, filters: { unitId: "u-1" } }]);
    expect(await deviceOnUnit("u-2")).toStrictEqual({ ok: true, value: null });
    expect(cache.refresh).not.toHaveBeenCalled();
  });

  it("sends each device and campaign write at its version", async () => {
    dal.coreOp.mockResolvedValue({ id: "c-new" });
    const cases: [() => Promise<unknown>, string, unknown, unknown][] = [
      [() => setMqttNode("d-1", 2, null), "devices.setMqttNode", { deviceId: "d-1", mqttNode: null }, v(2)],
      [() => bindDevice("d-1", 2, "u-1", "Install", "d-old"), "devices.bind", { deviceId: "d-1", unitId: "u-1", reason: "Install", replaceDeviceId: "d-old" }, v(2)],
      [() => bindDevice("d-1", 2, "u-1", "Install"), "devices.bind", { deviceId: "d-1", unitId: "u-1", reason: "Install" }, v(2)],
      [() => unbindDevice("d-1", 3, "Removed"), "devices.unbind", { deviceId: "d-1", reason: "Removed" }, v(3)],
      [() => deleteDevice("d-1", 3, "Scrapped"), "devices.delete", { deviceId: "d-1", reason: "Scrapped" }, v(3)],
      [() => deleteCapability("m-1", 1, "Unused"), "capabilities.delete", { id: "m-1", reason: "Unused" }, v(1)],
      [() => deleteIotModel("i-1", 1, "Unused"), "iotModels.delete", { id: "i-1", reason: "Unused" }, v(1)],
      [() => checkDevice("d-1", 4), "devices.check", { id: "d-1" }, v(4)],
      [() => calibrateSensor("d-1", 4, "s-1", "temperature", "°C", 24, 24.6), "devices.calibrate", { deviceId: "d-1", sensorId: "s-1", metric: "temperature", unit: "°C", referenceValue: 24, measuredValue: 24.6, calibratedAt: "2026-09-14T01:00:00.000Z" }, v(4)],
      [() => updateFirmware("d-1", 4, "1.2.0"), "devices.updateFirmware", { deviceId: "d-1", firmwareVersion: "1.2.0" }, v(4)],
      [() => deleteCampaign("c-1", 5, "Done"), "firmwareCampaigns.delete", { id: "c-1", reason: "Done" }, v(5)],
      [() => controlCampaign("c-1", 5, "pause"), "firmwareCampaigns.control", { campaignId: "c-1", action: "pause" }, v(5)],
      [() => controlCampaign("c-1", 5, "retry_device", "d-1", "Network back"), "firmwareCampaigns.control", { campaignId: "c-1", action: "retry_device", deviceId: "d-1", reason: "Network back" }, v(5)],
    ];
    for (const [call, op, body, options] of cases) {
      expect(await call()).toStrictEqual({ ok: true, value: null });
      expect(lastCall()).toStrictEqual([op, body, options]);
    }
    expect(await scheduleCampaign({ modelId: "i-1", targetVersion: "1.2.0", deviceIds: ["d-1"], waves: "10%, 100", startLocal: "01:00", endLocal: "05:00", autoPause: "20", startAt: "" }, "2026-09-15T17:00:00Z")).toStrictEqual({ ok: true, value: "c-new" });
    expect(lastCall()).toStrictEqual(["firmwareCampaigns.schedule", { modelId: "i-1", targetVersion: "1.2.0", deviceIds: ["d-1"], waves: [{ label: "Wave 1 · 10 %", percent: 10 }, { label: "Wave 2 · 100 %", percent: 100 }], window: { startLocal: "01:00", endLocal: "05:00" }, autoPauseFailurePercent: 20, startAt: "2026-09-15T17:00:00Z" }, { write: true }]);
    await scheduleCampaign({ modelId: "i-1", targetVersion: "1.2.0", deviceIds: ["d-1"], waves: "x", startLocal: "01:00", endLocal: "05:00", autoPause: "20", startAt: "" }, "t");
    expect(lastCall()?.[1]).toMatchObject({ waves: [] }); // unparsable waves are sent as none for the API to refuse
    dal.coreOp.mockRejectedValueOnce(refused("CONFLICT"));
    expect(await unbindDevice("d-1", 3, "x")).toMatchObject({ ok: false, code: "CONFLICT" });
  });
});

const device = (id: string, over: Partial<ApiDevice> = {}): ApiDevice => ({
  id, version: 2, unitId: "u-1", serial: `AC-${id}`, connection: "online", lastSeenAt: "2026-09-14T00:59:00Z", firmwareVersion: "1.0.0", iotModelId: "i-1", mqttNode: null,
  powerSignal: "on", tamper: "clear", sensors: [{ id: "s-1", metric: "temperature", unit: "°C", staleAfterSeconds: 300, calibratedAt: null }], ...over,
});
const iotModel: ApiIoTModel = { id: "i-1", version: 1, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "IoT-Co", model: "n1", sensors: [], firmwareCandidates: ["1.1.0", "1.2.0"] };
const campaign = (id: string): ApiCampaign => ({
  id, version: 1, modelId: "i-1", fromVersions: ["1.0.0"], targetVersion: "1.2.0", deviceIds: ["d-1"], waves: [{ label: "Wave 1", percent: 100 }], window: { startLocal: "01:00", endLocal: "05:00" },
  autoPauseFailurePercent: 20, startAt: "2026-09-15T17:00:00Z", state: "scheduled", checksum: "sha256:x", createdAt: "2026-09-14T00:00:00Z", createdByMembershipId: "m-1",
  progress: { succeeded: 0, installing: 0, pending: 1, failed: 0, skipped: 0 }, results: [], reason: null,
});
function core(ops: Record<string, (input: unknown) => unknown>) {
  const op = new Map(Object.entries(ops));
  dal.coreOp.mockImplementation((name, input) => Promise.try(() => (op.get(name) ?? (() => ({ items: [], total: 0 })))(input)));
  const pages = new Map<string, unknown[]>([
    ["capabilities.list", [{ id: "m-1", version: 1, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "AC-Co", model: "m1", control: true, modeControl: true, fanControl: true, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [] }]],
    ["iotModels.list", [iotModel]],
    ["units.list", [{ id: "u-1", displayName: "Lobby AC", modelId: "m-1", iotModelId: "i-1", customerOrgId: "org-a", archived: false }, { id: "u-old", displayName: "Old", modelId: "m-1", customerOrgId: "org-a", archived: true }, { id: "u-2", displayName: "Hall AC", modelId: "m-1", customerOrgId: "org-a", archived: false }]],
  ]);
  dal.coreAllPage.mockImplementation((name) => Promise.resolve({ items: pages.get(name) ?? [], total: 0 }));
  dal.coreAll.mockImplementation((name) => Promise.resolve(name === "devices.list" ? [device("d-1"), device("d-2", { unitId: null })] : [{ id: "m-1", displayName: "HQ Ops" }]));
}
const props = (sp: Record<string, string> = {}) => ({ searchParams: Promise.resolve(sp), params: Promise.resolve({}) });
type Live = ComponentProps<typeof DevicesView>["live"];
async function liveOf(sp: Record<string, string> = {}): Promise<Live> {
  const page: ReactNode = await AdminDevicesPage(props(sp));
  if (!isValidElement<ComponentProps<typeof DevicesView>>(page) || page.type !== DevicesView) throw new TypeError("not DevicesView");
  return page.props.live;
}
const callsOf = (op: string) => dal.coreOp.mock.calls.filter(([name]) => name === op).map(([, input]) => input);

describe("the devices page (Server Component)", () => {
  it("shows the demo outside API mode, and its loading and error states", async () => {
    dal.apiMode.mockReturnValue(false);
    const page: ReactNode = await AdminDevicesPage(props());
    expect(isValidElement(page) && page.type).toBe(DevicesDemo);
    expect(DevicesLoading().type).toBe(RouteLoading);
    const error = new Error("down"), retry = vi.fn();
    const failed = DevicesErrorState({ error, retry });
    expect(failed.type === RouteError && failed.props).toMatchObject({ error, retry });
  });

  it("opens the models tab with every device counted and the campaign total, and IoT models on their tab", async () => {
    core({ "firmwareCampaigns.list": () => ({ items: [], total: 3 }) });
    const live = await liveOf();
    expect([live.tab, live.deviceTotal, live.campaignTotal, live.canWrite, live.canAudit, live.page, live.device]).toStrictEqual(["models", 2, 3, true, true, undefined, undefined]);
    expect(dal.coreAll).toHaveBeenCalledWith("devices.list", {}, 10_000);
    expect(live.units).toStrictEqual([{ id: "u-1", label: "Lobby AC", modelId: "m-1", iotModelId: "i-1" }, { id: "u-2", label: "Hall AC", modelId: "m-1", iotModelId: null }]); // archived units left out
    const iot = await liveOf({ tab: "iot" });
    expect(iot.tab).toBe("iot");
    dal.coreOp.mockImplementation(() => Promise.reject(refused("FORBIDDEN")));
    const noCampaigns = await liveOf({ tab: "bogus" });
    expect([noCampaigns.tab, noCampaigns.campaignTotal]).toStrictEqual(["models", null]);
  });

  it("pages the registry on the devices tab and opens the URL's device with its unit, history and events", async () => {
    registry.registryPage.mockResolvedValue({ items: [device("d-1"), device("d-2")], total: 42, next: "cur-2" });
    core({
      "devices.get": () => ({ ...device("d-2"), calibrationRefs: [], activeOperation: null }),
      "devices.operations": () => ({ items: [{ id: "op-1", kind: "firmware", status: "succeeded", targetVersion: "1.0.0", failureCode: null, createdAt: "2026-09-10T00:00:00Z", finishedAt: "2026-09-10T00:10:00Z" }], total: 1 }),
      "devices.calibrations": () => ({ items: [{ id: "c-1", sensorId: "s-1", metric: "temperature", unit: "°C", referenceValue: 24, measuredValue: 24.4, calibratedAt: "2026-09-12T00:00:00Z", actorId: "tech-1" }], total: 1 }),
      "devices.events": () => ({ items: [{ id: "e-1", version: 1, deviceId: "d-2", eventType: "tamper", evidenceSource: "tamper_signal", occurredAt: "2026-09-13T00:00:00Z", restoredAt: null, alertIds: ["al-1", "al-gone"], recovery: null, responseNotes: [] }], total: 1 }),
      "units.get": () => unitDetail({ id: "u-1", displayName: "Lobby AC", location: { pathLabels: ["Home A", "1F"], address: null, accessInstructions: null }, capabilityVersion: 3 }),
      "alerts.get": (q) => (JSON.stringify(q).includes("al-gone") ? Promise.reject(refused("NOT_FOUND")) : { id: "al-1", version: 1, status: "open", type: "tamper", severity: "warning" }),
    });
    const live = await liveOf({ tab: "devices", q: " AC ", cursor: "cur-1", deviceId: "d-2" });
    expect(registry.registryPage).toHaveBeenCalledWith("AC", "cur-1", "d-2");
    expect([live.deviceTotal, live.page, live.rows?.map((r) => r.id), live.device?.row.id]).toStrictEqual([42, { q: "AC", next: "cur-2", cursor: "cur-1" }, ["d-1", "d-2"], "d-2"]);
    expect(live.device?.unit).toStrictEqual({ name: "Lobby AC", place: "Home A › 1F", model: "AC-Co m1 v3" });
    expect(live.device?.firmware).toStrictEqual(["1.1.0", "1.2.0"]);
    expect(live.device?.events[0]?.alerts.map((a) => a.id)).toStrictEqual(["al-1"]); // an alert HQ cannot read is left out
    expect(callsOf("devices.operations")[0]).toStrictEqual({ deviceId: "d-2", query: { limit: 20, sort: { field: "createdAt", direction: "desc" } } });
  });

  it("opens an unbound device without reading a unit, and nothing without devices", async () => {
    registry.registryPage.mockResolvedValue({ items: [device("d-9", { unitId: null, iotModelId: "i-unknown" })], total: 1, next: null });
    core({ "devices.get": () => ({ ...device("d-9", { unitId: null, iotModelId: "i-unknown" }), calibrationRefs: [], activeOperation: null }) });
    const live = await liveOf({ tab: "devices" });
    expect([live.device?.unit, live.device?.firmware, live.page]).toStrictEqual([null, [], { q: "", next: null, cursor: null }]);
    expect(callsOf("units.get")).toStrictEqual([]);
    registry.registryPage.mockResolvedValue({ items: [], total: 0, next: null });
    const empty = await liveOf({ tab: "devices" });
    expect([empty.rows, empty.device]).toStrictEqual([[], undefined]);
  });

  it("lists the campaigns on the firmware tab with who scheduled them, and opens the URL's one", async () => {
    core({ "firmwareCampaigns.list": () => ({ items: [campaign("c-1"), campaign("c-2")], total: 2 }), "firmwareCampaigns.get": () => campaign("c-2") });
    const live = await liveOf({ tab: "firmware", campaignId: "c-2" });
    expect([live.campaigns?.map((c) => c.id), live.campaign?.id, live.campaignTotal]).toStrictEqual([["c-1", "c-2"], "c-2", null]);
    expect(callsOf("firmwareCampaigns.get")).toStrictEqual([{ id: "c-2" }]);
    core({ "firmwareCampaigns.list": () => ({ items: [], total: 0 }) });
    dal.coreAll.mockImplementation((name) => (name === "members.list" ? Promise.reject(refused("FORBIDDEN")) : Promise.resolve([])));
    const none = await liveOf({ tab: "firmware" });
    expect([none.campaigns, none.campaign]).toStrictEqual([[], undefined]);
  });
});
