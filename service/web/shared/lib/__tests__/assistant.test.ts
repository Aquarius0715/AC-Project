import { describe, expect, it } from "vitest";
import { candidateGroups, changeCard, examples, groupSummary, helpText, placeOf, progressOf, roomOf, stateText, temperatureText, type ResolvedIntent } from "@ac/web/lib/assistant";
import { DEFAULT_DISPLAY, translator } from "@ac/web/lib/i18n";
import type { ApiCommand, ApiUnitDetail } from "@ac/web/lib/units";

const en = translator("en"), ms = translator("ms");
const TOKYO = { locale: "ms" as const, timeZone: "Asia/Tokyo" };
// the fixed grammar of voice.resolveIntent (D09; service/api/internal/modules/voice): the suggestions must parse
const grammar = {
  en: [/^temperature (.+)$/i, /^set (.+) to (\d{1,3}) degrees$/i, /^help$/i],
  ms: [/^suhu (.+)$/i, /^tetapkan (.+) kepada (\d{1,3}) darjah$/i, /^bantuan$/i],
};
const unit = (over: Partial<ApiUnitDetail> = {}) => ({
  displayName: "Bedroom AC", location: { pathLabels: ["Home A", "1F", "Bedroom"], address: null, accessInstructions: null }, capabilities: { temperature: { min: 16, max: 30, step: 1 } },
  ...over,
}) as unknown as ApiUnitDetail;
const change = (celsius: number, before: number | null = 26): Extract<ResolvedIntent, { kind: "change" }> => ({ kind: "change", unitId: "u-0001-aaaa", celsius, before: { power: true, celsius: before, observedAt: null }, expectedVersion: 7 });
const command = (status: ApiCommand["status"], failureCode: string | null = null, acknowledgedAt: string | null = null): ApiCommand =>
  ({ id: "cmd-0240-aaaa", action: { kind: "set_temperature", celsius: 24 }, status, requestedAt: "2026-09-15T01:45:01Z", acknowledgedAt, expiresAt: "2026-09-15T01:45:31Z", failureCode });

describe("the customer assistant (FR-X02, D09, Figma Client 09a–09g)", () => {
  it("suggests only sentences the fixed grammar accepts, in each language", () => {
    for (const locale of ["en", "ms"] as const) {
      const ex = examples(locale);
      expect([ex.temperature, ex.change, ex.help].map((s, i) => grammar[locale][i].test(s))).toEqual([true, true, true]);
    }
    expect(examples("ms").change).toBe("tetapkan Bedroom kepada 24 darjah");
    expect([helpText(en).startsWith("Ask “temperature <room>”"), helpText(ms).startsWith("Tanya “suhu <bilik>”")]).toEqual([true, true]); // the help speaks its language's grammar
  });

  it("names the target with its place and answers a temperature question without inventing a zero", () => {
    expect(placeOf(unit())).toBe("Home A › 1F › Bedroom › Bedroom AC");
    expect([
      temperatureText(null, "Bedroom AC", en, DEFAULT_DISPLAY), temperatureText({ value: null, unit: "°C", observedAt: "2026-09-15T01:12:00Z", quality: "missing" }, "Bedroom AC", en, DEFAULT_DISPLAY),
      temperatureText({ value: 28, unit: "°C", observedAt: "2026-09-15T01:12:00Z", quality: "valid" }, "Bedroom AC", en, DEFAULT_DISPLAY),
      temperatureText({ value: 27.95, unit: "°C", observedAt: "2026-09-15T01:12:00Z", quality: "stale" }, "Bedroom AC", en, DEFAULT_DISPLAY),
    ]).toEqual(["Bedroom AC has no temperature reading yet.", "Bedroom AC has no temperature reading yet.", "Bedroom AC is 28.0 °C (measured 9:12 am MYT).", "Bedroom AC last read 28.0 °C at 9:12 am MYT — the reading is stale."]);
  });

  it("confirms a change with the current setting and the allowed range before anything is sent", () => {
    expect(changeCard(change(24), unit(), en)).toEqual({ target: "Home A › 1F › Bedroom › Bedroom AC", unit: "Bedroom AC", current: "26 °C (confirmed by the device)", requested: "24 °C", range: "16–30 °C · step 1", supported: true });
    expect(changeCard(change(31), unit(), en)).toMatchObject({ supported: false });
    expect(changeCard(change(24, null), null, en)).toMatchObject({ target: "u-0001-a", current: "not known", range: "not supported", supported: false });
  });

  it("asks for the room, then the AC, without choosing for the user (Figma 09d, IR101)", () => {
    const groups = candidateGroups([
      { unitId: "u-online-rto", pathLabel: "Home A > 1F > Bedroom > Bedroom AC" },
      { unitId: "u-non-rto-x", pathLabel: "Home A > 1F > Bedroom > Bedroom AC" },
      { unitId: "u-office-aa", pathLabel: "Office A > 2F > Bedroom > Office Bedroom AC" },
    ]);
    expect(groups).toEqual([
      { path: "Home A › 1F › Bedroom", units: [{ unitId: "u-online-rto", name: "Bedroom AC", label: "Bedroom AC · u-online" }, { unitId: "u-non-rto-x", name: "Bedroom AC", label: "Bedroom AC · u-non-rt" }] },
      { path: "Office A › 2F › Bedroom", units: [{ unitId: "u-office-aa", name: "Office Bedroom AC", label: "Office Bedroom AC" }] },
    ]);
    expect([roomOf(groups), groupSummary(groups[0], en), groupSummary(groups[1], en), groupSummary(groups[0], ms)])
      .toEqual(["Bedroom", "2 ACs: Bedroom AC · u-online, Bedroom AC · u-non-rt", "1 AC: Office Bedroom AC", "2 AC: Bedroom AC · u-online, Bedroom AC · u-non-rt"]);
    // different names in one room need no ID; a top-level room keeps its property in the path
    expect(candidateGroups([{ unitId: "a", pathLabel: "Home A > Bedroom > Bedroom AC" }, { unitId: "b", pathLabel: "Home A > Bedroom > Bedroom AC #2" }]))
      .toEqual([{ path: "Home A › Bedroom", units: [{ unitId: "a", name: "Bedroom AC", label: "Bedroom AC" }, { unitId: "b", name: "Bedroom AC #2", label: "Bedroom AC #2" }] }]);
    expect(roomOf([])).toBe("");
  });

  it("follows the command until the device answers (Figma 09e–09g)", () => {
    expect(progressOf(command("sent"), "Bedroom AC", 24, 26, null, en, DEFAULT_DISPLAY)).toEqual({
      tone: "info", title: "Sending to Bedroom AC…", rows: [["Command", "cmd-0240 · Set temperature 24°C"], ["Status", "Sent 9:45 am MYT · waiting for the device"]],
      detail: "The confirmed setting stays 26 °C until the device acknowledges.",
    });
    expect(progressOf(command("sent"), "Bedroom AC", 24, null, null, en, DEFAULT_DISPLAY).detail).toBe("The confirmed setting stays as it is until the device acknowledges.");
    expect(progressOf(command("acknowledged", null, "2026-09-15T01:45:03Z"), "Bedroom AC", 24, 26, 28, en, DEFAULT_DISPLAY)).toEqual({ tone: "ok", title: "Done — Bedroom AC is set to 24 °C", detail: "Acknowledged by the device at 9:45 am MYT. Room temperature is still 28.0 °C (measured)." });
    expect(progressOf(command("acknowledged"), "Bedroom AC", 24, 26, null, en, DEFAULT_DISPLAY).detail).toBe("Acknowledged by the device.");
    expect(progressOf(command("expired"), "Bedroom AC", 24, 26, null, en, DEFAULT_DISPLAY)).toEqual({ tone: "crit", title: "Couldn’t change Bedroom AC", detail: "The device did not answer within 30 s (expired). Nothing changed — the confirmed setting is still 26 °C." });
    expect(progressOf({ ...command("expired"), expiresAt: undefined }, "Bedroom AC", 24, 26, null, en, DEFAULT_DISPLAY).detail).toBe("The device did not answer in time (expired). Nothing changed — the confirmed setting is still 26 °C.");
    expect(progressOf(command("failed", "OFFLINE"), "Bedroom AC", 24, null, null, en, DEFAULT_DISPLAY).detail).toBe("The device refused it (OFFLINE). Nothing changed.");
    expect(stateText("confirm", en)).toBe("Needs confirmation");
  });
});

describe("the customer assistant in Malay with the display time zone (IR306)", () => {
  it("words the answers, the confirmation and the progress", () => {
    expect(temperatureText({ value: 28, unit: "°C", observedAt: "2026-09-15T01:12:00Z", quality: "valid" }, "Bedroom AC", ms, TOKYO)).toBe("Bedroom AC ialah 28.0 °C (diukur 10:12 PG GMT+9).");
    expect(changeCard(change(24), unit(), ms)).toMatchObject({ current: "26 °C (disahkan oleh peranti)", range: "16–30 °C · langkah 1" });
    expect(progressOf(command("acknowledged"), "Bedroom AC", 24, 26, null, ms, TOKYO).title).toBe("Selesai — Bedroom AC ditetapkan kepada 24 °C");
    expect([stateText("idle", ms), stateText("sending", ms), stateText("success", ms)]).toEqual(["Melahu", "Menghantar", "Berjaya"]);
  });
});
