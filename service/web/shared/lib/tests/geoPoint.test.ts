// The property pin's typed coordinates and radius, the map and address-search settings, and the address search
// (DEC-109, IR824).
// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { parsePoint, parseRadius, pointOf, showPoint } from "@ac/web/lib/geoPoint";
import { geocoderOf, mapConfig, mapOrigins, mapStyleUrl } from "@ac/web/lib/mapConfig";
import { searchPlaces } from "@ac/web/lib/geocode";
import { must } from "./must";

vi.mock("server-only", () => ({}));
afterEach(() => { vi.unstubAllEnvs(); vi.unstubAllGlobals(); });

const STYLE = "https://tiles.openfreemap.org/styles/liberty";

describe("the typed pin", () => {
  it("is no pin when both fields are empty, a rounded pin when both are numbers on the globe", () => {
    expect(parsePoint(" ", "")).toStrictEqual({ ok: true, point: null });
    expect(parsePoint("3.15790012", "101.7116")).toStrictEqual({ ok: true, point: { latitude: 3.1579, longitude: 101.7116 } });
    expect(parsePoint("-90", "+180")).toStrictEqual({ ok: true, point: { latitude: -90, longitude: 180 } });
  });
  it("refuses one empty field, words, exponents and points off the globe", () => {
    const refused: [string, string][] = [["3.1", ""], ["north", "1"], ["1e2", "1"], ["90.1", "0"], ["0", "-180.5"], ["Infinity", "0"]];
    for (const [a, b] of refused) expect(parsePoint(a, b), `${a},${b}`).toStrictEqual({ ok: false });
  });
  it("shows six decimals and rounds a map click to them", () => {
    expect(showPoint({ latitude: 3.1579, longitude: 101.7116 })).toBe("3.157900, 101.711600");
    expect(pointOf(3.123456789, -0.0000004)).toStrictEqual({ latitude: 3.123457, longitude: -0 });
  });
  it("takes a radius of 50–2000 whole metres, empty for the default", () => {
    expect([parseRadius(""), parseRadius("50"), parseRadius("2000")]).toStrictEqual([{ ok: true, radius: null }, { ok: true, radius: 50 }, { ok: true, radius: 2000 }]);
    for (const r of ["49", "2001", "150.5", "-60", "x"]) expect(parseRadius(r), r).toStrictEqual({ ok: false });
  });
});

describe("the map and search settings", () => {
  it("draws a map only for an HTTPS style, and allows only its origin", () => {
    expect([mapStyleUrl({ MAP_STYLE_URL: STYLE }), mapOrigins({ MAP_STYLE_URL: STYLE })]).toStrictEqual([STYLE, ["https://tiles.openfreemap.org"]]);
    // eslint-disable-next-line unicorn/prefer-https -- the plain-HTTP style below is the case refused
    for (const v of [undefined, "", "http://tiles.example.com/style.json", "javascript:alert(1)", "not a url"]) expect([mapStyleUrl({ MAP_STYLE_URL: v }), mapOrigins({ MAP_STYLE_URL: v })], v).toStrictEqual([null, []]);
  });
  it("searches only with a known provider, and Amazon Location only with its region and key", () => {
    expect([geocoderOf({}), geocoderOf({ GEOCODER: "google" }), geocoderOf({ GEOCODER: "nominatim" })]).toStrictEqual([{ kind: "none" }, { kind: "none" }, { kind: "nominatim" }]);
    expect(geocoderOf({ GEOCODER: "aws", AWS_LOCATION_REGION: "ap-southeast-5", AWS_LOCATION_API_KEY: "k" })).toStrictEqual({ kind: "aws", region: "ap-southeast-5", key: "k" });
    expect([geocoderOf({ GEOCODER: "aws", AWS_LOCATION_API_KEY: "k" }), geocoderOf({ GEOCODER: "aws", AWS_LOCATION_REGION: "evil.example/x", AWS_LOCATION_API_KEY: "k" })]).toStrictEqual([{ kind: "none" }, { kind: "none" }]);
    expect(mapConfig({ MAP_STYLE_URL: STYLE, GEOCODER: "nominatim" })).toStrictEqual({ styleUrl: STYLE, search: true });
  });
});

describe("the address search", () => {
  it("is off without a provider and sends nothing", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    expect(await searchPlaces("Jalan Ampang")).toStrictEqual({ ok: false, reason: "off" });
    expect(fetch).not.toHaveBeenCalled();
  });
  it("asks Amazon Location with the query alone and keeps the places that have a position", async () => {
    vi.stubEnv("GEOCODER", "aws");
    vi.stubEnv("AWS_LOCATION_REGION", "ap-southeast-5");
    vi.stubEnv("AWS_LOCATION_API_KEY", "public-key");
    const fetch = vi.fn<(url: URL, init: RequestInit) => Promise<Response>>(() => Promise.resolve(Response.json({ ResultItems: [{ Title: "KLCC", Address: { Label: "KLCC, Kuala Lumpur" }, Position: [101.7116, 3.1579] }, { Title: "No position" }] })));
    vi.stubGlobal("fetch", fetch);
    expect(await searchPlaces("  KLCC  ")).toStrictEqual({ ok: true, places: [{ label: "KLCC, Kuala Lumpur", location: { latitude: 3.1579, longitude: 101.7116 } }] });
    const [url, init] = must(fetch.mock.calls[0]);
    expect([url.origin + url.pathname, init.body]).toStrictEqual(["https://places.geo.ap-southeast-5.amazonaws.com/v2/search-text", JSON.stringify({ QueryText: "KLCC", MaxResults: 5 })]);
  });
  it("answers unavailable when the provider fails or answers what the schema does not take, and nothing for a short or non-string query", async () => {
    vi.stubEnv("GEOCODER", "aws");
    vi.stubEnv("AWS_LOCATION_REGION", "ap-southeast-5");
    vi.stubEnv("AWS_LOCATION_API_KEY", "public-key");
    vi.spyOn(console, "warn").mockImplementation(() => { /* the failures below are the cases */ });
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(Response.json({ ResultItems: [{ Title: "x", Position: [200, 3] }] }))));
    expect(await searchPlaces("KLCC")).toStrictEqual({ ok: false, reason: "unavailable" });
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response("", { status: 500 }))));
    expect(await searchPlaces("KLCC")).toStrictEqual({ ok: false, reason: "unavailable" });
    expect([await searchPlaces("K"), await searchPlaces({ q: "KLCC" })]).toStrictEqual([{ ok: true, places: [] }, { ok: true, places: [] }]);
  });
  it("asks Nominatim at most once a second", async () => {
    vi.stubEnv("GEOCODER", "nominatim");
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(Response.json([{ display_name: "Petronas Towers", lat: "3.1579", lon: "101.7116" }]))));
    let now = 10_000;
    expect(await searchPlaces("Petronas", () => now)).toStrictEqual({ ok: true, places: [{ label: "Petronas Towers", location: { latitude: 3.1579, longitude: 101.7116 } }] });
    now += 500;
    expect(await searchPlaces("Petronas", () => now)).toStrictEqual({ ok: false, reason: "busy" });
    now += 600;
    const third = await searchPlaces("Petronas", () => now);
    expect(third.ok).toBe(true);
  });
});
