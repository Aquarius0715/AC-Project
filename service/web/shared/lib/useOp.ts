"use client";

// Data-source switch for screens: in the Phase 1A demo (DATA_SOURCE=mock) screens keep their fixture data; with
// DATA_SOURCE=API the same screen reads the Core API through the BFF relay.
import { useEffect, useEffectEvent, useState } from "react";
import { callOp, OpError } from "@ac/web/lib/ops";
import type { OpArgs, OpCall } from "@ac/web/lib/opTypes";

export interface BffSession { dataSource: "mock" | "api"; signedIn: boolean; role: string | null; membershipId: string | null; clientRole: string | null; serverNow: string | null }

let cached: Promise<BffSession> | null = null;

// invalidation: writes bump the revision so every mounted useOp read refetches (IR71 lists simplified to "all")
let revision = 0;
const watchers = new Set<(r: number) => void>();
export function invalidate() {
  revision += 1;
  watchers.forEach((w) => { w(revision); });
}
function useRevision(): number {
  const [r, setR] = useState(revision);
  useEffect(() => {
    watchers.add(setR);
    return () => {
      watchers.delete(setR);
    };
  }, []);
  return r;
}

export function bffSession(): Promise<BffSession> {
  cached ??= fetch("/bff/session", { cache: "no-store" })
    .then((r) => r.json() as Promise<BffSession>)
    .catch(() => ({ dataSource: "mock", signedIn: false, role: null, membershipId: null, clientRole: null, serverNow: null }) as BffSession);
  return cached;
}

export function useBffSession(): BffSession | null {
  const [s, setS] = useState<BffSession | null>(null);
  useEffect(() => {
    let isLive = true;
    bffSession().then((v) => isLive && setS(v));
    return () => {
      isLive = false;
    };
  }, []);
  return s;
}

/** Reads an operation in API mode; returns the mock value in mock mode (and while the data source is unknown).
The fetch result is stored with the request it answers; `loading` is derived (no setState in the effect body). */
export function useOp<T, R>(...[operation, input, mock, map, enabled = true]: [...OpCall, mock: R, map: (data: T) => R, enabled?: boolean]): { data: R; loading: boolean; error: OpError | null; source: "mock" | "api" | null } {
  const session = useBffSession();
  const rev = useRevision();
  const request = `${operation}|${JSON.stringify(input)}|${String(rev)}`;
  const [state, setState] = useState<{ request: string | null; data: R; error: OpError | null }>({ request: null, data: mock, error: null });
  const isActive = session?.dataSource === "api" && enabled;
  // callers pass a new input object and an inline projection on every render: the read uses the latest of both, and
  // the request (operation, input and revision) decides when it runs again
  const read = useEffectEvent(() => callOp<T>(...([operation, input] as OpArgs<never>)).then(map));
  useEffect(() => {
    if (!isActive) return;
    let isLive = true;
    read()
      .then((data) => isLive && setState({ request, data, error: null }))
      .catch((error: unknown) => isLive && setState((s) => ({ request, data: s.data, error: error instanceof OpError ? error : null })));
    return () => {
      isLive = false;
    };
  }, [isActive, request]);
  if (session?.dataSource !== "api") return { data: mock, loading: false, error: null, source: session?.dataSource ?? null }; // live mock value
  return { data: state.data, loading: isActive && state.request !== request, error: state.request === request ? state.error : null, source: "api" };
}

/** The business clock for date ranges: the Core API clock in API mode (demo scenario clock), else the browser clock. */
export function useNow(): Date | null {
  const s = useBffSession();
  if (!s) return null;
  return s.serverNow ? new Date(s.serverNow) : new Date();
}
