import { describe, expect, it } from "vitest";
import { translator } from "@ac/web/lib/i18n";
import { actionText, historyRow, latest, type ApiCommand, type ApiUnitDetail } from "@ac/web/lib/units";

const ms = translator("ms");
const tokyo = { locale: "en" as const, timeZone: "Asia/Tokyo" };
const cmd = (over: Partial<ApiCommand>): ApiCommand => ({ id: "cmd-12345678-abcd", action: { kind: "set_temperature", celsius: 24 }, status: "acknowledged", requestedAt: "2026-09-14T01:00:00Z", failureCode: null, ...over });

describe("unit commands and readings in the display language and time zone (FR-X01, IR44, IR259)", () => {
  it("words every command in English and Malay, the stored values unchanged", () => {
    expect([actionText({ kind: "set_power", power: true }), actionText({ kind: "set_mode", mode: "cool" }), actionText({ kind: "set_fan", fanLevel: "mid" })]).toEqual(["Set power ON", "Set mode COOL", "Set fan MID"]);
    expect([actionText({ kind: "set_power", power: false }, ms), actionText({ kind: "set_temperature", celsius: 24 }, ms), actionText({ kind: "set_mode", mode: "dry" }, ms)]).toEqual(["Tetapkan kuasa MATI", "Tetapkan suhu 24°C", "Tetapkan mod KERING"]);
  });

  it("states a command's outcome and source, with the IR44 time of the request", () => {
    expect(historyRow(cmd({}))).toEqual({ id: "cmd-1234", text: "Set temperature 24°C — acknowledged by device", when: "14 Sept 2026, 9:00 am MYT", bad: false });
    expect(historyRow(cmd({ status: "failed", failureCode: "device_rejected", source: "automation" }), ms, { locale: "ms", timeZone: "Asia/Tokyo" }))
      .toEqual({ id: "cmd-1234", text: "Tetapkan suhu 24°C — gagal (device_rejected) · oleh automasi", when: "14 Sep 2026, 10:00 PG GMT+9", bad: true });
    expect(historyRow(cmd({ status: "expired" }), undefined, tokyo)).toMatchObject({ text: "Set temperature 24°C — no device response (expired)", when: "14 Sept 2026, 10:00 am GMT+9", bad: true });
  });

  it("keeps a stale reading's quality beside it and times it in the display time zone", () => {
    const d = { latestMeasurements: [
      { id: "m1", unitId: "u1", sensorId: "s1", metric: "temperature", value: 28.04, unit: "°C", observedAt: "2026-09-14T01:12:00Z", origin: "measured", quality: "stale", qualityReason: null },
      { id: "m2", unitId: "u1", sensorId: "s2", metric: "humidity", value: null, unit: "%", observedAt: "2026-09-14T01:12:00Z", origin: "measured", quality: "missing", qualityReason: null },
    ] } as unknown as ApiUnitDetail;
    expect(latest(d, "temperature")).toEqual({ text: "28.0 °C (stale)", at: "9:12 am MYT" });
    expect(latest(d, "temperature", ms, tokyo)).toEqual({ text: "28.0 °C (lapuk)", at: "10:12 am GMT+9" });
    expect([latest(d, "humidity"), latest(d, "power")]).toEqual([null, null]);
  });
});
