// The customer's away/home presence (DEC-109, IR826): when the browser reports again, the result's words, and the
// result checked where it enters the browser.
import { afterEach, describe, expect, it, vi } from "vitest";
import { parsePresenceResult, presenceReporter, presenceStored, presenceText, shouldReport, storePresence, subscribePresence, type Fix } from "@ac/web/lib/presence";
import { haversine } from "@ac/web/lib/geoPoint";
import { translator } from "@ac/web/lib/i18n";

const t = translator("en");
const at = (latitude: number, accuracyMeters: number, minute: number): Fix => ({ latitude, accuracyMeters, longitude: 101.7116, at: minute * 60_000 });
const HOME = "p-home";
const names = new Map([[HOME, "Home A"]]);
const result = (event: "arrival" | "departure", n: number) => ({ transitions: [{ event, propertyId: HOME, commandIds: Array.from({ length: n }, (_, i) => `c${String(i)}`) }] });

describe("presence reports", () => {
  it("measures the great-circle distance as the Core API does", () => {
    expect(Math.round(haversine({ latitude: 3.1579, longitude: 101.7116 }, { latitude: 3.1597, longitude: 101.7116 }))).toBe(200);
    expect(Math.round(haversine({ latitude: 3.139, longitude: 101.6869 }, { latitude: 1.3521, longitude: 103.8198 }))).toBe(309_253);
  });
  it("sends the first precise fix, then after 50 m or 5 minutes, and never a coarse one", () => {
    expect(shouldReport(null, at(3.1579, 20, 0))).toBe(true);
    expect(shouldReport(null, at(3.1579, 120, 0))).toBe(false);
    const last = at(3.1579, 20, 0);
    expect(shouldReport(last, at(3.1582, 20, 1))).toBe(false); // 33 m, a minute later
    expect(shouldReport(last, at(3.1584, 20, 1))).toBe(true); // 56 m
    expect(shouldReport(last, at(3.1579, 20, 5))).toBe(true); // the same place, 5 minutes later
  });
  it("words an arrival or a departure with the automations it ran, and nothing for no change", () => {
    expect(presenceText(result("arrival", 0), names, t)).toBe("Arrived at Home A.");
    expect(presenceText(result("arrival", 1), names, t)).toBe("Arrived at Home A — 1 automation ran.");
    expect(presenceText(result("departure", 2), names, t)).toBe("Left Home A — 2 automations ran.");
    expect(presenceText({ transitions: [] }, names, t)).toBeNull();
    expect(presenceText({ transitions: [{ propertyId: "other", event: "departure", commandIds: [] }] }, names, t)).toBe("Left your property.");
  });
  it("takes only a PresenceResult from the Core API", () => {
    expect(parsePresenceResult({ transitions: [{ propertyId: HOME, event: "arrival", commandIds: [] }] })).not.toBeNull();
    expect(parsePresenceResult({ transitions: [{ propertyId: HOME, event: "teleport", commandIds: [] }] })).toBeNull();
    expect(parsePresenceResult(null)).toBeNull();
  });
});

// a send the test answers: true when the report was sent, false when it failed
const deferredSend = () => {
  const answers: ((sent: boolean) => void)[] = [];
  const send = vi.fn<(fix: Fix) => Promise<boolean>>(() => new Promise<boolean>((resolve) => { answers.push(resolve); }));
  const answer = async (sent: boolean) => { answers.shift()?.(sent); await new Promise((resolve) => { setTimeout(resolve, 0); }); };
  return { send, answer };
};
const blocked = () => { throw new DOMException("blocked", "SecurityError"); };

describe("presence reporter", () => {
  it("tries again on the next fix after a failed report, even in the same place", async () => {
    const { send, answer } = deferredSend();
    const offer = presenceReporter(send);
    offer(at(3.1579, 20, 0));
    await answer(false);
    offer(at(3.1579, 20, 1)); // the same place a minute later: retried, because nothing was sent
    expect(send).toHaveBeenCalledTimes(2);
    await answer(true);
    offer(at(3.1579, 20, 2)); // now sent: the same place waits for 50 m or 5 minutes
    expect(send).toHaveBeenCalledTimes(2);
  });
  it("sends one report at a time and drops fixes while one is in flight", async () => {
    const { send, answer } = deferredSend();
    const offer = presenceReporter(send);
    offer(at(3.1579, 20, 0));
    offer(at(3.17, 20, 0)); // 1.3 km away, but the first is still in flight
    expect(send).toHaveBeenCalledOnce();
    await answer(true);
    offer(at(3.17, 20, 1));
    expect(send).toHaveBeenCalledTimes(2);
  });
  it("tries again after a send that threw (a server action rejects when the network is down)", async () => {
    const send = vi.fn<(fix: Fix) => Promise<boolean>>(() => Promise.reject(new TypeError("Failed to fetch")));
    const offer = presenceReporter(send);
    offer(at(3.1579, 20, 0));
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    offer(at(3.1579, 20, 1));
    expect(send).toHaveBeenCalledTimes(2);
  });
  it("never sends a coarse fix", () => {
    const { send } = deferredSend();
    presenceReporter(send)(at(3.1579, 120, 0));
    expect(send).not.toHaveBeenCalled();
  });
});

describe("presence choice on this device", () => {
  afterEach(() => { vi.restoreAllMocks(); storePresence(false); });
  it("keeps the choice in localStorage and tells each subscriber until it unsubscribes", () => {
    const listener = vi.fn();
    const unsubscribe = subscribePresence(listener);
    storePresence(true);
    expect(localStorage.getItem("ac.presence.on")).toBe("1");
    expect(presenceStored()).toBe(true);
    storePresence(false);
    expect(localStorage.getItem("ac.presence.on")).toBeNull();
    expect(presenceStored()).toBe(false);
    expect(listener).toHaveBeenCalledTimes(2);
    unsubscribe();
    storePresence(true);
    expect(listener).toHaveBeenCalledTimes(2);
  });
  it("reads only \"1\" as on", () => {
    localStorage.setItem("ac.presence.on", "true");
    expect(presenceStored()).toBe(false);
  });
  it("keeps the choice for this page when storage is blocked", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(blocked);
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(blocked);
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(blocked);
    storePresence(true);
    expect(presenceStored()).toBe(true);
    storePresence(false);
    expect(presenceStored()).toBe(false);
  });
});
