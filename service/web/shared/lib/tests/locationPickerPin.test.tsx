// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";

// The property pin (DEC-109, IR824): the typed latitude and longitude with their check, Remove pin, the browser's own
// position (shared, refused, unsupported), the places an address search finds and the busy answer, and the map —
// a click places the pin and a drag moves it, a pin set elsewhere is followed, and a map that cannot load says so.
const ml = vi.hoisted(() => {
  type Handler = (e: { lngLat: { lat: number; lng: number } }) => void;
  const state = { fail: false, clicks: [] as Handler[], drags: [] as (() => void)[], eased: [] as unknown[], removed: 0, pinAt: { lat: 0, lng: 0 }, onMap: false, workerUrl: "" };
  class Marker {
    on(_: string, f: () => void) { state.drags.push(f); return this; }
    setLngLat([lng, lat]: [number, number]) { state.pinAt = { lat, lng }; return this; }
    getLngLat() { return state.pinAt; }
    addTo() { state.onMap = true; return this; }
    remove() { state.onMap = false; return this; }
  }
  class MapStub {
    constructor() { if (state.fail) throw new Error("no WebGL"); }
    addControl() { return this; }
    on(_: string, f: Handler) { state.clicks.push(f); return this; }
    getZoom() { return 11; }
    easeTo(o: unknown) { state.eased.push(o); }
    remove() { state.removed++; }
  }
  class NavigationControl { readonly showCompass = false; }
  const setWorkerUrl = (url: string) => { state.workerUrl = url; };
  return { state, module: { Marker, NavigationControl, setWorkerUrl, Map: MapStub } };
});
vi.mock("maplibre-gl", () => ml.module);

import { LocationPicker, PinMap } from "@ac/web/components/LocationPicker";
import type { GeoPoint } from "@ac/web/lib/assets";
import type { SearchResult } from "@ac/web/lib/geocode";

const flush = () => act(() => new Promise<void>((r) => { setTimeout(r, 0); }));
const kl: GeoPoint = { latitude: 3.1478, longitude: 101.6953 };
const field = (name: RegExp) => screen.getByRole<HTMLInputElement>("textbox", { name });

afterEach(() => {
  cleanup();
  Object.assign(ml.state, { fail: false, clicks: [], drags: [], eased: [], removed: 0, onMap: false });
  vi.unstubAllGlobals();
});

describe("the typed pin", () => {
  it("accepts a latitude and longitude, refuses one out of range, and removes the pin", () => {
    const onChange = vi.fn();
    render(<LocationPicker value={null} onChange={onChange} styleUrl={null} onSearch={null} address="" />);
    expect(screen.queryByRole("button", { name: "Remove pin" })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Search an address" })).toBeNull(); // no search on the server: no box
    fireEvent.change(field(/^Latitude/), { target: { value: "3.1478" } });
    fireEvent.change(field(/^Longitude/), { target: { value: "101.6953" } });
    expect(onChange).toHaveBeenLastCalledWith(kl, true);
    fireEvent.change(field(/^Latitude/), { target: { value: "95" } });
    expect(onChange).toHaveBeenLastCalledWith(null, false); // the last valid value stays the caller's
    expect(field(/^Latitude/).getAttribute("aria-invalid")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Remove pin" }));
    expect(onChange).toHaveBeenLastCalledWith(null, true);
    expect([field(/^Latitude/).value, field(/^Longitude/).value]).toStrictEqual(["", ""]);
  });
});

describe("the browser's own position", () => {
  it("places the pin where the device is, says the browser refused, or that it cannot share", () => {
    const onChange = vi.fn();
    const answers: ("ok" | "no")[] = ["ok", "no"];
    vi.stubGlobal("navigator", {
      geolocation: {
        getCurrentPosition: (ok: (p: { coords: { latitude: number; longitude: number; accuracy: number } }) => void, no: () => void) => {
          if (answers.shift() === "ok") ok({ coords: { latitude: kl.latitude, longitude: kl.longitude, accuracy: 12.4 } });
          else no();
        },
      },
    });
    render(<LocationPicker value={kl} onChange={onChange} styleUrl={null} onSearch={null} address="" />);
    fireEvent.click(screen.getByRole("button", { name: /Use my location/ }));
    expect(onChange).toHaveBeenLastCalledWith(kl, true);
    expect(screen.getByRole("status").textContent).toBe("Placed at this device’s location (accuracy about 12 m).");
    fireEvent.click(screen.getByRole("button", { name: /Use my location/ }));
    expect(screen.getByRole("status").textContent).toBe("The browser did not share its location.");
    vi.stubGlobal("navigator", {});
    fireEvent.click(screen.getByRole("button", { name: /Use my location/ }));
    expect(screen.getByRole("status").textContent).toBe("This browser cannot share its location.");
  });
});

describe("the address search", () => {
  it("lists the places found and places the pin on the one chosen", async () => {
    const onChange = vi.fn();
    const onSearch = vi.fn((): Promise<SearchResult> => Promise.resolve({ ok: true, places: [{ label: "Menara KL", location: kl }, { label: "KLCC", location: { latitude: 3.158, longitude: 101.712 } }] }));
    render(<LocationPicker value={null} onChange={onChange} styleUrl={null} onSearch={onSearch} address="Jalan Ampang" />);
    fireEvent.click(screen.getByRole("button", { name: /Search/ }));
    await flush();
    expect(screen.getAllByRole("listitem").map((li) => li.textContent)).toStrictEqual(["Menara KL", "KLCC"]);
    fireEvent.click(screen.getByRole("button", { name: "Menara KL" }));
    expect(onChange).toHaveBeenLastCalledWith(kl, true);
    expect(screen.queryByRole("list", { name: "Places found" })).toBeNull();
  });

  it("asks to wait when searching too fast", async () => {
    render(<LocationPicker value={null} onChange={vi.fn()} styleUrl={null} onSearch={() => Promise.resolve({ ok: false, reason: "busy" })} address="Jalan Ampang" />);
    fireEvent.change(screen.getByRole("textbox", { name: "Search an address" }), { target: { value: "Jalan Tun Razak" } });
    fireEvent.click(screen.getByRole("button", { name: /Search/ }));
    await flush();
    expect(screen.getByRole("status").textContent).toBe("Searching too fast. Wait a second and search again.");
  });
});

describe("the map", () => {
  it("places the pin with a click, moves it with a drag, and follows a pin set elsewhere", async () => {
    const onChange = vi.fn();
    const { rerender } = render(<LocationPicker value={null} onChange={onChange} styleUrl="https://tiles.example/style.json" onSearch={null} address="" />);
    await flush();
    expect(screen.getByRole("region", { name: "Map: click to place the pin, drag the pin to move it" })).toBeDefined();
    act(() => { ml.state.clicks[0]?.({ lngLat: { lat: 3.2, lng: 101.6 } }); });
    expect(onChange).toHaveBeenLastCalledWith({ latitude: 3.2, longitude: 101.6 }, true);
    rerender(<LocationPicker value={{ latitude: 3.2, longitude: 101.6 }} onChange={onChange} styleUrl="https://tiles.example/style.json" onSearch={null} address="" />);
    expect([ml.state.onMap, ml.state.eased]).toStrictEqual([true, []]); // placed by the map: the view stays where the user put it
    ml.state.pinAt = { lat: 3.25, lng: 101.65 };
    act(() => { ml.state.drags[0]?.(); });
    expect(onChange).toHaveBeenLastCalledWith({ latitude: 3.25, longitude: 101.65 }, true);
    rerender(<LocationPicker value={kl} onChange={onChange} styleUrl="https://tiles.example/style.json" onSearch={null} address="" />);
    expect(ml.state.eased).toStrictEqual([{ center: [kl.longitude, kl.latitude], zoom: 16 }]); // typed or searched: the map goes there
    rerender(<LocationPicker value={null} onChange={onChange} styleUrl="https://tiles.example/style.json" onSearch={null} address="" />);
    expect(ml.state.onMap).toBe(false);
  });

  it("shows a saved pin without editing, and says when the map cannot load", async () => {
    const { unmount } = render(<PinMap styleUrl="https://tiles.example/style.json" point={kl} label="Property map" />);
    await flush();
    expect([ml.state.onMap, ml.state.clicks.length, ml.state.drags.length]).toStrictEqual([true, 0, 0]);
    unmount();
    expect(ml.state.removed).toBe(1);
    ml.state.fail = true;
    render(<PinMap styleUrl="https://tiles.example/style.json" point={null} label="Property map" />);
    await flush();
    expect(screen.getByText("The map could not be loaded. Type the latitude and longitude instead.")).toBeDefined();
  });
});
