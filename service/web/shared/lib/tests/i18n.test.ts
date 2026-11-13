import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { DICTIONARIES, intlTag, isLocale, showRecent, showSpan, showTime, translate, translator, zonedInstant, zonedParts, type Locale } from "@ac/web/lib/i18n";
import { id } from "@ac/web/lib/i18n-id";
import { ja } from "@ac/web/lib/i18n-ja";
import { ms } from "@ac/web/lib/i18n-ms";
import { ordinal } from "@ac/web/lib/partnerJobDetail";
import { ROLES } from "@ac/web/lib/nav";
import { consentNote } from "@ac/web/lib/preferences";
import { byText } from "@ac/web/lib/order";
import { must } from "./must";
import { entryOf, lookup } from "@ac/web/lib/lookup";

const TEXT_2026_09_16 = "2026-09-16";
const ASIA_KUALA_LUMPUR = "Asia/Kuala_Lumpur";
const ASIA_TOKYO = "Asia/Tokyo";

/** A JSON text read, or null when it is not JSON. */
function parsed(json: string): unknown {
  try {
    return JSON.parse(json);
  } catch {
    return null;
  }
}
/** The text a string literal stands for, or null when it is not one JSON reads (a template or a regex part). */
function literalText(literal: string): string | null {
  const value = parsed(literal);
  return typeof value === "string" ? value : null;
}
/** The TypeScript sources under a directory, without dependencies, builds and tests. */
const sources = (directory: string): string[] => readdirSync(directory, { recursive: true, encoding: "utf8" })
  .filter((f) => f.split(path.sep).every((part) => !["node_modules", ".next", "tests", "e2e"].includes(part)) && /\.tsx?$/.test(f) && !f.endsWith("i18n-ms.ts") && !f.endsWith("i18n-ja.ts") && !f.endsWith("i18n-id.ts"))
  .map((f) => path.join(directory, f));

const names = (text: string) => text.matchAll(/\{(\w+)\}/g).map((m) => must(m[1])).toArray().toSorted(byText);

/** The dictionaries every check below runs on: each language that is not English (IR639). */
const LANGUAGES: [Exclude<Locale, "en">, string, Readonly<Record<string, string>>][] = [["ms", "Malay", ms], ["ja", "Japanese", ja], ["id", "Indonesian", id]];

describe("Display language (FR-X01, AT-X01-N, IR258, IR639)", () => {
  it("keeps English as the key and falls back to it when Malay has no entry", () => {
    expect([translate("en", "Sign out"), translate("ms", "Sign out")]).toStrictEqual(["Sign out", "Log keluar"]);
    expect(translate("ms", "A text nobody translated yet")).toBe("A text nobody translated yet");
  });

  it("tells one English text apart by its context (IR287)", () => {
    expect([translate("en", "tamper::Clear"), translate("ms", "tamper::Clear"), translate("ms", "Clear")]).toStrictEqual(["Clear", "Tiada gangguan", "Kosongkan"]);
    expect(translate("ms", "nobody::Untranslated")).toBe("Untranslated"); // English after the context when Malay has none
  });

  it("fills {name} placeholders in both languages and leaves unknown ones as they are", () => {
    expect(translate("en", "Not saved: {reason}", { reason: "conflict" })).toBe("Not saved: conflict");
    expect(translate("ms", "Not saved: {reason}", { reason: "conflict" })).toBe("Tidak disimpan: conflict");
    expect(translate("ms", " · {n} recovery codes left", { n: 7 })).toBe(" · 7 kod pemulihan berbaki");
    expect(translate("en", "{a} and {b}", { a: "x" })).toBe("x and {b}");
  });

  it("has the same English keys, in the same order, in every dictionary", () => {
    expect(Object.keys(ja)).toStrictEqual(Object.keys(ms));
    expect(Object.keys(id)).toStrictEqual(Object.keys(ms));
  });

  describe.each(LANGUAGES)("the %s dictionary (%s)", (locale, _name, dictionary) => {
    it("keeps every placeholder of an English key in its text", () => {
      for (const [en, text] of Object.entries(dictionary)) expect(names(text), en).toStrictEqual(names(en));
    });
    it("is the dictionary translate() reads", () => {
      expect(entryOf(DICTIONARIES, locale)).toBe(dictionary);
    });
  });

  // NFR-07 / IR44 key check: the English texts the code translates and the dictionary's keys are the same set
  const root = path.resolve(import.meta.dirname, "../../..");
  const code = ["shared", "customer/app", "partner/app", "technician/app", "admin/app"].flatMap((d) => sources(path.join(root, d))).map((f) => readFileSync(f, "utf8")).join("\n");

  it.each(LANGUAGES)("has a %s text for every literal the code translates", (_locale, _name, dictionary) => {
    const literals = [
      ...code.matchAll(/\b(?:t\(|translate\(\s*[\w.]+,)\s*"((?:[^"\\]|\\.)*)"/g),
      ...code.matchAll(/<Route(?:Loading|Error) what="((?:[^"\\]|\\.)*)"/g), // the route states translate what loads (IR269)
    ].map((m) => String(JSON.parse(`"${m[1] ?? ""}"`)));
    expect(literals.length).toBeGreaterThan(100);
    expect([...new Set(literals.filter((text) => !Object.hasOwn(dictionary, text)))]).toStrictEqual([]);
  });

  it.each(["ms", "ja", "id"])("lists each English text once in the %s dictionary (a renamed key can collide with an existing one)", (locale) => {
    const keys = readFileSync(path.join(root, `shared/lib/i18n-${locale}.ts`), "utf8").matchAll(/^\s*("(?:[^"\\]|\\.)*"):/gm).map((m) => m[1]).toArray();
    expect(keys.length).toBeGreaterThan(1000);
    expect(keys.filter((k, n) => keys.indexOf(k) !== n)).toStrictEqual([]);
  });

  // it searches the whole code once per key: seconds of work, beyond the default 5 s while the other files run beside it
  it("has no entry the code no longer shows (the dictionaries share their keys)", () => {
    const dynamic = new Set(Object.values(ROLES).flatMap((config) => [config.sub, config.chip.split(" — ", 1)[0], ...config.nav.map((i) => i.label)]));
    expect(Object.keys(ms).filter((key) => !dynamic.has(key) && !code.includes(JSON.stringify(key)))).toStrictEqual([]);
  }, 30_000);

  // IR639: a Japanese text is Japanese; only names and formats that read the same in every language stay as written
  it("leaves in English only the Japanese texts that read the same in every language", () => {
    const same = Object.entries(ja).filter(([key, text]) => text === (key.includes("::") ? key.slice(key.indexOf("::") + 2) : key)).map(([key]) => key);
    expect(same).toStrictEqual(["CO2", "PM2.5", "CO₂", "WhatsApp", "MRV", "{field} — {problem}", "v{version} · {state}", "fw {version}", "RTO", "HH:MM – HH:MM", "{from} – {to}"]);
  });
  it("writes every Japanese text with Japanese characters unless it is a name or a format", () => {
    const proper = new Set(["CO2", "PM2.5", "CO₂", "WhatsApp", "MRV", "RTO"]);
    const japanese = /[\u{3040}-\u{30FF}\u{4E00}-\u{9FFF}]/u;
    // a text of placeholders, numbers and signs alone ("{from}～{to}") has no word to translate
    const english = Object.entries(ja).filter(([key, text]) => !proper.has(key) && !japanese.test(text) && /[A-Za-z]{4,}/.test(text.replaceAll(/\{\w+\}/g, ""))).map(([key]) => key);
    expect(english).toStrictEqual([]);
  });

  it("speaks to customers, contractors, technicians and HQ without error codes or spec IDs (IR584, IR585, IR587, IR590, IR591, IR595, IR622, IR626)", () => {
    const files = ["customer/app", "partner/app", "technician/app", "admin/app", "shared/lib", "shared/components", "shared/screens"].flatMap((d) => sources(path.join(root, d)));
    // each string literal as written in the source, read as the text it stands for
    const strings = files.flatMap((f) => readFileSync(f, "utf8").matchAll(/"((?:[^"\\\n]|\\.)*)"/g).map((m) => literalText(`"${m[1] ?? ""}"`)).filter((x) => x !== null).toArray());
    const texts = [...new Set(strings.filter((text) => Object.hasOwn(ms, text)))]; // every literal the dictionaries translate (they share their keys), maps of texts included
    expect(texts.length).toBeGreaterThan(100);
    // screen IDs as well ("Access & roles (A03)")
    const coded = /\b(?:IR\d+|(?:FR|DD|BR|NFR|DEC|AT|SCR|DDC|REV)-[A-Z]*\d+|SR\d+|D\d{2,})\b|^(?:CONFLICT|NOT_FOUND|FORBIDDEN|UNAVAILABLE|VALIDATION|TIMEOUT|OFFLINE)\b|\((?:CONFLICT|NOT_FOUND|FORBIDDEN|UNAVAILABLE|VALIDATION|TIMEOUT|OFFLINE)\)|\([ACPTX]\d{2}[a-z]?\)/;
    expect(texts.filter((text) => coded.test(text))).toStrictEqual([]);
  });

  // IR626: a status or event code ("release_requested", "end_failed") is said in words; dotted permissions such as
  // restriction.read and {placeholders} are not codes of this kind
  it("shows no snake_case status or event code in a text the apps translate", () => {
    expect(Object.keys(ms).filter((text) => /(?<![\w./{-])[a-z]+_[a-z_]+(?![\w}])/.test(text))).toStrictEqual([]);
  });

  // IR626: no field or operation name of the code ("previousAlertId", "jobs.saveCost") and no IR number either; units
  // such as kWh and kgCO₂e and the {placeholders} are not names of this kind
  it("shows no field name, operation name or IR number in a text the apps translate", () => {
    const named = /\b(?!kWh\b|kgCO)[a-z]+[A-Z][a-z][A-Za-z]*\b|\b[a-z]+\.[a-z]+[A-Z][A-Za-z]*\b|\bIR\d+\b/;
    expect(Object.keys(ms).filter((text) => named.test(text.replaceAll(/\{\w+\}/g, "")))).toStrictEqual([]);
  });

  // IR624: the keys are English; a Japanese term of the specification ("受領") once reached the partner's schedule as one
  it("shows no Japanese term in an English key", () => {
    expect(Object.keys(ms).filter((text) => /[\u{3040}-\u{30FF}\u{4E00}-\u{9FFF}]/u.test(text))).toStrictEqual([]);
  });

  it.each(LANGUAGES)("names every sidebar item and the role words of the four apps in %s", (_locale, _name, dictionary) => {
    for (const config of Object.values(ROLES)) {
      const translated = config.nav.filter((i) => i.label !== "MRV"); // the MRV label is the same in every language
      for (const item of translated) expect(lookup(dictionary, item.label), item.label).toBeTruthy();
      expect(lookup(dictionary, config.sub), config.sub).toBeTruthy();
      const word = must(config.chip.split(" — ", 1)[0]); // the role chip of the header
      expect(lookup(dictionary, word), word).toBeTruthy();
    }
  });

  it("puts the consent line in the display language", () => {
    const c = { id: "c1", version: 2, granted: true, grantedAt: "2026-09-14T01:00:00Z", revokedAt: null };
    expect([consentNote(c, translator("ms")), consentNote(null, translator("ms"))]).toStrictEqual(["Diberikan 2026-09-14", "Belum diberikan"]);
    expect([consentNote(c, translator("ja")), consentNote(null, translator("ja"))]).toStrictEqual(["2026-09-14 に同意", "まだ同意していません"]);
  });

  it("speaks Japanese, with the same placeholders and fallbacks as Malay (IR639)", () => {
    expect([translate("ja", "Sign out"), translate("ja", "Not saved: {reason}", { reason: "conflict" })]).toStrictEqual(["ログアウト", "保存できませんでした: conflict"]);
    expect([translate("ja", "tamper::Clear"), translate("ja", "Clear")]).toStrictEqual(["解除", "クリア"]);
    expect(translate("ja", "A text nobody translated yet")).toBe("A text nobody translated yet");
    expect(translate("ja", "nobody::Untranslated")).toBe("Untranslated");
  });

  it("speaks Indonesian, with the same placeholders and fallbacks as Malay (DEC-108)", () => {
    expect([translate("id", "Sign out"), translate("id", "Not saved: {reason}", { reason: "conflict" })]).toStrictEqual(["Keluar", "Tidak tersimpan: conflict"]);
    expect([translate("id", "tamper::Clear"), translate("id", "Clear")]).toStrictEqual(["Tidak ada perusakan", "Bersihkan"]);
    expect(translate("id", "A text nobody translated yet")).toBe("A text nobody translated yet");
    expect(translate("id", "nobody::Untranslated")).toBe("Untranslated");
  });

  // DEC-108: Indonesian shares many loanwords with English (Unit, Status, Filter, Online); only names, formats and such words may stay as written
  it("leaves in English only short names, formats and loanwords in the Indonesian dictionary", () => {
    const same = Object.entries(id).filter(([key, text]) => text === (key.includes("::") ? key.slice(key.indexOf("::") + 2) : key)).map(([key]) => key);
    expect(same.length).toBeLessThan(120);
    expect(same.filter((key) => key.length > 40)).toStrictEqual([]);
  });

  it("shows an Indonesian date and time with the zone's abbreviation from the English data (DEC-108)", () => {
    expect(showTime("2026-09-14T01:00:00Z", { locale: "id", timeZone: ASIA_KUALA_LUMPUR })).toMatch(/MYT$/);
  });

  it("counts in each language: 1st, ke-1 (Malay and Indonesian), 第1", () => {
    expect([ordinal(1), ordinal(2), ordinal(11), ordinal(3, "ms"), ordinal(3, "ja"), ordinal(3, "id")]).toStrictEqual(["1st", "2nd", "11th", "ke-3", "第3", "ke-3"]);
  });

  it("accepts only the four locales; English and Malay format for Malaysia, Japanese for Japan, Indonesian for Indonesia", () => {
    expect([isLocale("en"), isLocale("ms"), isLocale("ja"), isLocale("id"), isLocale("fr"), isLocale(null)]).toStrictEqual([true, true, true, true, false, false]);
    expect([intlTag("en"), intlTag("ms"), intlTag("ja"), intlTag("id")]).toStrictEqual(["en-MY", "ms-MY", "ja-JP", "id-ID"]);
  });

  it("shows a Japanese date and time with the zone's abbreviation, as in English (IR639)", () => {
    const display = { locale: "ja" as const, timeZone: ASIA_KUALA_LUMPUR };
    const text = showTime("2026-09-14T01:00:00Z", display);
    expect(text).toBe("2026/09/14 9:00 MYT"); // Japan's medium date, and the zone is MYT, not GMT+8
    expect(showTime("2026-09-14T01:00:00Z", { locale: "ja", timeZone: ASIA_TOKYO })).toBe("2026/09/14 10:00 GMT+9");
    // English and Malay keep the abbreviations of their own data (the Malay one says SGT for Singapore)
    expect(showTime("2026-09-14T01:00:00Z", { locale: "ms", timeZone: "Asia/Singapore" })).toMatch(/SGT$/);
  });

  it("reads a typed date and time in the display time zone and prefills it back (NFR-08)", () => {
    expect([zonedInstant(TEXT_2026_09_16, "10:00", ASIA_KUALA_LUMPUR), zonedInstant(TEXT_2026_09_16, "10:00", ASIA_TOKYO), zonedInstant(TEXT_2026_09_16, "10:00", "UTC")])
      .toStrictEqual(["2026-09-16T02:00:00.000Z", "2026-09-16T01:00:00.000Z", "2026-09-16T10:00:00.000Z"]);
    expect([zonedInstant(TEXT_2026_09_16, "", ASIA_TOKYO), zonedInstant("", "10:00", ASIA_TOKYO), zonedInstant("2026-02-31", "10:00", "UTC")]).toStrictEqual(["", "", "2026-03-03T10:00:00.000Z"]);
    expect(zonedInstant(TEXT_2026_09_16, "10:00", "Not/AZone")).toBe("2026-09-16T02:00:00.000Z"); // an unknown zone falls back to Kuala Lumpur
    expect([zonedParts("2026-09-16T01:00:00Z", ASIA_TOKYO), zonedParts("2026-09-15T23:30:00Z", ASIA_KUALA_LUMPUR)]).toStrictEqual([{ date: TEXT_2026_09_16, time: "10:00" }, { date: TEXT_2026_09_16, time: "07:30" }]);
  });

  it("shows a span in one line, with plain spaces whatever the ICU (no hydration difference)", () => {
    const span = showSpan("2026-09-16T02:00:00Z", "2026-09-16T04:00:00Z", { locale: "ms", timeZone: ASIA_TOKYO }, true);
    expect(span).toBe("Rab, 16 Sep, 11:00 PG – 1:00 PTG GMT+9");
    expect(/[\u{A0}\u{2009}\u{202F}]/u.test(span + showTime("2026-09-16T02:00:00Z"))).toBe(false);
    expect(showSpan("2026-09-14T00:00:00Z", "2026-09-20T00:00:00Z")).toBe("14 Sept, 8:00 am – 20 Sept, 8:00 am MYT");
  });

  it("shows a time in the user's language and time zone with the zone's abbreviation (IR44)", () => {
    const at = "2026-09-14T01:00:00Z";
    expect([showTime(at), showTime(at, { locale: "ms", timeZone: ASIA_KUALA_LUMPUR }), showTime(at, { locale: "en", timeZone: ASIA_TOKYO }), showTime(at, { locale: "en", timeZone: "UTC" })])
      .toStrictEqual(["14 Sept 2026, 9:00 am MYT", "14 Sep 2026, 9:00 PG MYT", "14 Sept 2026, 10:00 am GMT+9", "14 Sept 2026, 1:00 am UTC"]);
  });
  it("shows a recent time alone on today and with its date on another day (IR613)", () => {
    const at = "2026-09-14T00:30:00Z"; // 8:30 am in Kuala Lumpur
    expect([showRecent(at, Date.parse("2026-09-14T10:00:00Z")), showRecent(at, Date.parse("2026-09-23T06:00:00Z")), showRecent(at, Date.parse("2026-09-13T17:00:00Z")), showRecent(null, 0)])
      .toStrictEqual(["8:30 am MYT", "14 Sept 2026, 8:30 am MYT", "8:30 am MYT", "—"]); // 13 Sept 17:00 UTC is already the 14th in Kuala Lumpur
    expect(showRecent(at, Date.parse("2026-09-14T15:30:00Z"), { locale: "en", timeZone: ASIA_TOKYO })).toBe("14 Sept 2026, 9:30 am GMT+9"); // the 15th in Tokyo
  });
});
