// The parts of a time the pickers choose from (IR638): TimeSelect works on "HH:mm", DateTimeSelect on
// "YYYY-MM-DDTHH:mm" — the same strings the native time and datetime-local inputs gave, so the forms, their validators
// and the zone conversions stay as they were.

export const HOURS: readonly string[] = Array.from({ length: 24 }, (_, h) => String(h).padStart(2, "0"));
/** The minutes on offer: a quarter of an hour apart. */
export const MINUTES: readonly string[] = ["00", "15", "30", "45"];

const TIME = /^([01]\d|2[0-3]):([0-5]\d)$/;
const DATE_TIME = /^(\d{4}-\d{2}-\d{2})T(.+)$/;

/** "09:30" → hour and minute; anything else (empty, unfinished, out of range) → both empty. */
export function splitTime(value: string): { hour: string; minute: string } {
  const m = TIME.exec(value);
  return { hour: m?.[1] ?? "", minute: m?.[2] ?? "" };
}

/** Hour and minute → "HH:mm"; an empty hour is no time at all. */
export function joinTime(hour: string, minute: string): string {
  if (hour === "") return "";
  return `${hour}:${minute === "" ? "00" : minute}`;
}

/** "2026-09-01T09:30" → date and time ("" when the value has none). */
export function splitDateTime(value: string): { date: string; time: string } {
  const m = DATE_TIME.exec(value);
  return m ? { date: m[1] ?? "", time: m[2] ?? "" } : { date: value, time: "" };
}

/** Date and time → "YYYY-MM-DDTHH:mm". No date is no value; a date alone takes `fallback` as its time. */
export function joinDateTime(date: string, time: string, fallback = "00:00"): string {
  if (date === "") return "";
  return `${date}T${time === "" ? fallback : time}`;
}

// The minutes to choose from. A stored minute off the quarter hours (07:10, or the current minute as a default) is kept as one more option and shown as it is: the picker never rounds a value on its own.
export function minuteOptions(current: string): readonly string[] {
  return current === "" || MINUTES.includes(current) ? MINUTES : [...MINUTES, current].toSorted((a, b) => Number(a) - Number(b));
}
