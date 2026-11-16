// The customer's away/home presence (DEC-109, IR826): when the browser reports again, the result's words, and the
// result checked where it enters the browser.
import { describe, expect, it } from "vitest";
import { parsePresenceResult, presenceText, shouldReport, type Fix } from "@ac/web/lib/presence";
import { haversine } from "@ac/web/lib/geoPoint";
import { translator } from "@ac/web/lib/i18n";

const t = translator("en");
const at = (latitude: number, accuracyMeters: number, minute: number): Fix => ({ latitude, accuracyMeters, longitude: 101.7116, at: minute * 60_000 });
const HOME = "p-home";
const names = new Map([[HOME, "Home A"]]);
const result = (event: "arrival" | "departure", n: number) => ({ transitions: [{ event, propertyId: HOME, commandIds: Array.from({ length: n }, (_, i) => `c${String(i)}`) }] });

describe("presence reports", () => {
  it("measures the great-circle distance as the Core API does", () => {
    expect(Math.round(haversine({ latitude: 3.1579, longitude: 101.7116 }, { latitude: 3.1597, longitude: 101.7116 }))).toBe(200);
    expect(Math.round(haversine({ latitude: 3.139, longitude: 101.6869 }, { latitude: 1.3521, longitude: 103.8198 }))).toBe(309_253);
  });
  it("sends the first precise fix, then after 50 m or 5 minutes, and never a coarse one", () => {
    expect(shouldReport(null, at(3.1579, 20, 0))).toBe(true);
    expect(shouldReport(null, at(3.1579, 120, 0))).toBe(false);
    const last = at(3.1579, 20, 0);
    expect(shouldReport(last, at(3.1582, 20, 1))).toBe(false); // 33 m, a minute later
    expect(shouldReport(last, at(3.1584, 20, 1))).toBe(true); // 56 m
    expect(shouldReport(last, at(3.1579, 20, 5))).toBe(true); // the same place, 5 minutes later
  });
  it("words an arrival or a departure with the automations it ran, and nothing for no change", () => {
    expect(presenceText(result("arrival", 0), names, t)).toBe("Arrived at Home A.");
    expect(presenceText(result("arrival", 1), names, t)).toBe("Arrived at Home A — 1 automation ran.");
    expect(presenceText(result("departure", 2), names, t)).toBe("Left Home A — 2 automations ran.");
    expect(presenceText({ transitions: [] }, names, t)).toBeNull();
    expect(presenceText({ transitions: [{ propertyId: "other", event: "departure", commandIds: [] }] }, names, t)).toBe("Left your property.");
  });
  it("takes only a PresenceResult from the Core API", () => {
    expect(parsePresenceResult({ transitions: [{ propertyId: HOME, event: "arrival", commandIds: [] }] })).not.toBeNull();
    expect(parsePresenceResult({ transitions: [{ propertyId: HOME, event: "teleport", commandIds: [] }] })).toBeNull();
    expect(parsePresenceResult(null)).toBeNull();
  });
});
