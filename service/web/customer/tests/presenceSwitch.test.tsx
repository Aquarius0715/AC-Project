// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { storePresence } from "@ac/web/lib/presence";

// the away/home switch (DEC-109, IR826): a report's line, a report that is answered after the switch was turned off
// and on again, and a browser without geolocation (IR842, R954)
const actions = vi.hoisted(() => ({ reportPresence: vi.fn() }));
vi.mock("../app/customer/automations/actions", () => actions);
vi.mock("@ac/web/components/I18n", () => ({
  useT: () => (s: string, values?: Readonly<Record<string, string | number>>) => {
    if (!values) return s;
    let text = s;
    const entries = Object.entries(values);
    for (const [key, value] of entries) text = text.replaceAll(`{${key}}`, () => String(value));
    return text;
  },
}));
vi.mock("@ac/web/components/ui", () => ({
  Toggle: ({ on, onChange, label }: Readonly<{ on: boolean; onChange: (v: boolean) => void; label: string }>) => (
    <button type="button" role="switch" aria-checked={on} aria-label={label} onClick={() => { onChange(!on); }} />
  ),
}));

import { PresenceSwitch } from "../app/customer/automations/_components/presence-switch";

type Watcher = (p: { coords: { latitude: number; longitude: number; accuracy: number } }) => void;

const watchers = new Map<number, Watcher>();
const failures = new Map<number, () => void>();
const geolocation = {
  watchPosition: vi.fn((ok: Watcher, fail: () => void) => { const id = watchers.size + 1; watchers.set(id, ok); failures.set(id, fail); return id; }),
  clearWatch: vi.fn((id: number) => { watchers.delete(id); }),
};
const fix = (latitude: number) => ({ coords: { latitude, longitude: 101.7, accuracy: 10 } });
const latest = (): Watcher => {
  const last = watchers.values().toArray().at(-1);
  if (!last) throw new Error("no watch");
  return last;
};
const answer = (propertyId: string, event: "arrival" | "departure", commandIds: string[]) =>
  ({ ok: true, value: { transitions: [{ propertyId, event, commandIds }] } });
const flip = () => { fireEvent.click(screen.getByRole("switch")); };

beforeEach(() => {
  Object.defineProperty(navigator, "geolocation", { value: geolocation, configurable: true });
  storePresence(true);
});
afterEach(() => {
  cleanup();
  storePresence(false);
  watchers.clear();
  failures.clear();
  vi.clearAllMocks();
  Reflect.deleteProperty(navigator, "geolocation");
});

describe("PresenceSwitch", () => {
  it("says what a report did, and that a refused one is tried again", async () => {
    actions.reportPresence.mockResolvedValueOnce({ ok: true, value: null });
    actions.reportPresence.mockResolvedValueOnce({ ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} });
    render(<PresenceSwitch places={{}} />);
    await act(async () => { latest()(fix(3.1)); await Promise.resolve(); });
    expect(screen.getByRole("status").textContent).toBe("Location sent · nothing changed.");
    await act(async () => { latest()(fix(3.2)); await Promise.resolve(); });
    expect(screen.getByRole("status").textContent).toBe("This device’s location could not be sent — the next one is tried in a moment.");
  });

  it("names the property a report arrived at or left, and how many automations ran", async () => {
    actions.reportPresence.mockResolvedValueOnce(answer("prop-1", "arrival", ["cmd-1"]));
    actions.reportPresence.mockResolvedValueOnce(answer("prop-9", "departure", []));
    actions.reportPresence.mockResolvedValueOnce({ ok: true, value: { transitions: [] } });
    render(<PresenceSwitch places={{ "prop-1": "Home" }} />);
    await act(async () => { latest()(fix(3.1)); await Promise.resolve(); });
    expect(screen.getByRole("status").textContent).toBe("Arrived at Home — 1 automation ran.");
    await act(async () => { latest()(fix(3.2)); await Promise.resolve(); });
    expect(screen.getByRole("status").textContent).toBe("Left your property.");
    await act(async () => { latest()(fix(3.3)); await Promise.resolve(); });
    expect(screen.getByRole("status").textContent).toBe("Location sent · nothing changed.");
    expect(actions.reportPresence).toHaveBeenLastCalledWith({ latitude: 3.3, longitude: 101.7, accuracyMeters: 10 });
  });

  it("says a report that did not reach the server could not be sent", async () => {
    actions.reportPresence.mockRejectedValueOnce(new TypeError("Failed to fetch"));
    render(<PresenceSwitch places={{}} />);
    await act(async () => { latest()(fix(3.1)); await Promise.resolve(); });
    expect(screen.getByRole("status").textContent).toBe("This device’s location could not be sent — the next one is tried in a moment.");
  });

  it("says so when the browser does not share its location", () => {
    render(<PresenceSwitch places={{}} />);
    const fail = failures.values().toArray().at(-1);
    if (!fail) throw new Error("no watch");
    act(() => { fail(); });
    expect(screen.getByRole("status").textContent).toBe("This browser did not share its location — allow it for this site, or switch this off.");
    flip();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("renders off with no line on the server, whatever this browser chose", () => {
    const html = renderToString(<PresenceSwitch places={{}} />);
    expect(html).toContain('aria-checked="false"');
    expect(html).not.toContain('role="status"');
    expect(geolocation.watchPosition).not.toHaveBeenCalled();
  });

  it("ignores the answer to a report sent before the switch was turned off and on again", async () => {
    interface Result { ok: false; messageKey: string; code: string; fieldErrors: Record<string, string> }
    const first = Promise.withResolvers<Result>();
    actions.reportPresence.mockReturnValueOnce(first.promise);
    render(<PresenceSwitch places={{}} />);
    act(() => { latest()(fix(3.1)); });
    expect(actions.reportPresence).toHaveBeenCalledOnce();
    flip();
    flip();
    expect(geolocation.clearWatch).toHaveBeenCalledOnce();
    await act(async () => { first.resolve({ ok: false, messageKey: "error.unavailable", code: "UNAVAILABLE", fieldErrors: {} }); await first.promise; });
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says so when the browser cannot give a location at all", () => {
    Reflect.deleteProperty(navigator, "geolocation");
    render(<PresenceSwitch places={{}} />);
    expect(screen.getByRole("status").textContent).toBe("This browser cannot share a location — switch this off, or use another browser.");
    flip();
    expect(screen.queryByRole("status")).toBeNull();
  });
});
