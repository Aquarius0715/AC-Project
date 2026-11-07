import { describe, expect, it } from "vitest";
import { translator } from "@ac/web/lib/i18n";
import { ms } from "@ac/web/lib/i18n-ms";
import { EVIDENCE_KEYS, evidenceText, resolutionText } from "@ac/web/lib/evidence";

const DEMO_COOLING_LOAD_ROSE_WHILE_THE_OUTDOOR = "Demo: cooling load rose while the outdoor temperature was stable.";
const CLEANING_DUE_54_DAYS_SINCE_THE_LAST_CLEA = "Cleaning due (54 days since the last cleaning, run time unknown). Not a fault.";

describe("evidence texts and resolution reasons in the display language (IR480, IR481)", () => {
  const en = translator("en"), malay = translator("ms");

  it("has a Malay text for every evidence text the sources write", () => {
    expect(EVIDENCE_KEYS.filter((key) => !Object.hasOwn(ms, key))).toStrictEqual([]);
  });

  it("keeps a stored English text in English and translates it into Malay", () => {
    expect(evidenceText(DEMO_COOLING_LOAD_ROSE_WHILE_THE_OUTDOOR, en)).toBe(DEMO_COOLING_LOAD_ROSE_WHILE_THE_OUTDOOR);
    expect(evidenceText(DEMO_COOLING_LOAD_ROSE_WHILE_THE_OUTDOOR, malay)).toBe(ms["Demo: cooling load rose while the outdoor temperature was stable."]);
  });

  it("keeps the values inside a text: hours, days, a serial, a metric", () => {
    expect(evidenceText(CLEANING_DUE_54_DAYS_SINCE_THE_LAST_CLEA, malay)).toContain("54");
    expect(evidenceText(CLEANING_DUE_54_DAYS_SINCE_THE_LAST_CLEA, en)).toBe(CLEANING_DUE_54_DAYS_SINCE_THE_LAST_CLEA);
    expect(evidenceText("Cleaning due (310 h of run time since the last cleaning). Not a fault.", malay)).toContain("310");
    expect(evidenceText("Device SN-0042 reported its cover opened while powered (tamper_signal)", malay)).toContain("SN-0042");
    // the stored text keeps the server's event name; the screen says it in words (IR626)
    expect(evidenceText("Device SN-0042 reported its cover opened while powered (tamper_signal)", en)).toBe("Device SN-0042 reported its cover opened while powered");
    expect(evidenceText("Demo event: temperature gte threshold", malay)).toContain("temperature gte");
  });

  it("leaves a text no source writes as stored", () => {
    expect(evidenceText("A note from a device", malay)).toBe("A note from a device");
  });

  it("words the resolution reasons the system writes and keeps a person's own (IR481)", () => {
    expect(resolutionText("filter cleaned", malay)).toBe("penapis dibersihkan");
    expect(resolutionText("recovery resolved", en)).toBe("recovery resolved");
    expect(resolutionText("Recovered: temperature 24.5 < 26 for 300 s (D08)", malay)).toBe("Pulih: temperature 24.5 < 26 selama 300 s");
    // the stored reason keeps the server's "(D08)"; the screen shows it without the spec ID, in English too (IR626)
    expect(resolutionText("Recovered: temperature 24.5 < 26 for 300 s (D08)", en)).toBe("Recovered: temperature 24.5 < 26 for 300 s");
    expect(resolutionText("Customer confirmed the window was shut", malay)).toBe("Customer confirmed the window was shut");
    expect(resolutionText("Cleaning due. Not a fault.", malay)).toBe("Cleaning due. Not a fault."); // an evidence text is not a reason
  });
});
