import { describe, expect, it } from "vitest";
import {
  campaignErrors, capabilityDraft, capabilityErrors, capabilityInput, deviceRows, iotModelDraft, iotModelErrors, iotModelInput, parseWaves, waveStates,
  type ApiCampaign, type ApiCapability, type ApiDevice, type CampaignDraft, type CapabilityDraft, type IoTModelDraft,
} from "@ac/web/lib/devices";

// The HQ devices & models forms (DD-A04, DD-A20, DEC-105): the aircon model's draft, its every check and the input it
// sends; the IoT model's checks and input; the device list's unit words; the waves of a firmware campaign, their
// states, and the campaign's checks.

const saved: ApiCapability = {
  id: "cap-1", version: 4, updatedAt: "2026-09-01T00:00:00Z", manufacturer: "Daikin", model: "FTKF", control: true, modeControl: true, fanControl: true,
  temperature: { min: 18, max: 30, step: 0.5 }, modes: ["cool", "dry"], fanLevels: ["low", "high"], ventilation: true, ventilationLevels: ["low", "mid"],
};
const capability = (over: Partial<CapabilityDraft> = {}): CapabilityDraft => ({ ...capabilityDraft(saved), ...over });
const iot = (over: Partial<IoTModelDraft> = {}): IoTModelDraft => ({ manufacturer: "Espressif", model: "ESP32 kit", sensors: [{ metric: "temperature", staleAfterSeconds: "60" }], firmware: ["0.4.0"], changeReason: "", ...over });

const device = (id: string, over: Partial<ApiDevice>): ApiDevice => ({
  id, version: 1, unitId: null, serial: `SN-${id}`, connection: "online", lastSeenAt: null, firmwareVersion: "0.4.0", powerSignal: "on", tamper: "clear", sensors: [], ...over,
});
const result = (waveIndex: number, status: ApiCampaign["results"][number]["status"]) => ({ waveIndex, status, deviceId: `d-${String(waveIndex)}-${status}`, reasonKey: null, at: null });

describe("the aircon model form", () => {
  it("starts empty with a 16–30 °C range in steps of 1, or from the saved model", () => {
    expect(capabilityDraft()).toStrictEqual({
      manufacturer: "", model: "", control: false, modeControl: false, fanControl: false, temperatureOn: false, min: "16", max: "30", step: "1",
      modes: [], fanLevels: [], ventilation: false, ventilationLevels: [], changeReason: "",
    });
    expect(capability()).toMatchObject({ temperatureOn: true, min: "18", max: "30", step: "0.5", modes: ["cool", "dry"], ventilationLevels: ["low", "mid"] });
    expect(capabilityDraft({ ...saved, temperature: null }).temperatureOn).toBe(false);
  });

  it("checks the names, that each control needs power control and its values, and the change reason of an update", () => {
    expect(capabilityErrors(capability(), false)).toStrictEqual({});
    expect(capabilityErrors(capability({ manufacturer: " ", model: "x".repeat(121), control: false, modes: [], fanLevels: [], ventilationLevels: ["mid"] }), true)).toStrictEqual({
      manufacturer: "1–120 characters", model: "1–120 characters", control: "Mode, fan or temperature control needs power control", modes: "Choose at least one mode",
      fanLevels: "Choose at least one fan level", ventilationLevels: "Ventilation levels must include Low", changeReason: "A reason is required (1–1000 characters)",
    });
    expect(capabilityErrors(capability({ ventilationLevels: [] }), true)).toMatchObject({ ventilationLevels: "Ventilation levels must include Low" });
    expect(capabilityErrors(capability({ changeReason: "Firmware 2 adds fan" }), true)).toStrictEqual({});
    expect(capabilityErrors(capability({ modeControl: false, fanControl: false, temperatureOn: false, control: false, ventilation: false, modes: [], fanLevels: [] }), false)).toStrictEqual({}); // a monitor-only model
  });

  it("checks the temperature range: numbers, min below max, a step that fits", () => {
    const range = (min: string, max: string, step: string) => capabilityErrors(capability({ min, max, step }), false).temperature;
    expect([range("", "30", "1"), range("30", "30", "1"), range("a", "30", "1"), range("18", "30", "0"), range("18", "30", "13"), range("18", "25", "7")]).toStrictEqual([
      "min must be below max", "min must be below max", "min must be below max", "0 < step ≤ max − min", "0 < step ≤ max − min", undefined,
    ]);
    expect(capabilityErrors(capability({ temperatureOn: false, min: "x" }), false)).toStrictEqual({}); // an unused range is not checked
  });

  it("sends a new model without an id, an update with its reason, and only the values of the controls it has", () => {
    expect(capabilityInput(capability({ manufacturer: " Daikin ", changeReason: " new fan " }), "cap-1")).toStrictEqual({
      id: "cap-1", changeReason: "new fan", manufacturer: "Daikin", model: "FTKF", control: true, modeControl: true, fanControl: true,
      temperature: { min: 18, max: 30, step: 0.5 }, modes: ["cool", "dry"], fanLevels: ["low", "high"], ventilation: true, ventilationLevels: ["low", "mid"],
    });
    expect(capabilityInput(capability({ modeControl: false, fanControl: false, temperatureOn: false, ventilation: false }))).toStrictEqual({
      manufacturer: "Daikin", model: "FTKF", control: true, modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [], ventilation: false, ventilationLevels: [],
    });
  });
});

describe("the IoT model form", () => {
  it("starts empty or from the saved model", () => {
    expect(iotModelDraft()).toStrictEqual({ manufacturer: "", model: "", changeReason: "", sensors: [], firmware: [] });
  });

  it("checks one sensor per metric, its stale time, the firmware lengths and the reason of an update", () => {
    expect(iotModelErrors(iot(), false)).toStrictEqual({});
    expect(iotModelErrors(iot({ sensors: [{ metric: "humidity", staleAfterSeconds: "60" }, { metric: "humidity", staleAfterSeconds: "60" }] }), false)).toStrictEqual({ sensors: "One sensor per metric (humidity twice)" });
    expect(["9", "86401", "", "60.5"].map((s) => iotModelErrors(iot({ sensors: [{ metric: "co2", staleAfterSeconds: s }] }), false).sensors)).toStrictEqual(Array.from({ length: 4 }, () => "Stale after must be 10–86400 seconds"));
    expect(iotModelErrors(iot({ sensors: [{ metric: "co2", staleAfterSeconds: "86400" }] }), false)).toStrictEqual({});
    expect(iotModelErrors(iot({ manufacturer: "", firmware: ["v".repeat(33)] }), true)).toStrictEqual({
      manufacturer: "1–120 characters", firmware: "Firmware versions are at most 32 characters", changeReason: "A reason is required (1–1000 characters)",
    });
  });

  it("sends each sensor's unit, the power meter's boundary, and the firmware trimmed without blanks or repeats", () => {
    expect(iotModelInput(iot({ sensors: [{ metric: "power", staleAfterSeconds: "30" }, { metric: "humidity", staleAfterSeconds: "120" }], firmware: [" 0.4.0 ", "", "0.4.0", "0.5.0"], changeReason: " fix " }), "iot-1")).toStrictEqual({
      id: "iot-1", changeReason: "fix", manufacturer: "Espressif", model: "ESP32 kit",
      sensors: [{ metric: "power", unit: "kW", staleAfterSeconds: 30, boundaryId: "ac_input_electricity" }, { metric: "humidity", unit: "%", staleAfterSeconds: 120, boundaryId: null }],
      firmwareCandidates: ["0.4.0", "0.5.0"],
    });
  });
});

describe("the device list", () => {

  it("names the bound unit, the unit an unbound device is planned for, or says it is unbound", () => {
    const units = [{ id: "u-1", displayName: "Bedroom AC", modelId: "cap-1", customerOrgId: "org-a", archived: false }];
    const rows = deviceRows([
      device("d-1", { unitId: "u-1" }), device("d-2", { unitId: "u-gone" }), device("d-3", { targetUnitId: "u-1" }), device("d-4", { targetUnitId: "u-gone" }), device("d-5", { tamper: "detected", iotModelId: "iot-1" }),
    ], units);
    expect(rows.map((r) => r.unit)).toStrictEqual(["Bedroom AC", "unit", "Not bound yet — planned for Bedroom AC", "Not bound yet — planned for unit", "Unbound"]);
    expect([rows[4]?.tamper, rows[4]?.iotModelId, rows[0]?.iotModelId, rows[0]?.targetUnitId]).toStrictEqual([true, "iot-1", null, null]);
  });
});

describe("a firmware campaign", () => {
  it("reads ascending waves that end at 100 %, and refuses any other", () => {
    expect(parseWaves("10, 50%, 100")).toStrictEqual([{ label: "Wave 1 · 10 %", percent: 10 }, { label: "Wave 2 · 50 %", percent: 50 }, { label: "Wave 3 · 100 %", percent: 100 }]);
    expect(["", "10, 50", "50, 10, 100", "0, 100", "10, 101", "10.5, 100", "10, 10, 100"].map((s) => parseWaves(s))).toStrictEqual(Array.from({ length: 7 }, () => null));
    expect(parseWaves("100")).toStrictEqual([{ label: "Wave 1 · 100 %", percent: 100 }]);
  });

  it("marks a wave done at 95 % succeeded, the first other one current, and nothing before the start", () => {
    const waves = [{ label: "Wave 1", percent: 50 }, { label: "Wave 2", percent: 100 }];
    const done = [result(0, "succeeded")];
    const open = [result(1, "pending")];
    expect(waveStates({ waves, results: [...done, ...open], state: "running" })).toStrictEqual(["done", "running"]);
    expect(waveStates({ waves, results: [result(0, "failed"), ...open], state: "paused" })).toStrictEqual(["paused", "waiting"]);
    expect(waveStates({ waves, results: [result(0, "failed"), ...open], state: "aborted" })).toStrictEqual(["aborted", "waiting"]);
    expect(waveStates({ waves, results: [...done, ...open], state: "scheduled" })).toStrictEqual(["waiting", "waiting"]);
    expect(waveStates({ waves, results: [...done, result(1, "skipped")], state: "completed" })).toStrictEqual(["done", "done"]); // a completed campaign's last wave is done even with skips
  });

  it("checks every field, the window as HH:MM and a start at least a day ahead in the display time zone", () => {
    const now = new Date("2026-09-14T00:00:00Z"); // 08:00 in Kuala Lumpur
    const ok: CampaignDraft = { modelId: "cap-1", targetVersion: "0.5.0", deviceIds: ["d-1"], waves: "10, 100", startLocal: "01:00", endLocal: "05:00", autoPause: "10", startAt: "2026-09-15T09:00" };
    expect(campaignErrors(ok, now)).toStrictEqual({});
    expect(campaignErrors({ ...ok, startAt: "2026-09-15T07:00" }, now)).toStrictEqual({ startAt: "At least 24 hours ahead" }); // 23 h ahead in Kuala Lumpur
    expect(campaignErrors({ ...ok, startAt: "2026-09-15T07:00" }, now, "Asia/Tokyo")).toStrictEqual({ startAt: "At least 24 hours ahead" });
    expect(campaignErrors({ ...ok, startAt: "2026-09-15T10:00" }, now, "Asia/Tokyo")).toStrictEqual({}); // 25 h ahead, read in Tokyo
    expect(campaignErrors({ modelId: "", targetVersion: "", deviceIds: [], waves: "50", startLocal: "1:00", endLocal: "05:00", autoPause: "51", startAt: "" }, now)).toStrictEqual({
      modelId: "Choose a model", targetVersion: "Choose a target version", deviceIds: "Choose at least one device", waves: "Ascending percentages ending at 100, e.g. 10, 50, 100",
      autoPause: "1–50 %", window: "HH:MM – HH:MM", startAt: "At least 24 hours ahead",
    });
    expect(["0", "2.5", "x"].map((autoPause) => campaignErrors({ ...ok, autoPause }, now).autoPause)).toStrictEqual(["1–50 %", "1–50 %", "1–50 %"]);
  });
});
