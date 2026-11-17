"use client";

import { useEffect, useEffectEvent, useState, useSyncExternalStore } from "react";
import { Toggle } from "@ac/web/components/ui";
import { useT } from "@ac/web/components/I18n";
import { presenceReporter, presenceStored, presenceText, storePresence, subscribePresence, type Fix } from "@ac/web/lib/presence";
import { reportPresence } from "../actions";

/** Whether the browser has geolocation does not change while the page is open. */
const unsubscribe = (): void => {
  // nothing was subscribed
};
const noChange = (): (() => void) => unsubscribe;

/** "Use this device's location" (DEC-109, IR826): while this page is open and the switch is on, the browser's position is
reported when it moved 50 m or every 5 minutes; the Core API keeps only home or away. A browser does not report in the
background, so the line says so. The choice is this device's. */
export function PresenceSwitch({ places }: Readonly<{ places: Record<string, string> }>) {
  const t = useT();
  const on = useSyncExternalStore(subscribePresence, presenceStored, () => false); // the browser's choice; off on the server
  // a browser without geolocation never starts a watch, so the switch says so instead of staying silent (IR842)
  const located = useSyncExternalStore(noChange, () => "geolocation" in navigator, () => true);
  const [line, setLine] = useState<string | null>(null);
  const report = useEffectEvent(async (fix: Fix, current: () => boolean): Promise<boolean> => {
    // a server action rejects when the network is down: the same line as a refused report
    const r = await reportPresence({ latitude: fix.latitude, longitude: fix.longitude, accuracyMeters: fix.accuracyMeters }).catch(() => null);
    // the answer to a report of a watch that has ended (the switch turned off, or off and on again) says nothing (IR842)
    if (!current()) return false;
    if (!r?.ok) { setLine(t("This device’s location could not be sent — the next one is tried in a moment.")); return false; }
    setLine(r.value ? presenceText(r.value, new Map(Object.entries(places)), t) ?? t("Location sent · nothing changed.") : t("Location sent · nothing changed."));
    return true;
  });
  const unavailable = useEffectEvent(() => { setLine(t("This browser did not share its location — allow it for this site, or switch this off.")); });
  useEffect(() => {
    let current = true;
    const offer = presenceReporter((fix) => report(fix, () => current));
    const watch = on && "geolocation" in navigator ? navigator.geolocation.watchPosition(
      (p) => { offer({ latitude: p.coords.latitude, longitude: p.coords.longitude, accuracyMeters: p.coords.accuracy, at: Date.now() }); },
      () => { unavailable(); },
      { enableHighAccuracy: true, maximumAge: 60_000, timeout: 30_000 },
    ) : null;
    return () => { current = false; if (watch !== null) navigator.geolocation.clearWatch(watch); };
  }, [on]);
  const flip = (next: boolean) => { storePresence(next); setLine(null); };
  const shown = located ? line : t("This browser cannot share a location — switch this off, or use another browser.");
  return (
    <div className="mt-3 flex flex-col gap-1 border-t pt-3">
      <div className="flex items-center gap-2 text-[13px]"><Toggle on={on} onChange={flip} label={t("Use this device’s location")} /><span>{t("Use this device’s location")}</span></div>
      <p className="text-xs text-muted-foreground">{t("While this page is open, your position is compared with the map pins of your homes and offices; only home or away is kept. A browser does not report in the background.")}</p>
      {on && shown && <p className="text-xs" role="status">{shown}</p>}
    </div>
  );
}
