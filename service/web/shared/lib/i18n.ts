// Display language (FR-X01, AT-X01-N, IR44, IR258). English is the default and the key: each text is looked up in the
// Malay dictionary (lib/i18n-ms.ts, a draft until Business / UI / UX review it) and stays English when it has no entry,
// with a warning in development (IR44). The language never changes stored values, units, IDs or UTC times. As in the
// Next.js internationalization guide, the dictionary is a plain object and the locale comes from the server (the
// user's Preferences). lib/__tests__/i18n.test.ts keeps the code's texts and the dictionary's keys the same (NFR-07).
import { ms } from "@ac/web/lib/i18n-ms";

export type Locale = "en" | "ms";
export const LOCALES: Locale[] = ["en", "ms"];
export const isLocale = (v: unknown): v is Locale => v === "en" || v === "ms";
/** The browser demo keeps the chosen language in this browser (Preferences demo). */
export const LOCALE_KEY = "ac-locale";

export type Params = Record<string, string | number>;
export type T = (text: string, params?: Params) => string;

const warned = new Set<string>();

/** The text in `locale`, with `{name}` placeholders filled from params; English when Malay has no entry. */
export function translate(locale: Locale, text: string, params?: Params): string {
  let base = text;
  if (locale === "ms") {
    base = ms[text] ?? text;
    if (!(text in ms) && process.env.NODE_ENV === "development" && !warned.has(text)) {
      warned.add(text);
      console.warn(`[i18n] no Malay text for "${text}"`);
    }
  }
  return params ? base.replace(/\{(\w+)\}/g, (all, k: string) => (k in params ? String(params[k]) : all)) : base;
}

export const translator = (locale: Locale): T => (text, params) => translate(locale, text, params);

/** The BCP 47 tag for dates and numbers shown in `locale` (Malaysia for both). */
export const intlTag = (locale: Locale) => (locale === "ms" ? "ms-MY" : "en-MY");

/** How a user's screens show text and times: the display language and time zone of Preferences. */
export type Display = { locale: Locale; timeZone: string };
export const DEFAULT_DISPLAY: Display = { locale: "en", timeZone: "Asia/Kuala_Lumpur" };

/** `opts` in the user's language and display time zone with the zone's abbreviation; a zone this runtime does not
 * know falls back to the default one. */
function zoned(iso: string | null, d: Display, opts: Intl.DateTimeFormatOptions): string {
  if (!iso) return "—";
  const at = new Date(iso);
  const tag = intlTag(d.locale);
  for (const timeZone of [d.timeZone, DEFAULT_DISPLAY.timeZone]) {
    try {
      const zone = new Intl.DateTimeFormat(tag, { timeZone, timeZoneName: "short" }).formatToParts(at).find((p) => p.type === "timeZoneName")?.value;
      const text = at.toLocaleString(tag, { ...opts, timeZone });
      return zone ? `${text} ${zone}` : text;
    } catch {
      // RangeError: unknown time zone
    }
  }
  return iso;
}

/** A date and time as the user reads it (IR44): medium date and short time in their language, in their display time
 * zone, with the zone's abbreviation — “14 Sept 2026, 9:00 am MYT”; “—” without a time. Stored times stay UTC. */
export const showTime = (iso: string | null, d: Display = DEFAULT_DISPLAY) => zoned(iso, d, { dateStyle: "medium", timeStyle: "short" });

/** The IR44 time without the date, where a screen shows only the time of a recent reading: “9:12 am MYT”. */
export const showClock = (iso: string | null, d: Display = DEFAULT_DISPLAY) => zoned(iso, d, { timeStyle: "short" });

/** The IR44 date without the time, for the day something was done: “28 Sept 2026” in the user's display time zone. */
export function showDate(iso: string | null, d: Display = DEFAULT_DISPLAY): string {
  if (!iso) return "—";
  for (const timeZone of [d.timeZone, DEFAULT_DISPLAY.timeZone]) {
    try {
      return new Date(iso).toLocaleDateString(intlTag(d.locale), { dateStyle: "medium", timeZone });
    } catch {
      // RangeError: unknown time zone
    }
  }
  return iso.slice(0, 10);
}

/** The translator with the display, for the pure helpers that word and time a screen's text (IR259, IR260). */
export type I18n = { t: T; display: Display };
export const i18nOf = (display: Display): I18n => ({ t: translator(display.locale), display });
export const EN: I18n = i18nOf(DEFAULT_DISPLAY);

/** The calendar day (YYYY-MM-DD) of an instant in a time zone (the default zone for one the runtime does not know). */
function dayKey(ms: number, timeZone: string): string {
  try {
    return new Date(ms).toLocaleDateString("en-CA", { timeZone });
  } catch {
    return new Date(ms).toLocaleDateString("en-CA", { timeZone: DEFAULT_DISPLAY.timeZone });
  }
}

/** “today 9:12 am MYT”, “yesterday …”, “tomorrow …” on the days next to now in the user's display time zone, else the
 * IR44 date and time: the relative day stands in for the date (IR260). */
export function relativeTime(iso: string, nowMs: number, i: I18n = EN): string {
  const tz = i.display.timeZone;
  const day = dayKey(Date.parse(iso), tz);
  const time = showClock(iso, i.display);
  if (day === dayKey(nowMs, tz)) return i.t("today {time}", { time });
  if (day === dayKey(nowMs - 86_400_000, tz)) return i.t("yesterday {time}", { time });
  if (day === dayKey(nowMs + 86_400_000, tz)) return i.t("tomorrow {time}", { time });
  return showTime(iso, i.display);
}
