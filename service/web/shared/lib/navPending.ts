"use client";

import { useEffect, useSyncExternalStore } from "react";

// How many URL patches are still waiting for the server's render (UI/UX loop R76). One shared count, so the app shell can
// show a progress line for a change made anywhere on the page.
let pending = 0;
const listeners = new Set<() => void>();
const notify = () => { for (const l of listeners) l(); };
const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l); }; };
const snapshot = () => pending > 0;
const serverSnapshot = () => false;

/** Counts this component's pending navigation while `isPending` is true. */
export function useReportNavPending(isPending: boolean): void {
  useEffect(() => {
    // 1 while pending, 0 otherwise; a notify with an unchanged count re-renders nothing (the snapshot is the same)
    const delta = isPending ? 1 : 0;
    pending += delta;
    notify();
    return () => { pending -= delta; notify(); };
  }, [isPending]);
}

/** Whether any URL patch on the page is still waiting for its render. */
export function useNavPending(): boolean {
  return useSyncExternalStore(subscribe, snapshot, serverSnapshot);
}
