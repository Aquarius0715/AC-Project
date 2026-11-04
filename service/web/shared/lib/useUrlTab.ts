"use client";

import { setSearchParameter, useSearchParameter } from "@ac/web/lib/urlState";

/**
Tab state kept in the URL key `tab` (screen-catalog allowed_tabs, D10 URL state).
`map` translates the page's internal tab ids to the catalogued URL values. The URL is the single source of truth,
read through an external store (no effect, no Suspense boundary) and written with replaceState.
*/
export function useUrlTab<T extends string>(map: Record<T, string>, fallback: T): [T, (t: T) => void] {
  const v = useSearchParameter("tab");
  const tab = (Object.keys(map) as T[]).find((key) => map[key] === v) ?? fallback;
  return [tab, (t: T) => { setSearchParameter("tab", map[t]); }];
}
