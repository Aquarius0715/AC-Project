"use client";

// URL query state as an external store (React: useSyncExternalStore for browser APIs). Reading the URL this way needs
// no effect and no Suspense boundary: the server snapshot is empty, the client snapshot is location.search, and
// writes use history.replaceState plus a notification so every subscriber re-renders.
import { useSyncExternalStore } from "react";

const CHANGE = "ac:urlchange";

function subscribe(onChange: () => void) {
  globalThis.addEventListener("popstate", onChange);
  globalThis.addEventListener(CHANGE, onChange);
  return () => {
    globalThis.removeEventListener("popstate", onChange);
    globalThis.removeEventListener(CHANGE, onChange);
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
  globalThis.dispatchEvent(new Event(CHANGE));
}

const STORAGE = "ac:storagechange";

/** A localStorage value as an external store (null on the server or when storage is blocked). */
export function useStoredValue(key: string): string | null {
  return useSyncExternalStore(
    (onChange) => {
      globalThis.addEventListener("storage", onChange);
      globalThis.addEventListener(STORAGE, onChange);
      return () => {
        globalThis.removeEventListener("storage", onChange);
        globalThis.removeEventListener(STORAGE, onChange);
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
    globalThis.dispatchEvent(new Event(STORAGE));
  } catch {}
}
