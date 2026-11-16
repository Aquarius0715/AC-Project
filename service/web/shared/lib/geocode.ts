// The address search of the property pin (DEC-109, IR824), called by an Admin Server Action only. The provider is the
// deployment's (mapConfig.ts). The query goes to a fixed provider address, URL-encoded; nothing about the user or the
// customer is sent with it. Every answer is checked with Zod before use.
import "server-only";
import { z } from "zod";
import { geocoderOf, type Geocoder } from "@ac/web/lib/mapConfig";
import type { GeoPoint } from "@ac/web/lib/assets";
import { pointOf } from "@ac/web/lib/geoPoint";

/** One place the search found. */
export interface Place { label: string; location: GeoPoint }
export type SearchResult = { ok: true; places: Place[] } | { ok: false; reason: "unavailable" | "busy" | "off" };

const MAX_RESULTS = 5;
const TIMEOUT_MS = 5000;
const latitude = z.number().min(-90).max(90);
const longitude = z.number().min(-180).max(180);

// Nominatim's usage policy: at most one request a second from the whole app, and a client name.
const NOMINATIM_GAP_MS = 1000;
let lastNominatim = 0;

const nominatimRows = z.array(z.object({ display_name: z.string(), lat: z.coerce.number().pipe(latitude), lon: z.coerce.number().pipe(longitude) }));
const awsItem = z.object({ Title: z.string(), Address: z.object({ Label: z.string() }).optional(), Position: z.tuple([longitude, latitude]).optional() });
const awsRows = z.object({ ResultItems: z.array(awsItem).default([]) });

async function nominatim(query: string, signal: AbortSignal): Promise<Place[]> {
  const url = new URL("https://nominatim.openstreetmap.org/search");
  url.search = new URLSearchParams({ q: query, format: "jsonv2", limit: String(MAX_RESULTS) }).toString();
  const res = await fetch(url, { signal, headers: { "User-Agent": "ac-project-admin/1.0", Accept: "application/json" }, cache: "no-store" });
  if (!res.ok) throw new Error(`nominatim ${String(res.status)}`);
  return nominatimRows.parse(await res.json()).map((r) => ({ label: r.display_name, location: pointOf(r.lat, r.lon) }));
}

async function amazon(g: Extract<Geocoder, { kind: "aws" }>, query: string, signal: AbortSignal): Promise<Place[]> {
  const url = new URL(`https://places.geo.${g.region}.amazonaws.com/v2/search-text`);
  url.searchParams.set("key", g.key);
  const res = await fetch(url, { signal, method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ QueryText: query, MaxResults: MAX_RESULTS }), cache: "no-store" });
  if (!res.ok) throw new Error(`amazon location ${String(res.status)}`);
  type Item = z.infer<typeof awsItem>;
  const placed = awsRows.parse(await res.json()).ResultItems.filter((r): r is Item & { Position: [number, number] } => r.Position !== undefined); // a place without a position cannot be pinned
  return placed.map((r) => ({ label: r.Address?.Label ?? r.Title, location: pointOf(r.Position[1], r.Position[0]) }));
}

/** The places for an address (2–200 characters after trim). The query is a Server Action's argument, so it comes from
the network: anything but a string is no query. */
export async function searchPlaces(query: unknown, now: () => number = Date.now): Promise<SearchResult> {
  const g = geocoderOf();
  if (g.kind === "none") return { ok: false, reason: "off" };
  const q = z.string().catch("").parse(query).trim().slice(0, 200);
  if (q.length < 2) return { ok: true, places: [] };
  if (g.kind === "nominatim") {
    if (now() - lastNominatim < NOMINATIM_GAP_MS) return { ok: false, reason: "busy" };
    lastNominatim = now();
  }
  try {
    const signal = AbortSignal.timeout(TIMEOUT_MS);
    return { ok: true, places: g.kind === "aws" ? await amazon(g, q, signal) : await nominatim(q, signal) };
  } catch (error) {
    console.warn("geocode failed:", Error.isError(error) ? error.message : "unknown");
    return { ok: false, reason: "unavailable" };
  }
}
