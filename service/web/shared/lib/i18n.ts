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

/** The text in `locale`, with `{name}` placeholders filled from params; English when Malay has no entry. When one English
 * text means two things, its key names the context before "::" ("tamper::Clear" is a state, "Clear" a button); English
 * shows the text after it. */
export function translate(locale: Locale, text: string, params?: Params): string {
  const english = text.includes("::") ? text.slice(text.indexOf("::") + 2) : text;
  let base = english;
  if (locale === "ms") {
    base = ms[text] ?? english;
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

/** Intl output with plain spaces: ICU puts narrow and thin spaces around times and ranges, and versions differ, so a
 * time rendered on the server reads the same after hydration in any browser. */
const plain = (s: string) => s.replace(/[\u00a0\u2009\u202f]/g, " ");

/** `opts` in the user's language and display time zone with the zone's abbreviation (unless `withZone` is false); a
 * zone this runtime does not know falls back to the default one. */
function zoned(iso: string | null, d: Display, opts: Intl.DateTimeFormatOptions, withZone = true): string {
  if (!iso) return "—";
  const at = new Date(iso);
  const tag = intlTag(d.locale);
  for (const timeZone of [d.timeZone, DEFAULT_DISPLAY.timeZone]) {
    try {
      const zone = withZone ? new Intl.DateTimeFormat(tag, { timeZone, timeZoneName: "short" }).formatToParts(at).find((p) => p.type === "timeZoneName")?.value : undefined;
      const text = plain(at.toLocaleString(tag, { ...opts, timeZone }));
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

/** The IR44 time without the date, where a screen shows only the time of a recent reading: “9:12 am MYT”; without the
 * abbreviation (`zone` false) on a chart axis whose zone the chart names once. */
export const showClock = (iso: string | null, d: Display = DEFAULT_DISPLAY, zone = true) => zoned(iso, d, { timeStyle: "short" }, zone);

/** The short weekday and the day of the month, “Mon 14” (Figma Client 04a, 05a), in the user's language and a time zone. */
export function showDay(iso: string, d: Display = DEFAULT_DISPLAY): string {
  for (const timeZone of [d.timeZone, DEFAULT_DISPLAY.timeZone]) {
    try {
      const part = (o: Intl.DateTimeFormatOptions) => new Date(iso).toLocaleDateString(intlTag(d.locale), { timeZone, ...o });
      return plain(`${part({ weekday: "short" })} ${part({ day: "numeric" })}`); // "Mon 14", not the en-US "14 Mon"
    } catch {
      // RangeError: unknown time zone
    }
  }
  return iso.slice(0, 10);
}

/** The IR44 date without the time, for the day something was done: “28 Sept 2026” in the user's display time zone. */
export function showDate(iso: string | null, d: Display = DEFAULT_DISPLAY): string {
  if (!iso) return "—";
  for (const timeZone of [d.timeZone, DEFAULT_DISPLAY.timeZone]) {
    try {
      return plain(new Date(iso).toLocaleDateString(intlTag(d.locale), { dateStyle: "medium", timeZone }));
    } catch {
      // RangeError: unknown time zone
    }
  }
  return iso.slice(0, 10);
}

/** The day and month without the year (“15 Sept”), where a table of one statement or period shows its dates, in the
 * user's language and display time zone. */
export function showDayMonth(iso: string | null, d: Display = DEFAULT_DISPLAY): string {
  if (!iso) return "—";
  for (const timeZone of [d.timeZone, DEFAULT_DISPLAY.timeZone]) {
    try {
      return plain(new Date(iso).toLocaleDateString(intlTag(d.locale), { day: "numeric", month: "short", timeZone }));
    } catch {
      // RangeError: unknown time zone
    }
  }
  return iso.slice(5, 10);
}

/** A time span — a booked visit or a preferred time: “Tue, 22 Sept, 10:00 am – 12:00 pm MYT” in the user's language
 * and display time zone, the weekday only when asked for. The span is the stored instants, so it keeps its meaning when
 * the user changes the time zone (NFR-08). */
export function showSpan(startIso: string, endIso: string, d: Display = DEFAULT_DISPLAY, weekday = false): string {
  const a = new Date(startIso), b = new Date(endIso);
  const tag = intlTag(d.locale);
  for (const timeZone of [d.timeZone, DEFAULT_DISPLAY.timeZone]) {
    try {
      const zone = new Intl.DateTimeFormat(tag, { timeZone, timeZoneName: "short" }).formatToParts(a).find((p) => p.type === "timeZoneName")?.value;
      const range = plain(new Intl.DateTimeFormat(tag, { timeZone, ...(weekday ? { weekday: "short" } : {}), month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).formatRange(a, b));
      return zone ? `${range} ${zone}` : range;
    } catch {
      // RangeError: unknown time zone or invalid dates
    }
  }
  return `${startIso} – ${endIso}`;
}

/** The wall clock (as if UTC) of an instant in a time zone. */
function wallMs(ms: number, timeZone: string): number {
  const p = Object.fromEntries(new Intl.DateTimeFormat("en-US", { timeZone, hourCycle: "h23", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" })
    .formatToParts(ms).map((x) => [x.type, x.value]));
  return Date.UTC(+p.year, +p.month - 1, +p.day, +p.hour, +p.minute, +p.second);
}
const knownZone = (tz: string) => {
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: tz });
    return tz;
  } catch {
    return DEFAULT_DISPLAY.timeZone;
  }
};

/** The instant of a date (YYYY-MM-DD) and time (HH:mm) typed in a time zone — a form in the user's display time zone
 * (NFR-08); "" while the date or time is not complete. */
export function zonedInstant(date: string, time: string, timeZone: string): string {
  const wall = Date.parse(`${date}T${time}:00Z`);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !/^\d{2}:\d{2}$/.test(time) || Number.isNaN(wall)) return "";
  const tz = knownZone(timeZone);
  let at = wall - (wallMs(wall, tz) - wall);
  at = wall - (wallMs(at, tz) - at); // the offset at the instant itself (a zone that changes its offset)
  return new Date(at).toISOString();
}
/** The date (YYYY-MM-DD) and time (HH:mm) of an instant in a time zone, to prefill such a form. */
export function zonedParts(iso: string, timeZone: string): { date: string; time: string } {
  const w = new Date(wallMs(Date.parse(iso), knownZone(timeZone))).toISOString();
  return { date: w.slice(0, 10), time: w.slice(11, 16) };
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
