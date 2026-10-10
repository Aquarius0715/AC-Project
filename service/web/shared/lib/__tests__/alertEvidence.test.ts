import { describe, expect, it } from "vitest";
import { evidenceHeading, evidenceProblem, evidenceRefusal, evidenceRows, MAX_EVIDENCE, togglePick, type ApiEvidenceCandidate } from "@ac/web/lib/alertEvidence";
import { EN, i18nOf } from "@ac/web/lib/i18n";

const MS = i18nOf({ locale: "ms", timeZone: "Asia/Kuala_Lumpur" });
const NOW = Date.parse("2026-09-14T01:35:00Z"); // 09:35 in Kuala Lumpur
const reading = (id: string, metric: string, value: number | null, unit: string, at: string, origin: ApiEvidenceCandidate["origin"] = "measured"): ApiEvidenceCandidate =>
  ({ id, kind: "remeasurement", observedAt: at, metric, value, unit, origin, quality: "valid", eventType: null });
const items: ApiEvidenceCandidate[] = [
  reading("m-temp", "temperature", 26.4, "°C", "2026-09-14T01:20:00Z"),
  reading("m-co2", "co2", 650, "ppm", "2026-09-14T01:15:00Z", "inspection"),
  { id: "e-restored", kind: "device_event", observedAt: "2026-09-14T01:12:00Z", metric: null, value: null, unit: null, origin: null, quality: null, eventType: "restored" },
  { id: "0b6c2f4e-0000-4000-8000-000000000001", kind: "detection", observedAt: "2026-09-13T23:50:00Z", metric: null, value: null, unit: null, origin: null, quality: null, eventType: null },
];

describe("alert resolution evidence (IR327)", () => {
  it("lists each candidate with what it is, when, and where it comes from", () => {
    expect(evidenceRows(items, NOW)).toEqual([
      { id: "m-temp", kind: "remeasurement", title: "Remeasurement · Temperature 26.4 °C · today 9:20 am MYT", sub: "measured · valid" },
      { id: "m-co2", kind: "remeasurement", title: "Remeasurement · CO₂ 650 ppm · today 9:15 am MYT", sub: "recorded on site · valid" }, // the label's “(ppm)” left to the value
      { id: "e-restored", kind: "device_event", title: "Device event · today 9:12 am MYT", sub: "The device's signal is back" },
      { id: "0b6c2f4e-0000-4000-8000-000000000001", kind: "detection", title: "Evidence attached to this alert", sub: "0b6c2f4e · observed today 7:50 am MYT" },
    ]);
    expect(evidenceRows([reading("m-x", "refrigerant_pressure", null, "kPa", "2026-09-12T01:20:00Z")], NOW)[0].title).toBe("Remeasurement · Refrigerant pressure — · 12 Sept 2026, 9:20 am MYT");
  });

  it("words the rows in the display language", () => {
    const [r] = evidenceRows(items.slice(0, 1), NOW, MS);
    expect([r.title.startsWith("Pengukuran semula · "), r.sub]).toEqual([true, "diukur · sah"]);
    expect(evidenceHeading(true, MS.t).title).toBe("Bukti penyelesaian · wajib");
  });

  it("requires evidence without a policy and leaves it optional with one (IR66)", () => {
    expect(evidenceHeading(true, EN.t)).toEqual({ title: "Resolution evidence · required", sub: "At least one · measurements after detection" });
    expect(evidenceHeading(false, EN.t)).toEqual({ title: "Resolution evidence · optional", sub: "A policy alert may also be resolved by hand without evidence" });
    expect(evidenceProblem(true, [], EN.t)).toBe("Pick at least one evidence record — this alert has no policy");
    expect([evidenceProblem(true, ["m-temp"], EN.t), evidenceProblem(false, [], EN.t)]).toEqual([null, null]);
    expect(evidenceProblem(false, Array.from({ length: MAX_EVIDENCE + 1 }, (_, i) => `m${i}`), EN.t)).toBe("Pick at most 20 evidence records");
  });

  it("says why the Core API refused the evidence, and nothing for another refusal", () => {
    expect(evidenceRefusal({ resolutionEvidenceIds: "errors.evidence_required" }, EN.t)).toBe("Pick at least one evidence record — this alert has no policy");
    expect(evidenceRefusal({ resolutionEvidenceIds: "errors.evidence_unknown" }, EN.t)).toBe("An evidence record is no longer a candidate of this alert — pick again from the list");
    expect(evidenceRefusal({ resolutionEvidenceIds: "error.invalid" }, EN.t)).toBe("Pick each evidence record once, at most 20");
    expect([evidenceRefusal({ resolutionReason: "error.length" }, EN.t), evidenceRefusal(undefined, EN.t)]).toEqual([null, null]);
  });

  it("ticks a candidate on once and off again, in the order picked", () => {
    expect(togglePick(togglePick(["a"], "b", true), "b", true)).toEqual(["a", "b"]);
    expect(togglePick(["a", "b"], "a", false)).toEqual(["b"]);
  });
});
