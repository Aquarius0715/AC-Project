import { beforeEach, describe, expect, it, vi } from "vitest";

// the technician device screen (FR-T11) through the DAL; a device registered on site is reserved for its planned unit
// and stays unbound until devices.bind (D05, IR467)
// CoreError and orAbsent copy the DAL's: only FORBIDDEN and NOT_FOUND read as absent, anything else is thrown
const dal = vi.hoisted(() => {
  class CoreError extends Error {
    readonly error: { code: string };
    constructor(code: string, options?: ErrorOptions) {
      super(code, options);
      this.name = "CoreError";
      this.error = { code };
    }
  }
  return {
    CoreError, coreNow: vi.fn(), coreDisplay: vi.fn(), corePermissions: vi.fn(), coreAll: vi.fn(), coreOp: vi.fn(),
    orAbsent: async <T,>(read: Promise<T>): Promise<T | null> => read.catch((error: unknown) => {
      if (error instanceof CoreError && (error.error.code === "FORBIDDEN" || error.error.code === "NOT_FOUND")) return null;
      throw error;
    }),
  };
});
vi.mock("server-only", () => ({}));
vi.mock("@ac/web/lib/dal", () => dal);

import { loadDevices } from "../app/technician/devices/_lib/load";
import type { ApiCalibration, ApiDeviceDetail } from "@ac/web/lib/devices";
import type { ApiUnitDetail } from "@ac/web/lib/units";
import type { ApiAlertLite, ApiDeviceEventFull } from "@ac/web/lib/techDevices";

const planned: ApiDeviceDetail = {
  id: "device-1", version: 1, unitId: null, targetUnitId: "unit-1", serial: "AC-NEW-1", connection: "unknown", lastSeenAt: null, firmwareVersion: "1.0.0",
  mqttNode: null, powerSignal: "unknown", tamper: "clear", sensors: [], calibrationRefs: [], activeOperation: null,
};

const unitDetail = (id: string, name: string, iotModel: ApiUnitDetail["iotModel"]): ApiUnitDetail => ({
  id, iotModel, installState: "installed", modelId: "model-1", capabilityVersion: 3, serviceScope: ["indoor"], components: [], version: 1, spaceId: null, displayName: name,
  alertPolicyIds: [], connection: "online", effectivePowerState: "on", observedState: { power: true, celsius: 24, mode: null, fanLevel: null, observedAt: null },
  lastSeenAt: null, latestMeasurements: [],
  capabilities: { manufacturer: "Daikin", model: "FTKM", control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [] },
  effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: ["Site A", "Floor 2"], address: null, accessInstructions: null }, installedAt: null,
});

interface Setup {
  devices?: ApiDeviceDetail[];
  units?: { id: string; displayName: string; archived: boolean }[];
  jobs?: { projection: string; id?: string; unitId?: string; status?: string }[];
  perms?: string[];
  details?: Map<string, ApiUnitDetail>;
  calibrations?: ApiCalibration[];
  events?: ApiDeviceEventFull[] | "refused";
  alerts?: ApiAlertLite[];
  unitError?: string;
}

const alertReads: string[] = [];

function setup(s: Setup): void {
  dal.coreNow.mockResolvedValue(new Date("2026-11-10T02:00:00Z"));
  dal.coreDisplay.mockResolvedValue({ locale: "en", timeZone: "Asia/Kuala_Lumpur" });
  dal.corePermissions.mockResolvedValue(new Set(s.perms ?? ["device.maintain", "alert.read"]));
  const devices = s.devices ?? [planned];
  const lists = new Map<string, unknown[]>([
    ["devices.list", devices],
    ["units.list", s.units ?? [{ id: "unit-1", displayName: "Bedroom AC", archived: false }]],
    ["jobs.list", s.jobs ?? [{ projection: "summary", id: "job-1", unitId: "unit-1", status: "assigned" }]],
  ]);
  dal.coreAll.mockImplementation((operation: string) => Promise.resolve(lists.get(operation) ?? []));
  const alerts = new Map((s.alerts ?? []).map((a) => [a.id, a]));
  dal.coreOp.mockImplementation((operation: string, input: { id?: string }) => {
    if (operation === "devices.get") return Promise.resolve(devices.find((d) => d.id === input.id));
    if (operation === "units.get") {
      const u = input.id ? s.details?.get(input.id) : undefined;
      return u ? Promise.resolve(u) : Promise.reject(new dal.CoreError(s.unitError ?? "FORBIDDEN")); // before the work window: absent
    }
    if (operation === "devices.calibrations") return Promise.resolve({ items: s.calibrations ?? [] });
    if (operation === "devices.events") return s.events === "refused" ? Promise.reject(new dal.CoreError("FORBIDDEN")) : Promise.resolve({ items: s.events ?? [] });
    if (operation === "alerts.get") {
      alertReads.push(input.id ?? "");
      const a = input.id ? alerts.get(input.id) : undefined;
      return a ? Promise.resolve(a) : Promise.reject(new dal.CoreError("NOT_FOUND"));
    }
    return Promise.resolve({ items: [] });
  });
}

const called = (operation: string) => dal.coreOp.mock.calls.filter((c) => c[0] === operation);

describe("technician devices", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    alertReads.length = 0;
  });

  it("runs a device not bound yet under the job of its planned unit, so it can be bound (IR722)", async () => {
    setup({});
    const live = await loadDevices({ selected: "device-1", events: false });
    expect(live.device?.jobId).toBe("job-1");
    expect(live.device?.unit).toBeNull();
  });

  it("does not offer the planned unit of a registered, unbound device for another registration", async () => {
    setup({
      units: [{ id: "unit-1", displayName: "Bedroom AC", archived: false }, { id: "unit-2", displayName: "Living AC", archived: false }],
      jobs: [{ projection: "summary", id: "job-1", unitId: "unit-1", status: "assigned" }, { projection: "summary", id: "job-2", unitId: "unit-2", status: "assigned" }],
      details: new Map([["unit-1", unitDetail("unit-1", "Bedroom AC", null)], ["unit-2", unitDetail("unit-2", "Living AC", null)]]),
    });
    const live = await loadDevices({ events: false });
    expect(live.register.map((r) => r.unitId)).toStrictEqual(["unit-2"]);
  });

  it("lists only the jobs a device write can run under, in progress first, in the register form", async () => {
    setup({
      devices: [],
      units: [{ id: "unit-2", displayName: "Living AC", archived: false }],
      jobs: [
        { projection: "summary", id: "job-rwk00000", unitId: "unit-2", status: "rework_requested" },
        { projection: "summary", id: "job-done0000", unitId: "unit-2", status: "completed" },
        { projection: "summary", id: "job-asg00000", unitId: "unit-2", status: "assigned" },
        { projection: "summary", id: "job-run00000", unitId: "unit-2", status: "in_progress" },
        { projection: "summary", id: "job-cxl00000", unitId: "unit-2", status: "cancelled" },
      ],
      details: new Map([["unit-2", unitDetail("unit-2", "Living AC", { id: "m1", manufacturer: "Lab", model: "N1", sensors: [{ metric: "temperature", unit: "°C", staleAfterSeconds: 60 }], firmwareCandidates: [] })]]),
    });
    const live = await loadDevices({ events: false });
    expect(live.register).toStrictEqual([{
      unitId: "unit-2", label: "Living AC · Site A › Floor 2", model: "Daikin FTKM", sensors: ["temperature"],
      jobs: [{ id: "job-run00000", label: "job-run0 · in progress" }, { id: "job-asg00000", label: "job-asg0 · assigned" }, { id: "job-rwk00000", label: "job-rwk0 · rework requested" }],
    }]);
    expect(live.device).toBeNull();
  });

  it("leaves out archived units, units without an active job and units whose detail is not readable", async () => {
    setup({
      devices: [],
      units: [{ id: "unit-a", displayName: "Old", archived: true }, { id: "unit-b", displayName: "Done", archived: false }, { id: "unit-c", displayName: "Hidden", archived: false }],
      jobs: [
        { projection: "summary", id: "job-a", unitId: "unit-a", status: "assigned" },
        { projection: "summary", id: "job-b", unitId: "unit-b", status: "completed" },
        { projection: "summary", id: "job-c", unitId: "unit-c", status: "assigned" },
        { projection: "lookup", id: "job-x", unitId: "unit-c", status: "assigned" },
        { projection: "summary", unitId: "unit-c", status: "assigned" },
      ],
    });
    const live = await loadDevices({ events: false });
    expect(live.register).toStrictEqual([]);
    expect(live.rebind).toStrictEqual([{ unitId: "unit-c", label: "Hidden", jobId: "job-c" }]);
  });

  it("reads operations and calibrations with the maintain permission, and offers newer firmware of the device's model", async () => {
    const bound: ApiDeviceDetail = { ...planned, unitId: "unit-1", targetUnitId: null, iotModelId: "m1" };
    setup({
      devices: [bound],
      details: new Map([["unit-1", unitDetail("unit-1", "Bedroom AC", { id: "m1", manufacturer: "Lab", model: "N1", sensors: [], firmwareCandidates: ["1.0.0", "1.2.0"] })]]),
      calibrations: [{ id: "c1", sensorId: "s1", metric: "temperature", unit: "°C", referenceValue: 24, measuredValue: 24.5, calibratedAt: "2026-11-09T02:00:00Z", actorId: "a1" }],
    });
    const live = await loadDevices({ events: false });
    expect(live.device?.unit).toStrictEqual({ name: "Bedroom AC", place: "Site A › Floor 2", model: "Daikin FTKM v3" });
    expect(live.device?.firmware).toStrictEqual(["1.2.0"]);
    expect(live.device?.calibrations).toHaveLength(1);
    expect(live.device?.events).toBeNull();
    expect(live.device?.eventsLocked).toBe(false);
    expect(called("devices.operations")).toHaveLength(1);
    expect(called("devices.events")).toHaveLength(0);
  });

  it("offers no firmware when the unit's model is not the one the device was registered with", async () => {
    const bound: ApiDeviceDetail = { ...planned, unitId: "unit-1", targetUnitId: null, iotModelId: "m-old" };
    setup({ devices: [bound], details: new Map([["unit-1", unitDetail("unit-1", "Bedroom AC", { id: "m1", manufacturer: "Lab", model: "N1", sensors: [], firmwareCandidates: ["1.2.0"] })]]) });
    const live = await loadDevices({ events: false });
    expect(live.device?.firmware).toStrictEqual([]);
  });

  it("reads no operations without the maintain permission or without a job on the unit", async () => {
    setup({ perms: ["alert.read"] });
    const live = await loadDevices({ events: false });
    expect(live.canMaintain).toBe(false);
    expect(live.canAcknowledge).toBe(true);
    expect(called("devices.operations")).toHaveLength(0);
    vi.clearAllMocks();
    setup({ jobs: [] });
    const other = await loadDevices({ events: false });
    expect(other.device?.jobId).toBeNull();
    expect(called("devices.calibrations")).toHaveLength(0);
  });

  it("keeps the job of the URL only when it is the technician's job on the device's unit", async () => {
    const jobs = [
      { projection: "summary", id: "job-1", unitId: "unit-1", status: "in_progress" },
      { projection: "summary", id: "job-2", unitId: "unit-1", status: "assigned" },
      { projection: "summary", id: "job-9", unitId: "unit-9", status: "assigned" },
    ];
    setup({ jobs });
    const kept = await loadDevices({ events: false, jobId: "job-2" });
    expect(kept.device?.jobId).toBe("job-2");
    const elsewhere = await loadDevices({ events: false, jobId: "job-9" });
    expect(elsewhere.device?.jobId).toBe("job-1");
  });

  it("picks the device by id, else by the unit of the URL, else the first row", async () => {
    const other: ApiDeviceDetail = { ...planned, id: "device-2", serial: "AC-NEW-2", targetUnitId: "unit-2" };
    setup({ devices: [planned, other] });
    const byId = await loadDevices({ selected: "device-2", events: false });
    expect(byId.device?.row.id).toBe("device-2");
    const byUnit = await loadDevices({ selected: "nope", unitId: "unit-2", events: false });
    expect(byUnit.device?.row.id).toBe("device-2");
    const first = await loadDevices({ selected: "nope", events: false });
    expect(first.device?.row.id).toBe(first.rows[0]?.id);
    expect(first.rows).toHaveLength(2);
  });

  it("reads the events with their alerts on the events screen, once per alert, and skips alerts that are gone", async () => {
    const event: ApiDeviceEventFull = {
      id: "e1", version: 1, deviceId: "device-1", eventType: "communication_lost", evidenceSource: "heartbeat", occurredAt: "2026-11-10T01:00:00Z",
      restoredAt: null, alertIds: ["al-1", "al-gone"], recovery: null, responseNotes: [],
    };
    setup({ events: [event, { ...event, id: "e2", alertIds: ["al-1"] }], alerts: [{ id: "al-1", version: 1, status: "open", type: "communication_lost", severity: "high" }] });
    const live = await loadDevices({ selected: "device-1", events: true });
    expect(alertReads).toStrictEqual(["al-1", "al-gone"]);
    expect(live.device?.events).toHaveLength(2);
    expect(live.device?.eventsLocked).toBe(false);
    expect(called("devices.operations")).toHaveLength(0);
  });

  it("reads no unit and runs under no job for a device with neither a bound nor a planned unit", async () => {
    const loose: ApiDeviceDetail = { ...planned, targetUnitId: null };
    setup({ devices: [loose], jobs: [{ projection: "summary", id: "job-1", unitId: "unit-1" }] });
    const live = await loadDevices({ events: false, jobId: "job-1" });
    expect(live.device?.unit).toBeNull();
    expect(live.device?.jobId).toBeNull();
    expect(called("units.get")).toHaveLength(0);
    expect(live.rebind).toStrictEqual([]); // a job without a status is not one a device write runs under
  });

  it("fails the page when a unit read fails for another reason than absence", async () => {
    setup({ unitError: "UNAVAILABLE" });
    await expect(loadDevices({ events: false })).rejects.toThrow("UNAVAILABLE");
  });

  it("marks the events as locked when the events read is refused (no assignment on the unit)", async () => {
    setup({ events: "refused" });
    const live = await loadDevices({ selected: "device-1", events: true });
    expect(live.device?.events).toBeNull();
    expect(live.device?.eventsLocked).toBe(true);
  });
});
