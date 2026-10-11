import { describe, expect, it } from "vitest";
import {
  campaignErrors, campaignFacts, campaignRows, campaignStart, capabilityChanges, capabilityDraft, capabilityErrors, capabilityInput, deviceRows, firmwareProblem, modelRows, parseWaves,
  resultReason, shortChecksum, waveStates,
  type ApiCampaign, type ApiCapability, type ApiDevice, type CampaignDraft,
} from "@ac/web/lib/devices";
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

  it("checks a campaign: waves, the start in the display time zone and the device window", () => {
    expect(parseWaves("10, 50, 100")?.map((w) => w.percent)).toEqual([10, 50, 100]);
    expect([parseWaves("50, 10, 100"), parseWaves("10, 50"), parseWaves("0, 100")]).toEqual([null, null, null]);
    expect([campaignStart(draft({}), "Asia/Kuala_Lumpur"), campaignStart(draft({}), "Asia/Tokyo"), campaignStart(draft({ startAt: "" }), "Asia/Tokyo")]).toEqual(["2026-09-16T02:00:00.000Z", "2026-09-16T01:00:00.000Z", ""]);
    expect(campaignErrors(draft({}), NOW, "Asia/Tokyo")).toEqual({});
    expect(campaignErrors(draft({ startAt: "2026-09-15T09:30", waves: "10, 50", autoPause: "60", deviceIds: [] }), NOW, "Asia/Tokyo")).toEqual({
      startAt: "At least 24 hours ahead", waves: "Ascending percentages ending at 100, e.g. 10, 50, 100", autoPause: "1–50 %", deviceIds: "Choose at least one device",
    }); // 09:30 tomorrow in Tokyo is 23.5 h ahead
    const c: ApiCampaign = { id: "k1", version: 1, modelId: "m1", fromVersions: [], targetVersion: "v2", deviceIds: ["d1"], waves: [{ label: "Wave 1 · 100 %", percent: 100 }], window: { startLocal: "02:00", endLocal: "05:00" },
      autoPauseFailurePercent: 5, startAt: "2026-09-16T02:00:00Z", state: "scheduled", progress: { succeeded: 0, installing: 0, pending: 1, failed: 0, skipped: 0 }, results: [], reason: null,
      checksum: "sha256:9f3c0e5d7a1b2c3da1", createdAt: "2026-09-14T01:00:00Z", createdByMembershipId: "m-hq" };
    expect(campaignRows([c], [cap])[0]).toMatchObject({ model: "DemoAir SPL-100", versions: "any → v2", start: "16 Sept 2026, 10:00 am MYT" });
    // the facts above the waves (Figma Admin 04-9)
    expect(campaignFacts(campaignRows([c], [cap])[0])).toEqual([
      ["Model / version", "DemoAir SPL-100 · any → v2 (signed, sha256 9f3c…a1)"], ["Window", "02:00–05:00 device local time"], ["Auto-pause", "failure rate > 5 % in a wave"],
      ["Starts", "16 Sept 2026, 10:00 am MYT"], ["Created", "14 Sept 2026, 9:00 am MYT"],
    ]);
    // the scheduling member's name where the session may read members (IR344)
    expect(campaignFacts(campaignRows([c], [cap], undefined, new Map([["m-hq", "hq-operator"]]))[0]).at(-1)).toEqual(["Created", "hq-operator · 14 Sept 2026, 9:00 am MYT"]);
    expect([shortChecksum("sha256:abcd"), shortChecksum("plain0123456789")]).toEqual(["sha256 abcd", "plai…89"]);
    expect(campaignFacts(campaignRows([c], [cap], MS)[0], MS).map(([k]) => k)).toEqual(["Model / versi", "Tetingkap", "Jeda automatik", "Bermula", "Dicipta"]);
  });

  it("words the runner's results and each wave's state (BR-A20, IR339)", () => {
    expect([resultReason("errors.device_offline"), resultReason("errors.install_timeout"), resultReason("error.checksum_mismatch"), resultReason(null)])
      .toEqual(["Skipped — offline", "Failed — no answer within 60 s; the old version is kept", "checksum mismatch", ""]);
    const r = (waveIndex: number, status: ApiCampaign["results"][number]["status"]) => ({ deviceId: `d${waveIndex}${status}`, waveIndex, status, reasonKey: null, at: null });
    const waves = [{ label: "Pilot", percent: 34 }, { label: "All", percent: 100 }, { label: "Rest", percent: 100 }];
    const results = [r(0, "succeeded"), r(0, "succeeded"), r(1, "succeeded"), r(1, "failed"), r(2, "pending")];
    expect(waveStates({ waves, results, state: "running" })).toEqual(["done", "running", "waiting"]);
    expect(waveStates({ waves, results, state: "paused" })).toEqual(["done", "paused", "waiting"]);
    expect(waveStates({ waves, results: results.map((x) => ({ ...x, status: "pending" as const })), state: "scheduled" })).toEqual(["waiting", "waiting", "waiting"]);
    expect(waveStates({ waves: waves.slice(0, 1), results: [r(0, "succeeded")], state: "completed" })).toEqual(["done"]);
  });
});

describe("HQ device registry in Malay with the display time zone (IR295)", () => {
  it("words the rows, operations and campaign checks", () => {
    expect(modelRows([cap], units, [], MS)[0]).toMatchObject({ used: "1 unit · 0 peranti", updated: "10 Sep 2026, 11:00 PG GMT+9" });
    expect(deviceRows([device({ unitId: null })], units, MS.t)[0].unit).toBe("Tidak terikat");
    expect(campaignErrors(draft({ modelId: "" }), NOW, "Asia/Tokyo", MS.t)).toEqual({ modelId: "Pilih model" });
    expect(campaignRows([{ ...({} as ApiCampaign), id: "k", version: 1, modelId: "x", fromVersions: [], targetVersion: "v2", deviceIds: [], startAt: "2026-09-16T02:00:00Z", state: "paused" } as ApiCampaign], [], MS)[0])
      .toMatchObject({ model: "model", versions: "mana-mana → v2", start: "16 Sep 2026, 11:00 PG GMT+9" });
    expect(resultReason("errors.device_busy", MS.t)).toBe("Dilangkau — unit sedang sibuk (operasi, arahan, ujian diagnostik atau sekatan)");
  });
});

describe("the model editor's changes and firmware candidates (Figma Admin 106:3, IR350)", () => {
  it("names what a draft changes, not the reason", () => {
    const saved = capabilityDraft(cap);
    expect(capabilityChanges(saved, saved)).toEqual([]);
    expect(capabilityChanges({ ...saved, changeReason: "bulletin" }, saved)).toEqual([]);
    expect(capabilityChanges({ ...saved, ventilation: true, ventilationLevels: ["low"], firmware: ["v1"], max: "28" }, saved)).toEqual(["temperature", "ventilation levels", "firmware candidates"]);
    expect(capabilityChanges({ ...saved, modes: ["cool"], min: "16.0" }, saved)).toEqual([]); // the same values written differently
    expect(capabilityChanges({ ...saved, model: "SPL-101" }, saved, MS.t)).toEqual(["identiti"]);
  });
  it("adds a firmware version once, trimmed, and saves the list without duplicates", () => {
    expect([firmwareProblem("", ["v1"]), firmwareProblem(" v1 ", ["v1"]), firmwareProblem("x".repeat(33), []), firmwareProblem(" v3 ", ["v1"])])
      .toEqual(["Enter a version", "v1 is already a candidate", "Firmware versions are at most 32 characters", null]);
    expect(capabilityInput({ ...capabilityDraft(cap), firmware: ["v1", " v2", "v1"] }).firmwareCandidates).toEqual(["v1", "v2"]);
    expect(capabilityDraft(cap).firmware).toEqual(["v1", "v2"]);
  });
});
