// The ACs a customer pinned to their Overview (IR814), kept with the user's preferences on the server (IR819).
/** At most this many pins: the Overview card stays short (the preferences column's CHECK too). */
export const MAX_PINS = 8;

/** The pins after pinning or unpinning `id`: a new pin goes first; the oldest drops beyond MAX_PINS. */
export function togglePin(pins: string[], id: string): string[] {
  return pins.includes(id) ? pins.filter((x) => x !== id) : [id, ...pins].slice(0, MAX_PINS);
}
