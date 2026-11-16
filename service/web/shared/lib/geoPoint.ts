// The property pin's coordinates as the form types and shows them (DEC-109, IR824). Pure code, unit-tested.
import type { GeoPoint } from "@ac/web/lib/assets";

/** Six decimals: about 0.1 m, finer than any GPS fix. */
const roundDegrees = (x: number): number => Math.round(x * 1e6) / 1e6;
export const pointOf = (latitude: number, longitude: number): GeoPoint => ({ latitude: roundDegrees(latitude), longitude: roundDegrees(longitude) });

/** Kuala Lumpur: where the map opens when the property has no pin yet. */
export const DEFAULT_CENTER: GeoPoint = { latitude: 3.139, longitude: 101.6869 };

/** The typed latitude and longitude: both empty is no pin, both valid numbers on the globe is a pin, else invalid. */
export function parsePoint(latitude: string, longitude: string): { ok: true; point: GeoPoint | null } | { ok: false } {
  const a = latitude.trim();
  const b = longitude.trim();
  if (a === "" && b === "") return { ok: true, point: null };
  const number = /^[-+]?\d{1,3}(\.\d+)?$/;
  if (!number.test(a) || !number.test(b)) return { ok: false };
  const lat = Number(a);
  const lng = Number(b);
  return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 ? { ok: true, point: pointOf(lat, lng) } : { ok: false };
}

/** "3.157900, 101.711600" */
export const showPoint = (p: GeoPoint): string => `${p.latitude.toFixed(6)}, ${p.longitude.toFixed(6)}`;

/** The away/home radius the API uses when the property has none (metres). */
export const DEFAULT_GEOFENCE_RADIUS = 150;

/** The typed radius: empty is the default (null), else a whole number of metres in 50–2000. */
export function parseRadius(text: string): { ok: true; radius: number | null } | { ok: false } {
  const s = text.trim();
  if (s === "") return { ok: true, radius: null };
  if (!/^\d{1,4}$/.test(s)) return { ok: false };
  const r = Number(s);
  return r >= 50 && r <= 2000 ? { ok: true, radius: r } : { ok: false };
}

/** The great-circle distance between two points in metres (the mean Earth sphere), for deciding when the browser
reports again; the Core API measures the distances that decide (internal/platform/geo). */
export function haversine(a: GeoPoint, b: GeoPoint): number {
  const rad = Math.PI / 180;
  const h = Math.sin(((b.latitude - a.latitude) * rad) / 2) ** 2 + Math.cos(a.latitude * rad) * Math.cos(b.latitude * rad) * Math.sin(((b.longitude - a.longitude) * rad) / 2) ** 2;
  return 2 * 6_371_008.8 * Math.asin(Math.min(1, Math.sqrt(h)));
}
