import { describe, expect, it } from "vitest";
import { translator } from "@ac/web/lib/i18n";
import { unitDetail } from "./unitFixture";
import { actionText, answerSeconds, controlLayout, coolingHint, undoActions, historyRow, latest, setsTemperature, type ApiCommand, type ApiUnitDetail } from "@ac/web/lib/units";

const TEMPERATURE = "temperature";

const ms = translator("ms");
const tokyo = { locale: "en" as const, timeZone: "Asia/Tokyo" };
const deviceCommand = (over: Partial<ApiCommand>): ApiCommand => ({ id: "cmd-12345678-abcd", action: { kind: "set_temperature", celsius: 24 }, status: "acknowledged", requestedAt: "2026-09-14T01:00:00Z", failureCode: null, ...over });

describe("unit commands and readings in the display language and time zone (FR-X01, IR44, IR259)", () => {
  it("words every command in English and Malay, the stored values unchanged", () => {
    expect([actionText({ kind: "set_power", power: true }), actionText({ kind: "set_mode", mode: "cool" }), actionText({ kind: "set_fan", fanLevel: "mid" })]).toStrictEqual(["Set power ON", "Set mode COOL", "Set fan MID"]);
    expect([actionText({ kind: "set_power", power: false }, ms), actionText({ kind: "set_temperature", celsius: 24 }, ms), actionText({ kind: "set_mode", mode: "dry" }, ms)]).toStrictEqual(["Tetapkan kuasa MATI", "Tetapkan suhu 24°C", "Tetapkan mod KERING"]);

    // every kind of the contract has words: ventilation (an automation) and a restriction's apply / remove (IR314)
    const limit = { kind: "apply_restriction", restrictionId: "r1", rulesVersion: "v1", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 26 } } as const;
    const remove = { kind: "remove_restriction", restrictionId: "r1", rulesVersion: "v1" } as const;
    expect([actionText({ kind: "ventilate", level: "high" }), actionText(limit), actionText({ ...limit, policy: { kind: "power_off" } }), actionText(remove)])
      .toStrictEqual(["Set ventilation HIGH", "Apply restriction: Temperature limit — cooling setpoint ≥ 26 °C", "Apply restriction: Power off", "Remove restriction"]);
    expect([actionText({ kind: "ventilate", level: "low" }, ms), actionText(limit, ms), actionText(remove, ms)])
      .toStrictEqual(["Tetapkan pengudaraan RENDAH", "Kenakan sekatan: Had suhu — suhu tetapan penyejukan ≥ 26 °C", "Tarik balik sekatan"]);
    expect(historyRow(deviceCommand({ action: limit, source: "restriction" })).text).toBe("Apply restriction: Temperature limit — cooling setpoint ≥ 26 °C — acknowledged by device · by a restriction");
  });

  it("states a command's outcome and source, with the IR44 time of the request", () => {
    expect(historyRow(deviceCommand({}))).toStrictEqual({ id: "cmd-1234", text: "Set temperature 24°C — acknowledged by device", when: "14 Sept 2026, 9:00 am MYT", bad: false });
    expect(historyRow(deviceCommand({ status: "failed", failureCode: "DEVICE_REJECTED", source: "automation" }), ms, { locale: "ms", timeZone: "Asia/Tokyo" }))
      .toStrictEqual({ id: "cmd-1234", text: "Tetapkan suhu 24°C — gagal: AC menolaknya · oleh automasi", when: "14 Sep 2026, 10:00 PG GMT+9", bad: true });
    // IR799: a failure code is never shown as it is; one the web does not know reads as no reason given
    expect(historyRow(deviceCommand({ status: "failed", failureCode: "SOMETHING_NEW" })).text).toBe("Set temperature 24°C — failed: no reason given");
    // IR804: a command sent by hand says whether the reader sent it; an automation's keeps its source
    const byHand = (actor: string, source = "ui") => historyRow(deviceCommand({ actorMembershipId: actor, source }), undefined, undefined, "m-me").text;
    expect([byHand("m-me"), byHand("m-other"), byHand("m-other", "automation"), historyRow(deviceCommand({ actorMembershipId: "m-other" })).text]).toStrictEqual([
      "Set temperature 24°C — acknowledged by device · by you", "Set temperature 24°C — acknowledged by device · by someone else", "Set temperature 24°C — acknowledged by device · by an automation", "Set temperature 24°C — acknowledged by device",
    ]);
    expect(historyRow(deviceCommand({ status: "expired" }), undefined, tokyo)).toMatchObject({ text: "Set temperature 24°C — no device response (expired)", when: "14 Sept 2026, 10:00 am GMT+9", bad: true });
    // a command from Unit Control (UI) carries no label
    expect(historyRow(deviceCommand({ source: "ui" })).text).toBe("Set temperature 24°C — acknowledged by device");
  });

  it("keeps a stale reading's quality beside it and times it in the display time zone", () => {
    const d = unitDetail({ latestMeasurements: [
      { id: "m1", unitId: "u1", sensorId: "s1", metric: TEMPERATURE, value: 28.04, unit: "°C", observedAt: "2026-09-14T01:12:00Z", origin: "measured", quality: "stale", qualityReason: null },
      { id: "m2", unitId: "u1", sensorId: "s2", metric: "humidity", value: null, unit: "%", observedAt: "2026-09-14T01:12:00Z", origin: "measured", quality: "missing", qualityReason: null },
    ] });
    expect(latest(d, TEMPERATURE)).toStrictEqual({ text: "28.0 °C (stale)", at: "9:12 am MYT" });
    expect(latest(d, TEMPERATURE, ms, tokyo)).toStrictEqual({ text: "28.0 °C (lapuk)", at: "10:12 am GMT+9" });
    expect([latest(d, "humidity"), latest(d, "power")]).toStrictEqual([null, null]);
    // with the scenario's now (IR613): the same day keeps the time alone, another day adds the date
    expect(latest(d, TEMPERATURE, undefined, undefined, Date.parse("2026-09-14T09:00:00Z"))?.at).toBe("9:12 am MYT");
    expect(latest(d, TEMPERATURE, undefined, undefined, Date.parse("2026-09-23T06:00:00Z"))?.at).toBe("14 Sept 2026, 9:12 am MYT");
    expect(latest(d, TEMPERATURE, ms, tokyo, Date.parse("2026-09-23T06:00:00Z"))?.at).toBe("14 Sept 2026, 10:12 am GMT+9");
  });
});

const model = (over: Partial<ApiUnitDetail["capabilities"]>): ApiUnitDetail["capabilities"] => {
  return {
    manufacturer: "DemoAir", model: "SPL-100", control: true, modeControl: true, fanControl: true, temperature: { min: 16, max: 30, step: 1 },
    modes: ["cool", "dry", "fan"], fanLevels: ["low", "mid", "high"], ventilation: false, ventilationLevels: [], ...over,
  };
};

describe("the remote control a model offers (deterministic contracts, AT-X06; IR524, IR530)", () => {

  it("offers every setting of a full model, and the demo screen's without a model", () => {
    expect(controlLayout(model({}))).toStrictEqual({ control: true, temperature: { min: 16, max: 30, step: 1 }, modes: ["cool", "dry", "fan"], fans: ["low", "mid", "high"], review: true });
    expect(controlLayout(null).review).toBe(true);
  });

  it("offers only the model's own modes and fan levels, and no temperature without a range", () => {
    expect(controlLayout(model({ modes: ["cool"], fanLevels: ["low", "high"], temperature: null }))).toMatchObject({ temperature: null, modes: ["cool"], fans: ["low", "high"], review: true });
    expect(controlLayout(model({ modeControl: false, fanControl: false }))).toMatchObject({ modes: [], fans: [], review: true }); // the temperature is left
  });

  it("offers the power alone without a review bar, and nothing on a model without control", () => {
    expect(controlLayout(model({ modeControl: false, fanControl: false, temperature: null, modes: [], fanLevels: [] }))).toStrictEqual({ control: true, temperature: null, modes: [], fans: [], review: false });
    expect(controlLayout(model({ control: false }))).toStrictEqual({ control: false, temperature: null, modes: [], fans: [], review: false });
    // other screens point a temperature to Unit Control only when its temperature row is there (IR544)
    expect([setsTemperature(model({})), setsTemperature(model({ temperature: null })), setsTemperature(model({ control: false })), setsTemperature(null)]).toStrictEqual([true, false, false, true]);
  });
});

// IR806: the countdown while waiting for the AC starts from the command's own answer window
describe("answer window", () => {
  it("is the span from request to expiry, else 30 s", () => {
    expect([
      answerSeconds({ requestedAt: "2026-09-22T13:31:40Z", expiresAt: "2026-09-22T13:32:10Z" }),
      answerSeconds({ requestedAt: "2026-09-22T13:31:40Z", expiresAt: "2026-09-22T13:33:40Z" }),
      answerSeconds({ requestedAt: "2026-09-22T13:31:40Z" }),
      answerSeconds({ requestedAt: "2026-09-22T13:31:40Z", expiresAt: "2026-09-22T13:31:40Z" }),
      answerSeconds({ requestedAt: "bad", expiresAt: "2026-09-22T13:31:40Z" }),
    ]).toStrictEqual([30, 120, 30, 30, 30]);
  });
});

// IR808: a very low cooling setpoint gets a hint; other modes and 22 °C and above do not
describe("cooling hint", () => {
  it("speaks only for cooling below 22 °C", () => {
    expect([coolingHint("cool", 18), coolingHint("cool", 22), coolingHint("dry", 18), coolingHint("fan", 16)].map((h) => h?.slice(0, 33) ?? null))
      .toStrictEqual(["18°C is very cold for cooling: a ", null, null, null]);
  });
});

// IR810: Undo puts back each sent setting to the confirmed one it replaced, nothing it cannot know or that did not change
describe("undo", () => {
  it("reverses the sent settings from the state before", () => {
    const before = { power: true, celsius: 26, mode: "cool" as const, fanLevel: "mid" as const, observedAt: null };
    expect(undoActions([{ kind: "set_temperature", celsius: 24 }, { kind: "set_fan", fanLevel: "high" }], before))
      .toStrictEqual([{ kind: "set_temperature", celsius: 26 }, { kind: "set_fan", fanLevel: "mid" }]);
    expect(undoActions([{ kind: "set_power", power: false }], before)).toStrictEqual([{ kind: "set_power", power: true }]);
    expect(undoActions([{ kind: "set_mode", mode: "dry" }, { kind: "set_temperature", celsius: 26 }], { ...before, mode: null })).toStrictEqual([]);
  });
});
