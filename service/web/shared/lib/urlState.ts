"use client";

// URL query state as an external store (React: useSyncExternalStore for browser APIs). Reading the URL this way needs
// no effect and no Suspense boundary: the server snapshot is empty, the client snapshot is location.search, and
// writes use history.replaceState plus a notification so every subscriber re-renders.
import { useSyncExternalStore } from "react";

const CHANGE = "ac:urlchange";

function subscribe(onChange: () => void) {
  addEventListener("popstate", onChange);
  addEventListener(CHANGE, onChange);
  return () => {
    removeEventListener("popstate", onChange);
    removeEventListener(CHANGE, onChange);
  };
}

/** The current query string ("" on the server). */
export function useSearch(): string {
  return useSyncExternalStore(subscribe, () => location.search, () => "");
}

/** One query parameter, or null. */
export function useSearchParameter(key: string): string | null {
  return new URLSearchParams(useSearch()).get(key);
}

/** Replaces one query parameter without navigation (D10 URL state). */
export function setSearchParameter(key: string, value: string | null) {
  const u = new URL(location.href);
  if (value === null) u.searchParams.delete(key);
  else u.searchParams.set(key, value);
  history.replaceState(null, "", u);
  dispatchEvent(new Event(CHANGE));
}

const STORAGE = "ac:storagechange";

/** A localStorage value as an external store (null on the server or when storage is blocked). */
export function useStoredValue(key: string): string | null {
  return useSyncExternalStore(
    (onChange) => {
      addEventListener("storage", onChange);
      addEventListener(STORAGE, onChange);
      return () => {
        removeEventListener("storage", onChange);
        removeEventListener(STORAGE, onChange);
      };
    },
    () => {
      try {
        return localStorage.getItem(key);
      } catch {
        return null;
      }
    },
    () => null,
  );
}

export function setStoredValue(key: string, value: string) {
  try {
    if (localStorage.getItem(key) === value) return;
    localStorage.setItem(key, value);
    dispatchEvent(new Event(STORAGE));
  } catch {}
}
