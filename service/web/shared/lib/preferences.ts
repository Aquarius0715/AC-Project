// Preferences screen helpers (FR-X01, FR-X08, DDC-07, IR246): time zone choices with their UTC offset at the demo clock,
// and the consent line. Offsets come from the numeric wall-clock parts, so the server and the browser render the same
// text whatever their Intl time zone names are.

export const ZONES = ["Asia/Kuala_Lumpur", "Asia/Singapore", "Asia/Jakarta", "Asia/Bangkok", "Asia/Tokyo", "UTC"];

/** "UTC+8", "UTC+0", "UTC+5:30", "UTC−3" for `tz` at `at`; "" for a zone this runtime does not know. */
export function zoneOffset(tz: string, at: Date): string {
  let p: Record<string, string>;
  try {
    p = Object.fromEntries(new Intl.DateTimeFormat("en-US", { timeZone: tz, hourCycle: "h23", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
      .formatToParts(at).map((x) => [x.type, x.value]));
  } catch {
    return "";
  }
  const minute = Math.floor(at.getTime() / 60_000) * 60_000;
  const min = Math.round((Date.UTC(+p.year, +p.month - 1, +p.day, +p.hour, +p.minute) - minute) / 60_000);
  const abs = Math.abs(min);
  return `UTC${min < 0 ? "−" : "+"}${Math.floor(abs / 60)}${abs % 60 ? `:${String(abs % 60).padStart(2, "0")}` : ""}`;
}

/** The time zone options, the saved zone first when it is not one of the usual ones. */
export function zoneOptions(saved: string, at: Date): { id: string; label: string }[] {
  return (ZONES.includes(saved) ? ZONES : [saved, ...ZONES]).map((id) => {
    const o = zoneOffset(id, at);
    return { id, label: o ? `${id} (${o})` : id };
  });
}

export type Consent = { id: string; version: number; granted: boolean; grantedAt: string | null; revokedAt: string | null };

/** The location consent line: when it was granted or withdrawn (dates as recorded, UTC), or that it never was. */
export function consentNote(c: Consent | null): string {
  if (c?.granted) return `Granted ${c.grantedAt?.slice(0, 10) ?? ""}`.trim();
  if (c?.revokedAt) return `Withdrawn ${c.revokedAt.slice(0, 10)}`;
  return "Not granted yet";
}
