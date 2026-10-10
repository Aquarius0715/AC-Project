import { describe, expect, it } from "vitest";
import { changeText, groupPlan, roomUnits, type GroupChange } from "@ac/web/lib/clientProperties";
import { i18nOf, translator } from "@ac/web/lib/i18n";
import type { ApiUnitDetail } from "@ac/web/lib/units";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Tokyo" });
const unit = (id: string, over: Partial<ApiUnitDetail> = {}): ApiUnitDetail => ({
  id, version: 1, spaceId: "room", displayName: `AC ${id}`, alertPolicyIds: [], connection: "online", effectivePowerState: "on",
  observedState: { power: true, celsius: 26, mode: "cool", fanLevel: "mid", observedAt: "2026-09-14T01:00:00Z" }, lastSeenAt: "2026-09-14T01:10:00Z",
  latestMeasurements: [
    { id: "m1", unitId: id, sensorId: "s1", metric: "temperature", value: 27.04, unit: "°C", observedAt: "2026-09-14T01:10:00Z", origin: "measured", quality: "valid", qualityReason: null },
    { id: "m2", unitId: id, sensorId: "s2", metric: "power", value: 0.68, unit: "kW", observedAt: "2026-09-14T01:10:00Z", origin: "measured", quality: "valid", qualityReason: null },
  ],
  capabilities: { manufacturer: "Demo", model: "M", control: true, modeControl: true, fanControl: true, temperature: { min: 16, max: 30, step: 1 }, modes: ["cool", "dry", "fan"], fanLevels: ["low", "mid", "high"], ventilation: false, ventilationLevels: [], sensors: [] },
  effectiveControlPolicy: { state: "unrestricted" }, controlAvailability: { state: "available" }, pendingCommands: [],
  location: { pathLabels: ["Home A", "1F", "Bedroom"], address: null, accessInstructions: null }, installedAt: null, capabilityVersion: 1, serviceScope: ["indoor"], components: [], ...over,
});
const ch: GroupChange = { power: true, celsius: 24, mode: "dry", fan: "high" };

describe("units & locations: the room list and group control (FR-C02, FR-C14, IR263)", () => {
  it("lists each AC with its setting, measured room temperature and power — in either language", () => {
    const [r] = roomUnits([unit("a")]);
    expect([r.set, r.temp, r.watts, r.power, r.seen]).toEqual(["Set 26°C · Cool · Fan mid", "27.0 °C room", "680 W", "running", "9:10 am MYT"]);
    const [m] = roomUnits([unit("a", { observedState: { power: null, celsius: null, mode: null, fanLevel: null, observedAt: null }, latestMeasurements: [] })], MS);
    expect([m.set, m.temp, m.watts, m.seen]).toEqual(["Belum ada tetapan dilaporkan", "—", "—", "10:10 PG GMT+9"]);
  });

  it("plans one command per changed setting, clamps to the model and a restriction, and skips what cannot be sent", () => {
    const rows = groupPlan([
      unit("ok"), unit("same", { observedState: { power: true, celsius: 24, mode: "dry", fanLevel: "high", observedAt: null } }),
      unit("limit", { effectiveControlPolicy: { state: "restricted", phase: "active", policy: { kind: "temperature_limit", minimumCoolingSetpoint: 25 } } }),
      unit("off", { connection: "offline" }), unit("blocked", { controlAvailability: { state: "blocked", reasonKey: "errors.reconciliation_required" } }),
    ], ch);
    expect(rows.map((r) => [r.id, r.result, r.change, r.note, r.actions.length])).toEqual([
      ["ok", "Will send", "26 → 24 °C, Dry, Fan high", "", 3],
      ["same", "No change", "already set", "", 0],
      ["limit", "Clamped", "26 → 25 °C, Dry, Fan high", "minimum 25 °C (restriction)", 3],
      ["off", "Skipped", "—", "Offline since 9:10 am MYT — not sent", 0],
      ["blocked", "Skipped", "—", "Control blocked (reconciliation required)", 0],
    ]);
    expect([changeText(ch), changeText({ ...ch, power: false }), changeText({ ...ch, fan: null })]).toEqual(["Power On · 24 °C · Dry · Fan high", "Power Off", "Power On · 24 °C · Dry · Fan unchanged"]);
  });

  it("words the plan in Malay; the result stays the plan's code", () => {
    const [ok, off] = groupPlan([unit("ok"), unit("off", { connection: "offline", lastSeenAt: null })], ch, MS);
    expect([ok.result, ok.change, off.note]).toEqual(["Will send", "26 → 24 °C, Kering, Kipas tinggi", "Luar talian — tidak dihantar"]);
    expect(changeText(ch, translator("ms"))).toBe("Kuasa Hidup · 24 °C · Kering · Kipas tinggi");
  });
});
