"use client";
// The property pin (DEC-109, IR824): a MapLibre map where a click places the pin and a drag moves it, the latitude and
// longitude typed (the keyboard way, and the only way when the deployment has no map), the browser's own position,
// and an address search on the server. PinMap alone shows a saved pin on the property's page. The page that uses them
// imports maplibre-gl/dist/maplibre-gl.css (an app's own CSS import).
import { useEffect, useEffectEvent, useRef, useState } from "react";
import type { Map as MapLibreMap, Marker } from "maplibre-gl";
import { Btn, Field, Input, Plain } from "@ac/web/components/ui";
import { Icon } from "@ac/web/components/Icon";
import { useT } from "@ac/web/components/I18n";
import type { GeoPoint } from "@ac/web/lib/assets";
import { DEFAULT_CENTER, parsePoint, pointOf } from "@ac/web/lib/geoPoint";
import type { Place, SearchResult } from "@ac/web/lib/geocode";

// MapLibre finds its worker next to its own module, which in a bundled chunk is a file that does not exist. The bundler
// copies the worker when it sees this URL, so the map starts it from the app's own origin (worker-src 'self').
// eslint-disable-next-line unicorn/no-missing-local-resource -- a package path the bundler resolves in node_modules, not a file beside this one
const WORKER_URL = new URL("maplibre-gl/dist/maplibre-gl-worker.mjs", import.meta.url).href;
const PIN_ZOOM = 16;
const AREA_ZOOM = 11;
const lngLat = (p: GeoPoint): [number, number] => [p.longitude, p.latitude];

/** A MapLibre map with the pin. With onPick a click places the pin and a drag moves it; without, it only shows it. */
export function PinMap({ styleUrl, point, onPick, label, className = "h-64 w-full overflow-hidden rounded-md border" }: Readonly<{ styleUrl: string; point: GeoPoint | null; onPick?: (p: GeoPoint) => void; label: string; className?: string }>) {
  const t = useT();
  const box = useRef<HTMLDivElement>(null);
  const map = useRef<MapLibreMap | null>(null);
  const marker = useRef<Marker | null>(null);
  const [failed, setFailed] = useState(false);
  const picked = useRef<GeoPoint | null>(null); // the pin the map itself placed: the view stays where the user put it
  const pick = useEffectEvent((p: GeoPoint) => {
    picked.current = p;
    onPick?.(p);
  });
  const start = useEffectEvent(() => ({ point, editable: !!onPick }));

  useEffect(() => {
    let cancelled = false;
    let created: MapLibreMap | null = null;
    import("maplibre-gl").then((maplibre) => {
      const container = box.current;
      if (cancelled || !container) return;
      const first = start();
      maplibre.setWorkerUrl(WORKER_URL);
      created = new maplibre.Map({ container, style: styleUrl, center: lngLat(first.point ?? DEFAULT_CENTER), zoom: first.point ? PIN_ZOOM : AREA_ZOOM, attributionControl: { compact: true } });
      created.addControl(new maplibre.NavigationControl({ showCompass: false }), "top-right");
      const pin = new maplibre.Marker({ draggable: first.editable, color: "#2563eb" });
      if (first.point) pin.setLngLat(lngLat(first.point)).addTo(created);
      if (first.editable) {
        pin.on("dragend", () => { const at = pin.getLngLat(); pick(pointOf(at.lat, at.lng)); });
        created.on("click", (e) => { pick(pointOf(e.lngLat.lat, e.lngLat.lng)); });
      }
      map.current = created;
      marker.current = pin;
    }).catch(() => { if (!cancelled) setFailed(true); });
    return () => {
      cancelled = true;
      created?.remove();
      map.current = null;
      marker.current = null;
    };
  }, [styleUrl]);

  useEffect(() => { // follow a pin set by typing, the search or the browser's position, and zoom in on it
    const m = map.current;
    const pin = marker.current;
    if (!m || !pin) return;
    if (!point) { pin.remove(); return; }
    pin.setLngLat(lngLat(point)).addTo(m);
    const fromMap = picked.current?.latitude === point.latitude && picked.current.longitude === point.longitude;
    if (fromMap) return;
    m.easeTo({ center: lngLat(point), zoom: Math.max(m.getZoom(), PIN_ZOOM) });
  }, [point]);

  return failed
    ? <p className="text-sm text-muted-foreground">{t("The map could not be loaded. Type the latitude and longitude instead.")}</p>
    : <div ref={box} role="region" aria-label={label} className={className} />;
}

/** The pin's editor of the property form. `onChange` gets the pin (null: none) and whether the typed values are valid. */
export function LocationPicker({ value, onChange, styleUrl, onSearch, address }: Readonly<{
  value: GeoPoint | null; onChange: (point: GeoPoint | null, valid: boolean) => void; styleUrl: string | null;
  onSearch: ((query: string) => Promise<SearchResult>) | null; address: string;
}>) {
  const t = useT();
  const [lat, setLat] = useState(value ? String(value.latitude) : "");
  const [lng, setLng] = useState(value ? String(value.longitude) : "");
  const [invalid, setInvalid] = useState(false);
  const [query, setQuery] = useState(address);
  const [places, setPlaces] = useState<Place[] | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const place = (p: GeoPoint | null) => {
    setLat(p ? String(p.latitude) : "");
    setLng(p ? String(p.longitude) : "");
    setInvalid(false);
    onChange(p, true);
  };
  const typed = (a: string, b: string) => {
    setLat(a);
    setLng(b);
    const parsed = parsePoint(a, b);
    setInvalid(!parsed.ok);
    onChange(parsed.ok ? parsed.point : value, parsed.ok);
  };
  const here = () => {
    setNote(null);
    if (!("geolocation" in navigator)) { setNote(t("This browser cannot share its location.")); return; }
    navigator.geolocation.getCurrentPosition(
      (pos) => { place(pointOf(pos.coords.latitude, pos.coords.longitude)); setNote(t("Placed at this device’s location (accuracy about {n} m).", { n: Math.round(pos.coords.accuracy) })); },
      () => { setNote(t("The browser did not share its location.")); },
      { enableHighAccuracy: true, timeout: 10_000 },
    );
  };
  const canSearch = !busy && query.trim().length >= 2; // the button and Enter alike (IR835)
  const search = async () => {
    if (!onSearch || !canSearch) return;
    setBusy(true);
    setNote(null);
    let r: SearchResult;
    try {
      r = await onSearch(query);
    } catch {
      r = { ok: false, reason: "unavailable" }; // the Server Action call itself failed (network, server error)
    } finally {
      setBusy(false);
    }
    if (r.ok) {
      setPlaces(r.places);
      if (r.places.length === 0) setNote(t("No place found for this address."));
      return;
    }
    setPlaces(null);
    setNote(r.reason === "busy" ? t("Searching too fast. Wait a second and search again.") : t("The address search is not available. Place the pin on the map or type it."));
  };

  return (
    <div className="flex flex-col gap-2">
      {onSearch && (
        <div className="flex flex-col gap-1">
          <div className="flex gap-2">
            <Input aria-label={t("Search an address")} value={query} placeholder={t("Search an address")} onChange={(x) => { setQuery(x.target.value); }}
              onKeyDown={(x) => {
                if (x.key !== "Enter") return;
                x.preventDefault();
                void search();
              }} />
            <Btn disabled={!canSearch} onClick={() => { void search(); }}><Icon name="search" /> {t("Search")}</Btn>
          </div>
          {places && places.length > 0 && (
            <ul className="flex flex-col rounded-md border" aria-label={t("Places found")}>
              {places.map((p) => (
                <li key={`${String(p.location.latitude)},${String(p.location.longitude)}`}>
                  <Plain className="w-full px-3 py-1.5 text-left text-sm hover:bg-muted focus-visible:bg-muted" onClick={() => { place(p.location); setPlaces(null); }}>{p.label}</Plain>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      {styleUrl && <PinMap styleUrl={styleUrl} point={value} onPick={place} label={t("Map: click to place the pin, drag the pin to move it")} />}
      <div className="grid grid-cols-2 gap-2">
        <Field label={t("Latitude")} error={invalid ? t("Latitude −90 to 90 and longitude −180 to 180, in degrees") : undefined}><Input inputMode="decimal" value={lat} onChange={(x) => { typed(x.target.value, lng); }} /></Field>
        <Field label={t("Longitude")}><Input inputMode="decimal" value={lng} onChange={(x) => { typed(lat, x.target.value); }} /></Field>
      </div>
      <div className="flex flex-wrap gap-2">
        <Btn size="sm" onClick={here}><Icon name="location" /> {t("Use my location")}</Btn>
        {(value !== null || lat !== "" || lng !== "") && <Btn size="sm" onClick={() => { place(null); }}>{t("Remove pin")}</Btn>}
      </div>
      {note && <p className="text-sm text-muted-foreground" role="status">{note}</p>}
    </div>
  );
}
