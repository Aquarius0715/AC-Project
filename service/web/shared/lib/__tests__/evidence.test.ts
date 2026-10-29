import { describe, expect, it } from "vitest";
import { translator } from "@ac/web/lib/i18n";
import { ms } from "@ac/web/lib/i18n-ms";
import { EVIDENCE_KEYS, evidenceText } from "@ac/web/lib/evidence";

describe("evidence texts in the display language (IR480)", () => {
  const en = translator("en"), malay = translator("ms");

  it("has a Malay text for every evidence text the sources write", () => {
    expect(EVIDENCE_KEYS.filter((key) => !(key in ms))).toEqual([]);
  });

  it("keeps a stored English text in English and translates it into Malay", () => {
    expect(evidenceText("Demo: cooling load rose while the outdoor temperature was stable.", en)).toBe("Demo: cooling load rose while the outdoor temperature was stable.");
    expect(evidenceText("Demo: cooling load rose while the outdoor temperature was stable.", malay)).toBe(ms["Demo: cooling load rose while the outdoor temperature was stable."]);
  });

  it("keeps the values inside a text: hours, days, a serial, a metric", () => {
    expect(evidenceText("Cleaning due (54 days since the last cleaning, run time unknown). Not a fault.", malay)).toContain("54");
    expect(evidenceText("Cleaning due (54 days since the last cleaning, run time unknown). Not a fault.", en)).toBe("Cleaning due (54 days since the last cleaning, run time unknown). Not a fault.");
    expect(evidenceText("Cleaning due (310 h of run time since the last cleaning). Not a fault.", malay)).toContain("310");
    expect(evidenceText("Device SN-0042 reported its cover opened while powered (tamper_signal)", malay)).toContain("SN-0042");
    expect(evidenceText("Demo event: temperature gte threshold", malay)).toContain("temperature gte");
  });

  it("leaves a text no source writes as stored", () => {
    expect(evidenceText("A note from a device", malay)).toBe("A note from a device");
  });
});
