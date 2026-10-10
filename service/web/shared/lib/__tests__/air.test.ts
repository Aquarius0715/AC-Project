import { describe, expect, it } from "vitest";
import { airNumber, metricCard, type ApiMeasurement } from "@ac/web/lib/air";
import { i18nOf } from "@ac/web/lib/i18n";

const MS_TOKYO = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const m = (metric: string, value: number | null, over: Partial<ApiMeasurement> = {}): ApiMeasurement => ({
  id: "m", unitId: "u", sensorId: "s", metric, value, unit: metric === "co2" ? "ppm" : metric === "pm25" ? "µg/m³" : metric === "temperature" ? "°C" : "%", observedAt: "2026-09-14T01:12:30Z", origin: "measured", quality: "valid", qualityReason: null, ...over,
});

describe("air-quality metric cards (FR-C07, IR44, IR99, IR213, IR260)", () => {
  it("rounds per IR44 and states a live reading, its guide and advice", () => {
    expect([airNumber("temperature", 28.05), airNumber("co2", 999.5), airNumber("pm25", 34.4), airNumber("power", 0.68)]).toEqual(["28.1", "1000", "34", "0.68"]);
    expect(metricCard("co2", m("co2", 1180), true)).toMatchObject({ value: "1180", feed: { text: "Live" }, status: { text: "High", tone: "warn" }, sub: "Observed 9:12 am MYT", advice: "Ventilation recommended (≥ 1000 ppm)", warn: true });
    expect(metricCard("pm25", m("pm25", 12, { origin: "estimated" }), true)).toMatchObject({ status: { text: "Within guide" }, sub: "Observed 9:12 am MYT · estimated", advice: "No current advice", warn: false });
    expect(metricCard("temperature", m("temperature", 26), true)).toMatchObject({ status: { text: "Measured" }, advice: "Setpoint is on Unit Control" });
  });

  it("never shows a stale, suspect, null or unsupported reading as a current value", () => {
    expect(metricCard("co2", m("co2", 900, { quality: "stale" }), true)).toMatchObject({ value: null, feed: { text: "Unavailable" }, status: { text: "Not measured" }, sub: "No reading since 9:12 am MYT (last 900 ppm)" });
    expect(metricCard("co2", m("co2", 5000, { quality: "suspect", qualityReason: "out_of_range" }), true)).toMatchObject({ value: "5000", feed: { text: "Suspect" }, sub: "Observed 9:12 am MYT · out of range", advice: "Not used for advice", warn: false });
    expect(metricCard("humidity", m("humidity", null), true)).toMatchObject({ value: null, feed: { text: "Unknown" }, sub: "No reading (null) — not 0% · 9:12 am MYT", advice: "Not enough data for advice" });
    expect(metricCard("pm25", undefined, false)).toMatchObject({ feed: { text: "Unsupported" }, sub: "No PM2.5 sensor on this model", advice: "Not enough data for PM2.5 advice" });
    expect(metricCard("co2", undefined, true).sub).toBe("No reading yet");
  });

  it("words the card in Malay and times the reading in the display time zone", () => {
    expect(metricCard("co2", m("co2", 1180), true, MS_TOKYO)).toMatchObject({ feed: { text: "Langsung" }, status: { text: "Tinggi" }, sub: "Diperhatikan 10:12 PG GMT+9", advice: "Pengudaraan disyorkan (≥ 1000 ppm)" });
    expect(metricCard("temperature", undefined, false, MS_TOKYO)).toMatchObject({ label: "Suhu", sub: "Tiada penderia Suhu pada model ini", feed: { text: "Tidak disokong" } });
  });
});
