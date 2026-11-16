"use client";
// The ACs a customer pinned to their Overview (IR814): a per-browser convenience, kept in localStorage and validated
// with Zod when read (a stored value is outside input); a blocked or broken store reads as no pins.
import { z } from "zod";
import { setStoredValue, useStoredValue } from "@ac/web/lib/urlState";

const KEY = "ac:pinned-units";
/** At most this many pins: the Overview card stays short. */
export const MAX_PINS = 8;
const pinsSchema = z.array(z.string().min(1).max(64)).max(MAX_PINS);

/** The pinned unit ids from a stored value: none when it is missing or not a valid list. */
export function parsePins(raw: string | null): string[] {
  if (raw === null) return [];
  try {
    const parsed = pinsSchema.safeParse(JSON.parse(raw));
    return parsed.success ? parsed.data : [];
  } catch {
    return [];
  }
}

/** The pins after pinning or unpinning `id`: a new pin goes first; the oldest drops beyond MAX_PINS. */
export function togglePin(pins: string[], id: string): string[] {
  return pins.includes(id) ? pins.filter((x) => x !== id) : [id, ...pins].slice(0, MAX_PINS);
}

export function usePinnedUnits(): [string[], (id: string) => void] {
  const pins = parsePins(useStoredValue(KEY));
  return [pins, (id) => { setStoredValue(KEY, JSON.stringify(togglePin(pins, id))); }];
}
