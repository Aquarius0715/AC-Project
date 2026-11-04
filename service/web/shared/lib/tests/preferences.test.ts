import { describe, expect, it } from "vitest";
import { consentNote, zoneOffset, zoneOptions } from "@ac/web/lib/preferences";

describe("Preferences (FR-X01, DDC-07, IR246)", () => {
  const at = new Date("2026-09-14T01:00:30Z");

  it("names each time zone with its offset from the wall-clock parts", () => {
    expect([zoneOffset("Asia/Kuala_Lumpur", at), zoneOffset("UTC", at), zoneOffset("Asia/Kolkata", at), zoneOffset("America/Sao_Paulo", at), zoneOffset("Not/AZone", at)])
      .toEqual(["UTC+8", "UTC+0", "UTC+5:30", "UTC−3", ""]);
    expect([zoneOffset("Europe/London", at), zoneOffset("Europe/London", new Date("2026-12-01T00:00:00Z"))]).toEqual(["UTC+1", "UTC+0"]);
  });

  it("lists the usual zones and keeps a saved zone that is not one of them first", () => {
    expect(zoneOptions("Asia/Tokyo", at).map((z) => z.label)).toEqual(["Asia/Kuala_Lumpur (UTC+8)", "Asia/Singapore (UTC+8)", "Asia/Jakarta (UTC+7)", "Asia/Bangkok (UTC+7)", "Asia/Tokyo (UTC+9)", "UTC (UTC+0)"]);
    expect(zoneOptions("Asia/Kolkata", at)[0]).toEqual({ id: "Asia/Kolkata", label: "Asia/Kolkata (UTC+5:30)" });
  });

  it("states when the location consent was granted or withdrawn", () => {
    const c = { id: "c1", version: 2, granted: true, grantedAt: "2026-09-14T01:00:00Z", revokedAt: null };
    expect([consentNote(c), consentNote({ ...c, granted: false, revokedAt: "2026-09-15T02:00:00Z" }), consentNote(null)]).toEqual(["Granted 2026-09-14", "Withdrawn 2026-09-15", "Not granted yet"]);
  });
});
