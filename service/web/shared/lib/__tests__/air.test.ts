import { describe, expect, it } from "vitest";
import {
  airGuidance, airNumber, airRooms, airRows, airSlots, airWindow, allergenView, axisLabels, cleanNote, co2Now, metricCard, ventRow, ventStrip, windowSub, windowTitle,
  type ApiAllergen, type ApiMeasurement, type ApiVentilationLog,
} from "@ac/web/lib/air";
import type { ApiPropertyRow, ApiSpaceRow, ApiUnitRow } from "@ac/web/lib/assets";
import { i18nOf, showClock, showDay } from "@ac/web/lib/i18n";

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

describe("the air-quality screen in the display language and time zone (FR-C07, IR266)", () => {
  const NOW = new Date("2026-09-14T01:12:40Z"); // Mon 09:12 KL
  const ps = [{ id: "p1", name: "Home A", archived: false }] as unknown as ApiPropertyRow[];
  const ss = [{ id: "s1", propertyId: "p1", parentSpaceId: null, name: "1F", archived: false }, { id: "s2", propertyId: "p1", parentSpaceId: "s1", name: "Bedroom", archived: false }] as unknown as ApiSpaceRow[];
  const us = [{ id: "u1", propertyId: "p1", spaceId: "s2", displayName: "Bedroom AC", archived: false }, { id: "u2", propertyId: "p1", spaceId: null, displayName: "Porch AC", archived: false }] as unknown as ApiUnitRow[];

  it("names the rooms, the guidance and the cleaning note in Malay", () => {
    expect(airRooms(ps, ss, us).map((r) => r.label)).toEqual(["Bedroom · Home A › 1F", "Not in a room · Home A"]);
    expect(airRooms(ps, ss, us, MS_TOKYO.t).map((r) => r.label)).toEqual(["Bedroom · Home A › 1F", "Bukan dalam bilik · Home A"]);
    const g = airGuidance(m("co2", 1180), m("pm25", 40), true);
    expect(g).toEqual(["ventilate", "clean"]);
    expect(ventStrip(g, m("co2", 1180), "Bedroom", "Bedroom AC", true)).toEqual({
      tone: "warn", title: "Ventilation recommended", text: "CO2 1180 ppm in Bedroom (≥ 1000 ppm guide). Open a window or run your ventilation fan, then log it — the record is kept in this room’s ventilation history.",
    });
    expect(ventStrip(airGuidance(m("co2", 1180), undefined, false), m("co2", 1180), "Bedroom", "Bedroom AC", false, MS_TOKYO.t)).toEqual({
      tone: "warn", title: "Udarakan secara manual, contohnya dengan membuka tingkap",
      text: "CO2 1180 ppm di Bedroom (panduan ≥ 1000 ppm). Bedroom AC tiada fungsi udara segar, jadi tiada apa-apa dihantar ke AC — buka tingkap, kemudian rekodkan apa yang anda lakukan.",
    });
    expect([ventStrip(["unavailable"], undefined, "Bedroom", "Bedroom AC", true).title, ventStrip(["none"], m("co2", 600), "Bedroom", "Bedroom AC", false, MS_TOKYO.t).title, ventStrip(["none"], m("co2", 600), "Bedroom", "Bedroom AC", true, MS_TOKYO.t).title])
      .toEqual(["Not enough data to provide guidance", "Pengudaraan manual", "Tiada panduan semasa"]);
    expect([cleanNote(g, m("pm25", 40), "Bedroom", MS_TOKYO.t), cleanNote(["none"], m("pm25", 10), "Bedroom")]).toEqual([
      "Pembersihan dan pemeriksaan penapis disyorkan — PM2.5 40 µg/m³ di Bedroom (panduan ≥ 35 µg/m³). Minta pembersihan penapis daripada Penyelenggaraan.", null,
    ]);
  });

  it("never reads missing allergen data as “no allergens”, in either language (IR98)", () => {
    const seen: ApiAllergen = { availability: "available", substance: "Dust mite", value: 12, unit: "ng/m³", sourceLabel: "Lab kit", observedAt: "2026-09-14T01:00:00Z", evidenceText: "Kit #4" };
    expect(allergenView(seen, MS_TOKYO)).toEqual({ title: "Alergen (Dust mite)", badge: { text: "Dikesan", tone: "warn", icon: "⚠" }, text: "12 ng/m³ · Sumber: Lab kit · 10:00 PG GMT+9 · Kit #4" });
    expect(allergenView({ ...seen, unit: null }).text).toBe("Value unknown (no unit) · Source: Lab kit · 9:00 am MYT · Kit #4");
    expect([allergenView(null).text, allergenView({ ...seen, evidenceText: null }, MS_TOKYO).badge.text, allergenView({ ...seen, availability: "unsupported" }, MS_TOKYO).text]).toEqual([
      "No observation for this selection — this does not mean “no allergens”.", "Tidak diketahui", "Unit ini tidak dapat memerhati alergen — ini tidak bermaksud “tiada alergen”.",
    ]);
  });

  it("labels the chart in the display time zone and names the 7-day window's Kuala Lumpur start (IR41, NFR-08)", () => {
    const day = airWindow("24h", NOW);
    expect([windowTitle("24h"), windowTitle("1h", MS_TOKYO.t), windowSub(day, "ppm", "Bedroom"), windowSub(day, "ppm", "Bedroom", MS_TOKYO)]).toEqual([
      "last 24 hours", "sejam lalu", "ppm · Bedroom · rolling window ending 9:12 am MYT · 5-min averages", "ppm · Bedroom · tetingkap bergerak berakhir 10:12 PG GMT+9 · purata 5 minit",
    ]);
    expect(axisLabels(day)).toEqual(["9:12 am yesterday", "3:12 pm", "9:12 pm", "3:12 am", "9:12 am now"]);
    expect(axisLabels(day, MS_TOKYO)).toEqual(["10:12 PG semalam", "4:12 PTG", "10:12 PTG", "4:12 PG", "10:12 PG sekarang"]);
    const week = airWindow("7d", NOW);
    expect([week.from, windowSub(week, "ppm", "Bedroom", MS_TOKYO)]).toEqual(["2026-09-07T16:00:00.000Z", "ppm · Bedroom · hari kalendar dalam Asia/Kuala_Lumpur dari 8 Sep 2026 hingga 10:12 PG GMT+9 sekarang · purata setiap jam"]);
    expect([axisLabels(week), axisLabels(week, MS_TOKYO)]).toEqual([["Tue 8", "Wed 9", "Fri 11", "Sat 12", "9:12 am now"], ["Sel 8", "Rab 9", "Jum 11", "Sab 12", "10:12 PG sekarang"]]);
    expect([showClock("2026-09-14T01:12:30Z", MS_TOKYO.display, false), showDay("2026-09-14T01:12:30Z", MS_TOKYO.display), showDay("2026-09-13T20:00:00Z", { locale: "en", timeZone: "Not/AZone" })]).toEqual(["10:12 PG", "Isn 14", "Mon 14"]);
  });

  it("lists each slot as a span, newest first, with the gaps merged", () => {
    const hour = airWindow("1h", NOW);
    const slots = airSlots([m("co2", 800, { observedAt: "2026-09-14T00:14:00Z" }), m("co2", 820, { observedAt: "2026-09-14T00:15:30Z" }), m("co2", 900, { observedAt: "2026-09-14T01:05:00Z" })], hour);
    expect(airRows(slots, "co2", hour.slotMs).map((r) => [r.time, r.value, r.note])).toEqual([
      ["14 Sept, 9:07 – 9:12 am MYT", "No data", "Gap — not joined"], ["14 Sept, 9:02 – 9:07 am MYT", "900 ppm", "1 reading"],
      ["14 Sept, 8:17 – 9:02 am MYT", "No data", "Gap — not joined"], ["14 Sept, 8:12 – 8:17 am MYT", "810 ppm", "2 readings"],
    ]);
    expect(airRows(slots, "co2", hour.slotMs, MS_TOKYO).slice(0, 2).map((r) => [r.time, r.value, r.note])).toEqual([["14 Sep, 10:07–10:12 PG GMT+9", "Tiada data", "Jurang — tidak disambung"], ["14 Sep, 10:02–10:07 PG GMT+9", "900 ppm", "1 bacaan"]]);
  });

  it("words the ventilation log and its current CO2 in the display language and time zone (IR110)", () => {
    expect([co2Now(m("co2", 1180)), co2Now(m("co2", 1180), MS_TOKYO.display), co2Now(m("co2", 1180, { quality: "stale" }))]).toEqual(["1180 ppm · 9:12 am MYT", "1180 ppm · 10:12 PG GMT+9", null]);
    const v: ApiVentilationLog = {
      id: "v1", spaceId: "s2", unitId: "u1", method: "window_opened", durationMinutes: 15, co2AtLog: { value: 1180, unit: "ppm", observedAt: "2026-09-14T01:00:00Z" }, loggedByMembershipId: "me", loggedAt: "2026-09-14T01:05:00Z",
    };
    expect(ventRow(v, "me", new Map([["u1", "Bedroom AC"]]), NOW.getTime())).toEqual({ id: "v1", text: "Window opened · 15 min · Bedroom AC", co2: "CO2 1180 ppm at 9:00 am MYT", by: "by you", when: "today 9:05 am MYT" });
    expect(ventRow({ ...v, unitId: "u9", co2AtLog: null, loggedByMembershipId: "x", loggedAt: "2026-09-13T01:05:00Z" }, "me", new Map(), NOW.getTime(), MS_TOKYO))
      .toEqual({ id: "v1", text: "Tingkap dibuka · 15 min · unit lain", co2: "CO2 tidak diukur semasa merekod", by: "oleh ahli lain", when: "semalam 10:05 PG GMT+9" });
  });
});
