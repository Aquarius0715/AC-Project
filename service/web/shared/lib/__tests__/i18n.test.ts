import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { intlTag, isLocale, showTime, translate, translator } from "@ac/web/lib/i18n";
import { ms } from "@ac/web/lib/i18n-ms";
import { ROLES } from "@ac/web/lib/nav";
import { consentNote } from "@ac/web/lib/preferences";

describe("Display language (FR-X01, AT-X01-N, IR258)", () => {
  it("keeps English as the key and falls back to it when Malay has no entry", () => {
    expect([translate("en", "Sign out"), translate("ms", "Sign out")]).toEqual(["Sign out", "Log keluar"]);
    expect(translate("ms", "A text nobody translated yet")).toBe("A text nobody translated yet");
  });

  it("fills {name} placeholders in both languages and leaves unknown ones as they are", () => {
    expect(translate("en", "Not saved: {reason}", { reason: "conflict" })).toBe("Not saved: conflict");
    expect(translate("ms", "Not saved: {reason}", { reason: "conflict" })).toBe("Tidak disimpan: conflict");
    expect(translate("ms", " · {n} recovery codes left", { n: 7 })).toBe(" · 7 kod pemulihan berbaki");
    expect(translate("en", "{a} and {b}", { a: "x" })).toBe("x and {b}");
  });

  it("keeps every placeholder of an English key in its Malay text", () => {
    const names = (text: string) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
    for (const [en, text] of Object.entries(ms)) expect(names(text), en).toEqual(names(en));
  });

  // NFR-07 / IR44 key check: the English texts the code translates and the dictionary's keys are the same set
  const root = resolve(__dirname, "../../..");
  const sources = (dir: string): string[] => readdirSync(dir).flatMap((f) => {
    const path = join(dir, f);
    if (["node_modules", ".next", "__tests__", "e2e"].includes(f)) return [];
    return statSync(path).isDirectory() ? sources(path) : /\.tsx?$/.test(f) && !f.endsWith("i18n-ms.ts") ? [path] : [];
  });
  const code = ["shared", "customer/app", "partner/app", "technician/app", "admin/app"].flatMap((d) => sources(join(root, d))).map((f) => readFileSync(f, "utf8")).join("\n");

  it("has a Malay text for every literal the code translates", () => {
    const literals = [...code.matchAll(/\b(?:t\(|translate\(\s*[\w.]+,)\s*"((?:[^"\\]|\\.)*)"/g)].map((m) => JSON.parse(`"${m[1]}"`) as string);
    expect(literals.length).toBeGreaterThan(100);
    expect([...new Set(literals.filter((text) => !(text in ms)))]).toEqual([]);
  });

  it("has no Malay entry the code no longer shows", () => {
    const dynamic = new Set(Object.values(ROLES).flatMap((cfg) => [cfg.sub, cfg.chip.split(" — ")[0], ...cfg.nav.map((i) => i.label)]));
    expect(Object.keys(ms).filter((key) => !dynamic.has(key) && !code.includes(JSON.stringify(key)))).toEqual([]);
  });

  it("names every sidebar item and the role words of the four apps in Malay", () => {
    for (const cfg of Object.values(ROLES)) {
      for (const item of cfg.nav.filter((i) => i.label !== "MRV")) expect(ms[item.label], item.label).toBeTruthy();
      expect(ms[cfg.sub], cfg.sub).toBeTruthy();
      const word = cfg.chip.split(" — ")[0]; // the role chip of the header
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

  it("shows a time in the user's language and time zone with the zone's abbreviation (IR44)", () => {
    const at = "2026-09-14T01:00:00Z";
    expect([showTime(at), showTime(at, { locale: "ms", timeZone: "Asia/Kuala_Lumpur" }), showTime(at, { locale: "en", timeZone: "Asia/Tokyo" }), showTime(at, { locale: "en", timeZone: "UTC" })])
      .toEqual(["14 Sept 2026, 9:00 am MYT", "14 Sep 2026, 9:00 PG MYT", "14 Sept 2026, 10:00 am GMT+9", "14 Sept 2026, 1:00 am UTC"]);
  });
});
