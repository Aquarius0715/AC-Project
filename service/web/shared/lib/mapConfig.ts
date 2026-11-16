// The map and the address search of the property pin (DEC-109, IR824). Both are chosen by the deployment's environment,
// read on the server at run time, so one image serves every environment:
//   MAP_STYLE_URL  a MapLibre style (an HTTPS URL). Development and test use key-free tiles (OpenFreeMap); production uses
//                  Amazon Location Maps, whose key is a public key restricted to the map and the app's address.
//                  Unset or invalid: no map, the pin is typed as latitude and longitude.
//   GEOCODER       `none` (default) | `nominatim` (development and test only: 1 request a second, a search on a
//                  button) | `aws` (Amazon Location Places, with AWS_LOCATION_REGION and AWS_LOCATION_API_KEY).
// The style's origin is the only map origin the Content-Security-Policy allows (tiles, glyphs and sprites of both
// providers come from the style's own host).
import { z } from "zod";

type Env = Readonly<Record<string, string | undefined>>;

const httpsUrl = z.url({ protocol: /^https$/ });
const geocoder = z.enum(["none", "nominatim", "aws"]);
const awsRegion = z.string().regex(/^[a-z]{2}(-[a-z]+)+-\d$/);
const apiKey = z.string().min(1).max(512);

/** What the property form needs to draw the map: the style, or null for no map, and whether the address search is on. */
export interface MapConfig { styleUrl: string | null; search: boolean }

/** The validated MapLibre style URL, or null. */
export function mapStyleUrl(env: Env = process.env): string | null {
  const parsed = httpsUrl.safeParse(env.MAP_STYLE_URL?.trim());
  return parsed.success ? parsed.data : null;
}

/** The origins the page may fetch map resources from (the style's own origin), for the Content-Security-Policy. */
export function mapOrigins(env: Env = process.env): string[] {
  const url = mapStyleUrl(env);
  return url ? [new URL(url).origin] : [];
}

/** The address search the server uses; Amazon Location without its region and key is no search. */
export type Geocoder = { kind: "none" } | { kind: "nominatim" } | { kind: "aws"; region: string; key: string };
export function geocoderOf(env: Env = process.env): Geocoder {
  const kind = geocoder.catch("none").parse(env.GEOCODER?.trim());
  if (kind !== "aws") return { kind };
  const region = awsRegion.safeParse(env.AWS_LOCATION_REGION?.trim());
  const key = apiKey.safeParse(env.AWS_LOCATION_API_KEY?.trim());
  return region.success && key.success ? { kind, region: region.data, key: key.data } : { kind: "none" };
}

export const mapConfig = (env: Env = process.env): MapConfig => ({ styleUrl: mapStyleUrl(env), search: geocoderOf(env).kind !== "none" });
