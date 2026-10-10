import { describe, expect, it } from "vitest";
import {
  calibrationItem, campaignErrors, campaignRows, campaignStart, capabilityDraft, capabilityErrors, deviceRows, modelRows, operationItem, parseWaves,
  type ApiCampaign, type ApiCapability, type ApiDevice, type CampaignDraft,
} from "@ac/web/lib/devices";
import { deviceEventItem } from "@ac/web/lib/audit";
import { i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const NOW = new Date("2026-09-14T01:00:00Z"); // 09:00 in Kuala Lumpur, 10:00 in Tokyo
const cap: ApiCapability = {
  id: "m1", version: 3, updatedAt: "2026-09-10T02:00:00Z", manufacturer: "DemoAir", model: "SPL-100", control: true, modeControl: true, fanControl: false, temperature: { min: 16, max: 30, step: 1 },
  modes: ["cool"], fanLevels: [], ventilation: false, ventilationLevels: [], sensors: [{ metric: "temperature", unit: "°C", staleAfterSeconds: 120, boundaryId: null }], firmwareCandidates: ["v1", "v2"],
};
const device = (over: Partial<ApiDevice>): ApiDevice => ({ id: "d1", version: 2, unitId: "u1", serial: "AC-DEMO-0001", connection: "online", lastSeenAt: null, firmwareVersion: "v1", powerSignal: "on", tamper: "clear", sensors: [], ...over });
const units = [{ id: "u1", displayName: "Bedroom AC", modelId: "m1", customerOrgId: "org-a", archived: false }, { id: "u2", displayName: "Old AC", modelId: "m1", customerOrgId: "org-a", archived: true }];
const draft = (over: Partial<CampaignDraft>): CampaignDraft => ({ modelId: "m1", targetVersion: "v2", deviceIds: ["d1"], waves: "10, 50, 100", startLocal: "02:00", endLocal: "05:00", autoPause: "5", startAt: "2026-09-16T10:00", ...over });

describe("HQ device registry", () => {
  it("lists models and devices", () => {
    expect(modelRows([cap], units, [device({}), device({ id: "d2", unitId: null })])[0]).toMatchObject({ name: "DemoAir SPL-100", used: "1 unit · 1 device", updated: "10 Sept 2026, 10:00 am MYT" });
    expect(deviceRows([device({}), device({ id: "d2", unitId: null, tamper: "detected" })], units).map((r) => [r.unit, r.tamper])).toEqual([["Bedroom AC", false], ["Unbound", true]]);
    expect(capabilityErrors({ ...capabilityDraft(cap), control: false, changeReason: "" }, true)).toEqual({ control: "Mode, fan or temperature control needs power control", changeReason: "A reason is required (1–1000 characters)" });
  });

  it("words operations, calibrations and device events in the display time zone", () => {
    expect(operationItem({ id: "o1", kind: "firmware", status: "failed", targetVersion: "v2", failureCode: "OFFLINE", createdAt: "2026-09-14T00:30:00Z", finishedAt: null }))
      .toEqual({ time: "14 Sept 2026, 8:30 am MYT", title: "Firmware update → v2 · failed", detail: "failure OFFLINE", tone: "crit" });
    expect(operationItem({ id: "o2", kind: "check", status: "succeeded", targetVersion: null, failureCode: null, createdAt: "2026-09-14T00:30:00Z", finishedAt: "2026-09-14T00:31:00Z" }).detail).toBe("finished 14 Sept 2026, 8:31 am MYT");
    expect(calibrationItem({ id: "c1", sensorId: "s1", metric: "temperature", unit: "°C", referenceValue: 25, measuredValue: 25.4, calibratedAt: "2026-09-14T00:30:00Z", actorId: "a" }))
      .toMatchObject({ title: "temperature: reference 25 °C · measured 25.4 °C", detail: "offset 0.40 °C · demo" });
    const ev = { id: "e1", deviceId: "d1", eventType: "communication_lost", evidenceSource: "heartbeat", occurredAt: "2026-09-14T00:30:00Z", restoredAt: "2026-09-14T00:40:00Z" };
    expect(deviceEventItem(ev)).toEqual({ time: "09-14 08:30", title: "communication lost", detail: "heartbeat · restored" }); // the audit screen's English and Kuala Lumpur
    expect(deviceEventItem(ev, MS, NOW.getTime())).toEqual({ time: "hari ini 9:30 PG GMT+9", title: "komunikasi terputus", detail: "heartbeat · dipulihkan" });
  });

  it("checks a campaign: waves, the start in the display time zone and the device window", () => {
    expect(parseWaves("10, 50, 100")?.map((w) => w.percent)).toEqual([10, 50, 100]);
    expect([parseWaves("50, 10, 100"), parseWaves("10, 50"), parseWaves("0, 100")]).toEqual([null, null, null]);
    expect([campaignStart(draft({}), "Asia/Kuala_Lumpur"), campaignStart(draft({}), "Asia/Tokyo"), campaignStart(draft({ startAt: "" }), "Asia/Tokyo")]).toEqual(["2026-09-16T02:00:00.000Z", "2026-09-16T01:00:00.000Z", ""]);
    expect(campaignErrors(draft({}), NOW, "Asia/Tokyo")).toEqual({});
    expect(campaignErrors(draft({ startAt: "2026-09-15T09:30", waves: "10, 50", autoPause: "60", deviceIds: [] }), NOW, "Asia/Tokyo")).toEqual({
      startAt: "At least 24 hours ahead", waves: "Ascending percentages ending at 100, e.g. 10, 50, 100", autoPause: "1–50 %", deviceIds: "Choose at least one device",
    }); // 09:30 tomorrow in Tokyo is 23.5 h ahead
    const c: ApiCampaign = { id: "k1", version: 1, modelId: "m1", fromVersions: [], targetVersion: "v2", deviceIds: ["d1"], waves: [{ label: "Wave 1 · 100 %", percent: 100 }], window: { startLocal: "02:00", endLocal: "05:00" },
      autoPauseFailurePercent: 5, startAt: "2026-09-16T02:00:00Z", state: "scheduled", progress: { succeeded: 0, installing: 0, pending: 1, failed: 0, skipped: 0 }, results: [], reason: null };
    expect(campaignRows([c], [cap])[0]).toMatchObject({ model: "DemoAir SPL-100", versions: "any → v2", start: "16 Sept 2026, 10:00 am MYT" });
  });
});

describe("HQ device registry in Malay with the display time zone (IR295)", () => {
  it("words the rows, operations and campaign checks", () => {
    expect(modelRows([cap], units, [], MS)[0]).toMatchObject({ used: "1 unit · 0 peranti", updated: "10 Sep 2026, 11:00 PG GMT+9" });
    expect(deviceRows([device({ unitId: null })], units, MS.t)[0].unit).toBe("Tidak terikat");
    expect(operationItem({ id: "o1", kind: "check", status: "running", targetVersion: null, failureCode: null, createdAt: "2026-09-14T00:30:00Z", finishedAt: null }, MS))
      .toMatchObject({ title: "Semakan sambungan · sedang berjalan", detail: "sedang berjalan" });
    expect(campaignErrors(draft({ modelId: "" }), NOW, "Asia/Tokyo", MS.t)).toEqual({ modelId: "Pilih model" });
    expect(campaignRows([{ ...({} as ApiCampaign), id: "k", version: 1, modelId: "x", fromVersions: [], targetVersion: "v2", deviceIds: [], startAt: "2026-09-16T02:00:00Z", state: "paused" } as ApiCampaign], [], MS)[0])
      .toMatchObject({ model: "model", versions: "mana-mana → v2", start: "16 Sep 2026, 11:00 PG GMT+9" });
  });
});
