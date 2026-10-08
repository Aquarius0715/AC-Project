"use client";

import { useEffect, useState } from "react";

/**
 * Tab state kept in the URL key `tab` (screen-catalog allowed_tabs, D10 URL state).
 * `map` translates the page's internal tab ids to the catalogued URL values.
 * The URL is read after mount (hydration-safe, no Suspense needed) and updated with replaceState.
 */
export function useUrlTab<T extends string>(map: Record<T, string>, fallback: T): [T, (t: T) => void] {
  const [tab, setTab] = useState<T>(fallback);
  useEffect(() => {
    const v = new URLSearchParams(window.location.search).get("tab");
    const k = (Object.keys(map) as T[]).find((key) => map[key] === v);
    if (k) setTab(k);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const set = (t: T) => {
    setTab(t);
    const u = new URL(window.location.href);
    u.searchParams.set("tab", map[t]);
    window.history.replaceState(null, "", u);
  };
  return [tab, set];
}
