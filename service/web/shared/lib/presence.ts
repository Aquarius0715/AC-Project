// The customer's away/home presence (DEC-109, IR826): while the automations page is open and the member switched it on,
// the browser reports the device's position; the Core API compares it with the pins of the customer's homes and
// offices and keeps only home or away. Pure code, unit-tested.
import { z } from "zod";
import { haversine } from "@ac/web/lib/geoPoint";
import type { T } from "@ac/web/lib/i18n";

/** A fix the browser reported: WGS 84 degrees, its accuracy in metres and when it was taken. */
export interface Fix { latitude: number; longitude: number; accuracyMeters: number; at: number }

/** The coarsest fix worth sending (the Core API refuses coarser ones). */
const MAX_ACCURACY = 100;
/** A report again after moving this far, in metres: less is the same place. */
const MOVED = 50;
/** A report again after this long in the same place, so a missed report does not leave the state behind. */
const AGAIN_MS = 5 * 60_000;

/** Whether a new fix is worth a report: precise enough, and the first one, moved away, or a while since the last. */
export function shouldReport(last: Fix | null, fix: Fix): boolean {
  if (fix.accuracyMeters > MAX_ACCURACY) return false;
  if (!last) return true;
  return haversine(last, fix) >= MOVED || fix.at - last.at >= AGAIN_MS;
}

/** The fixes of one watch, sent one at a time: a fix is compared with the last one that was sent, so a failed report
(network, 503, a refusal) is tried again with the next fix instead of waiting for 50 m or 5 minutes. `send` answers
whether the report was sent. */
export function presenceReporter(send: (fix: Fix) => Promise<boolean>): (fix: Fix) => void {
  let last: Fix | null = null;
  let inFlight = false;
  return (fix) => {
    if (inFlight || !shouldReport(last, fix)) return;
    inFlight = true;
    // a send that threw (a server action rejects when the network is down) was not sent either
    void send(fix).catch(() => false).then((sent) => {
      if (sent) last = fix;
    }).finally(() => { inFlight = false; });
  };
}

/** PresenceResult of service-contracts.ts, checked where it enters the browser. */
const transition = z.object({ propertyId: z.string(), event: z.enum(["arrival", "departure"]), commandIds: z.array(z.string()) });
const presenceResult = z.object({ transitions: z.array(transition) });
export type PresenceResult = z.infer<typeof presenceResult>;
export const parsePresenceResult = (value: unknown): PresenceResult | null => {
  const parsed = presenceResult.safeParse(value);
  return parsed.success ? parsed.data : null;
};

/** What the last report changed, in words: an arrival or a departure and how many automations it started. */
export function presenceText(result: PresenceResult, names: ReadonlyMap<string, string>, t: T): string | null {
  const first = result.transitions[0];
  if (!first) return null;
  const name = names.get(first.propertyId) ?? t("your property");
  const n = first.commandIds.length;
  const arrived = first.event === "arrival";
  if (n === 0) return arrived ? t("Arrived at {name}.", { name }) : t("Left {name}.", { name });
  if (n === 1) return arrived ? t("Arrived at {name} — 1 automation ran.", { name }) : t("Left {name} — 1 automation ran.", { name });
  return arrived ? t("Arrived at {name} — {n} automations ran.", { name, n }) : t("Left {name} — {n} automations ran.", { name, n });
}

/** The member's choice on this device, kept in the browser (a per-device convenience, read through Zod) and read with
useSyncExternalStore. Where storage is blocked (a private window) it lasts for this page only. */
const STORE_KEY = "ac.presence.on";
let inMemory = false;
const listeners = new Set<() => void>();
export function presenceStored(): boolean {
  try {
    return z.literal("1").safeParse(localStorage.getItem(STORE_KEY)).success;
  } catch {
    return inMemory;
  }
}
export function storePresence(on: boolean): void {
  inMemory = on;
  try {
    if (on) localStorage.setItem(STORE_KEY, "1");
    else localStorage.removeItem(STORE_KEY);
  } catch {
    // blocked storage: inMemory holds the choice
  }
  for (const listener of listeners) listener();
}
export function subscribePresence(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}
