import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { intlTag, isLocale, showRecent, showSpan, showTime, translate, translator, zonedInstant, zonedParts } from "@ac/web/lib/i18n";
import { ms } from "@ac/web/lib/i18n-ms";
import { ROLES } from "@ac/web/lib/nav";
import { consentNote } from "@ac/web/lib/preferences";
import { byText } from "@ac/web/lib/order";
import { must } from "./must";

describe("Display language (FR-X01, AT-X01-N, IR258)", () => {
  it("keeps English as the key and falls back to it when Malay has no entry", () => {
    expect([translate("en", "Sign out"), translate("ms", "Sign out")]).toEqual(["Sign out", "Log keluar"]);
    expect(translate("ms", "A text nobody translated yet")).toBe("A text nobody translated yet");
  });

  it("tells one English text apart by its context (IR287)", () => {
    expect([translate("en", "tamper::Clear"), translate("ms", "tamper::Clear"), translate("ms", "Clear")]).toEqual(["Clear", "Tiada gangguan", "Kosongkan"]);
    expect(translate("ms", "nobody::Untranslated")).toBe("Untranslated"); // English after the context when Malay has none
  });

  it("fills {name} placeholders in both languages and leaves unknown ones as they are", () => {
    expect(translate("en", "Not saved: {reason}", { reason: "conflict" })).toBe("Not saved: conflict");
    expect(translate("ms", "Not saved: {reason}", { reason: "conflict" })).toBe("Tidak disimpan: conflict");
    expect(translate("ms", " · {n} recovery codes left", { n: 7 })).toBe(" · 7 kod pemulihan berbaki");
    expect(translate("en", "{a} and {b}", { a: "x" })).toBe("x and {b}");
  });

  it("keeps every placeholder of an English key in its Malay text", () => {
    const names = (text: string) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => must(m[1])).toSorted(byText);
    for (const [en, text] of Object.entries(ms)) expect(names(text), en).toEqual(names(en));
  });

  // NFR-07 / IR44 key check: the English texts the code translates and the dictionary's keys are the same set
  const root = resolve(__dirname, "../../..");
  const sources = (directory: string): string[] => {
    return readdirSync(directory).flatMap((f) => {
      const path = join(directory, f);
      if (["node_modules", ".next", "tests", "e2e"].includes(f)) return [];
      if (statSync(path).isDirectory()) return sources(path);
      return /\.tsx?$/.test(f) && !f.endsWith("i18n-ms.ts") ? [path] : [];
    });
  };
  const code = ["shared", "customer/app", "partner/app", "technician/app", "admin/app"].flatMap((d) => sources(join(root, d))).map((f) => readFileSync(f, "utf8")).join("\n");

  it("has a Malay text for every literal the code translates", () => {
    const literals = [
      ...code.matchAll(/\b(?:t\(|translate\(\s*[\w.]+,)\s*"((?:[^"\\]|\\.)*)"/g),
      ...code.matchAll(/<Route(?:Loading|Error) what="((?:[^"\\]|\\.)*)"/g), // the route states translate what loads (IR269)
    ].map((m) => JSON.parse(`"${m[1] ?? ""}"`) as string);
    expect(literals.length).toBeGreaterThan(100);
    expect([...new Set(literals.filter((text) => !(text in ms)))]).toEqual([]);
  });

  it("lists each English text once in the Malay dictionary (a renamed key can collide with an existing one)", () => {
    const keys = [...readFileSync(join(root, "shared/lib/i18n-ms.ts"), "utf8").matchAll(/^\s*("(?:[^"\\]|\\.)*"):/gm)].map((m) => m[1]);
    expect(keys.length).toBeGreaterThan(1000);
    expect(keys.filter((k, n) => keys.indexOf(k) !== n)).toEqual([]);
  });

  // it searches the whole code once per key: seconds of work, beyond the default 5 s while the other files run beside it
  it("has no Malay entry the code no longer shows", () => {
    const dynamic = new Set(Object.values(ROLES).flatMap((config) => [config.sub, config.chip.split(" — ", 1)[0], ...config.nav.map((i) => i.label)]));
    expect(Object.keys(ms).filter((key) => !dynamic.has(key) && !code.includes(JSON.stringify(key)))).toEqual([]);
  }, 30_000);

  it("speaks to customers, contractors, technicians and HQ without error codes or spec IDs (IR584, IR585, IR587, IR590, IR591, IR595, IR622, IR626)", () => {
    const files = ["customer/app", "partner/app", "technician/app", "admin/app", "shared/lib", "shared/components", "shared/screens"].flatMap((d) => sources(join(root, d)));
    const strings = files.flatMap((f) => {
      return [...readFileSync(f, "utf8").matchAll(/"((?:[^"\\\n]|\\.)*)"/g)].flatMap((m) => {
        const literal = `"${m[1] ?? ""}"`; // the string literal as written in the source
        try { return [JSON.parse(literal) as string]; } catch { return []; }
      });
    });
    const texts = [...new Set(strings.filter((text) => text in ms))]; // every literal the dictionary translates, maps of texts included
    expect(texts.length).toBeGreaterThan(100);
    // screen IDs as well ("Access & roles (A03)")
    const coded = /\b(?:IR\d+|(?:FR|DD|BR|NFR|DEC|AT|SCR|DDC|REV)-[A-Z]*\d+|SR\d+|D\d{2,})\b|^(?:CONFLICT|NOT_FOUND|FORBIDDEN|UNAVAILABLE|VALIDATION|TIMEOUT|OFFLINE)\b|\((?:CONFLICT|NOT_FOUND|FORBIDDEN|UNAVAILABLE|VALIDATION|TIMEOUT|OFFLINE)\)|\([ACPTX]\d{2}[a-z]?\)/;
    expect(texts.filter((text) => coded.test(text))).toEqual([]);
  });

  // IR626: a status or event code ("release_requested", "end_failed") is said in words; dotted permissions such as
  // restriction.read and {placeholders} are not codes of this kind
  it("shows no snake_case status or event code in a text the apps translate", () => {
    expect(Object.keys(ms).filter((text) => /(?<![\w./{-])[a-z]+_[a-z_]+(?![\w}])/.test(text))).toEqual([]);
  });

  // IR626: no field or operation name of the code ("previousAlertId", "jobs.saveCost") and no IR number either; units
  // such as kWh and kgCO₂e and the {placeholders} are not names of this kind
  it("shows no field name, operation name or IR number in a text the apps translate", () => {
    const named = /\b(?!kWh\b|kgCO)[a-z]+[A-Z][a-z][A-Za-z]*\b|\b[a-z]+\.[a-z]+[A-Z][A-Za-z]*\b|\bIR\d+\b/;
    expect(Object.keys(ms).filter((text) => named.test(text.replaceAll(/\{\w+\}/g, "")))).toEqual([]);
  });

  // IR624: the apps speak English and Malay; a Japanese term of the specification ("受領") once reached the partner's schedule
  it("shows no Japanese term in a text the apps translate", () => {
    expect(Object.keys(ms).filter((text) => /[\u3040-\u30FF\u4E00-\u9FFF]/.test(text))).toEqual([]);
  });

  it("names every sidebar item and the role words of the four apps in Malay", () => {
    for (const config of Object.values(ROLES)) {
      for (const item of config.nav.filter((i) => i.label !== "MRV")) expect(ms[item.label], item.label).toBeTruthy();
      expect(ms[config.sub], config.sub).toBeTruthy();
      const word = must(config.chip.split(" — ", 1)[0]); // the role chip of the header
      expect(ms[word], word).toBeTruthy();
    }
  });

  it("puts the consent line in the display language", () => {
    const c = { id: "c1", version: 2, granted: true, grantedAt: "2026-09-14T01:00:00Z", revokedAt: null };
    expect([consentNote(c, translator("ms")), consentNote(null, translator("ms"))]).toEqual(["Diberikan 2026-09-14", "Belum diberikan"]);
  });

  it("accepts only the two locales and formats both for Malaysia", () => {
    expect([isLocale("en"), isLocale("ms"), isLocale("ja"), isLocale(null)]).toEqual([true, true, false, false]);
    expect([intlTag("en"), intlTag("ms")]).toEqual(["en-MY", "ms-MY"]);
  });

  it("reads a typed date and time in the display time zone and prefills it back (NFR-08)", () => {
    expect([zonedInstant("2026-09-16", "10:00", "Asia/Kuala_Lumpur"), zonedInstant("2026-09-16", "10:00", "Asia/Tokyo"), zonedInstant("2026-09-16", "10:00", "UTC")])
      .toEqual(["2026-09-16T02:00:00.000Z", "2026-09-16T01:00:00.000Z", "2026-09-16T10:00:00.000Z"]);
    expect([zonedInstant("2026-09-16", "", "Asia/Tokyo"), zonedInstant("", "10:00", "Asia/Tokyo"), zonedInstant("2026-02-31", "10:00", "UTC")]).toEqual(["", "", "2026-03-03T10:00:00.000Z"]);
    expect(zonedInstant("2026-09-16", "10:00", "Not/AZone")).toBe("2026-09-16T02:00:00.000Z"); // an unknown zone falls back to Kuala Lumpur
    expect([zonedParts("2026-09-16T01:00:00Z", "Asia/Tokyo"), zonedParts("2026-09-15T23:30:00Z", "Asia/Kuala_Lumpur")]).toEqual([{ date: "2026-09-16", time: "10:00" }, { date: "2026-09-16", time: "07:30" }]);
  });

  it("shows a span in one line, with plain spaces whatever the ICU (no hydration difference)", () => {
    const span = showSpan("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z", { locale: "ms", timeZone: "Asia/Tokyo" }, true);
    expect(span).toBe("Rab, 16 Sep, 11:00 PG – 1:00 PTG GMT+9");
    expect(/[\u00A0\u2009\u202F]/.test(span + showTime("2026-09-16T02:00:00Z"))).toBe(false);
    expect(showSpan("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z")).toBe("14 Sept, 8:00 am – 20 Sept, 8:00 am MYT");
  });

  it("shows a time in the user's language and time zone with the zone's abbreviation (IR44)", () => {
    const at = "2026-09-14T01:00:00Z";
    expect([showTime(at), showTime(at, { locale: "ms", timeZone: "Asia/Kuala_Lumpur" }), showTime(at, { locale: "en", timeZone: "Asia/Tokyo" }), showTime(at, { locale: "en", timeZone: "UTC" })])
      .toEqual(["14 Sept 2026, 9:00 am MYT", "14 Sep 2026, 9:00 PG MYT", "14 Sept 2026, 10:00 am GMT+9", "14 Sept 2026, 1:00 am UTC"]);
  });
  it("shows a recent time alone on today and with its date on another day (IR613)", () => {
    const at = "2026-09-14T00:30:00Z"; // 8:30 am in Kuala Lumpur
    expect([showRecent(at, Date.parse("2026-09-14T10:00:00Z")), showRecent(at, Date.parse("2026-09-23T06:00:00Z")), showRecent(at, Date.parse("2026-09-13T17:00:00Z")), showRecent(null, 0)])
      .toEqual(["8:30 am MYT", "14 Sept 2026, 8:30 am MYT", "8:30 am MYT", "—"]); // 13 Sept 17:00 UTC is already the 14th in Kuala Lumpur
    expect(showRecent(at, Date.parse("2026-09-14T15:30:00Z"), { locale: "en", timeZone: "Asia/Tokyo" })).toBe("14 Sept 2026, 9:30 am GMT+9"); // the 15th in Tokyo
  });
});
