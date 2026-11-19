import { describe, expect, it } from "vitest";
import { translator } from "@ac/web/lib/i18n";
import { ms } from "@ac/web/lib/i18n-ms";
import { ja } from "@ac/web/lib/i18n-ja";
import { id } from "@ac/web/lib/i18n-id";
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
    const NO_CLEANING = "Cleaning due (420 h of run time, no cleaning recorded). Not a fault.";
    expect(evidenceText(NO_CLEANING, en)).toBe(NO_CLEANING);
    expect(evidenceText(NO_CLEANING, malay)).toBe(ms["Cleaning due ({hours} h of run time, no cleaning recorded). Not a fault."]?.replace("{hours}", "420"));
    expect(evidenceText("Device SN-0042 reported its cover opened while powered (tamper_signal)", malay)).toContain("SN-0042");
    // the stored text keeps the server's event name; the screen says it in words (IR626)
    expect(evidenceText("Device SN-0042 reported its cover opened while powered (tamper_signal)", en)).toBe("Device SN-0042 reported its cover opened while powered");
    expect(evidenceText("Demo event: temperature gte threshold", malay)).toBe("Peristiwa demo: Suhu pada atau melebihi ambang amarannya"); // IR812: the metric and the comparison in words
    expect(evidenceText("Demo event: heartbeat_gap gte threshold", en)).toBe("Demo event: Heartbeat gap at or above its alert threshold");
  });

  it("has a Japanese and an Indonesian text for every evidence text the sources write", () => {
    expect(EVIDENCE_KEYS.filter((key) => !Object.hasOwn(ja, key))).toStrictEqual([]);
    expect(EVIDENCE_KEYS.filter((key) => !Object.hasOwn(id, key))).toStrictEqual([]);
  });

  it("words the restriction-bypass alert for each fault and names the restriction in words (DEC-110, IR847)", () => {
    const japanese = translator("ja");
    const tail = " while the unit is under a power_off restriction: the restriction may no longer hold";
    expect(evidenceText(`Device SN-0042 stopped reporting${tail}`, en)).toBe("Device SN-0042 stopped reporting while the unit is under a restriction (Power off): the restriction may no longer hold");
    expect(evidenceText(`Device SN-0042 lost power${tail}`, japanese)).toBe("デバイス SN-0042 の電源が切れました。ユニットに制限（電源オフ）がかかっているため、制限が保たれていないおそれがあります");
    expect(evidenceText("Device SN-0042 reported its cover opened while the unit is under a temperature_limit restriction: the restriction may no longer hold", malay)).toBe("Peranti SN-0042 melaporkan penutupnya dibuka semasa unit di bawah sekatan (Had suhu): sekatan itu mungkin tidak lagi berkuat kuasa");
    // a restriction kind with no word stays as sent
    expect(evidenceText("Device SN-0042 lost power while the unit is under a new_kind restriction: the restriction may no longer hold", en)).toBe("Device SN-0042 lost power while the unit is under a restriction (new_kind): the restriction may no longer hold");
  });

  it("leaves a text no source writes as stored", () => {
    expect(evidenceText("A note from a device", malay)).toBe("A note from a device");
  });

  it("words the resolution reasons the system writes and keeps a person's own (IR481)", () => {
    expect(resolutionText("filter cleaned", malay)).toBe("penapis dibersihkan");
    expect(resolutionText("recovery resolved", en)).toBe("recovery resolved");
    expect(resolutionText("Recovered: temperature 24.5 < 26 for 300 s (D08)", malay)).toBe("Pulih: Suhu 24.5 < 26 selama 300 s");
    // the stored reason keeps the server's "(D08)" and metric code; the screen shows neither, in English too (IR626)
    expect(resolutionText("Recovered: temperature 24.5 < 26 for 300 s (D08)", en)).toBe("Recovered: Temperature 24.5 < 26 for 300 s");
    expect(resolutionText("Recovered: refrigerant_pressure 1.9 > 1.5 for 300 s (D08)", en)).toBe("Recovered: Refrigerant pressure 1.9 > 1.5 for 300 s");
    expect(resolutionText("Recovered: unknown_metric 3 > 1 for 60 s (D08)", en)).toBe("Recovered: unknown_metric 3 > 1 for 60 s"); // a metric with no word stays as sent
    expect(resolutionText("Customer confirmed the window was shut", malay)).toBe("Customer confirmed the window was shut");
    expect(resolutionText("Cleaning due. Not a fault.", malay)).toBe("Cleaning due. Not a fault."); // an evidence text is not a reason
  });
});
