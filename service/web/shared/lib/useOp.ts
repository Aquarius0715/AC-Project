"use client";

// Data-source switch for screens: in the Phase 1A demo (DATA_SOURCE=mock) screens keep their fixture data; with
// DATA_SOURCE=api the same screen reads the Core API through the BFF relay.
import { useEffect, useState } from "react";
import { callOp, OpError } from "@ac/web/lib/ops";

export type BffSession = { dataSource: "mock" | "api"; signedIn: boolean; role: string | null; membershipId: string | null; serverNow: string | null };

let cached: Promise<BffSession> | null = null;

// invalidation: writes bump the revision so every mounted useOp read refetches (IR71 lists simplified to "all")
let revision = 0;
const watchers = new Set<(r: number) => void>();
export function invalidate() {
  revision += 1;
  watchers.forEach((w) => w(revision));
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
    .catch(() => ({ dataSource: "mock", signedIn: false, role: null, membershipId: null, serverNow: null }) as BffSession);
  return cached;
}

export function useBffSession(): BffSession | null {
  const [s, setS] = useState<BffSession | null>(null);
  useEffect(() => {
    let live = true;
    bffSession().then((v) => live && setS(v));
    return () => {
      live = false;
    };
  }, []);
  return s;
}

/** Reads an operation in api mode; returns the mock value in mock mode (and while the data source is unknown).
 * The fetch result is stored with the request it answers; `loading` is derived (no setState in the effect body). */
export function useOp<T, R>(operation: string, input: unknown, mock: R, map: (data: T) => R, enabled = true): { data: R; loading: boolean; error: OpError | null; source: "mock" | "api" | null } {
  const session = useBffSession();
  const rev = useRevision();
  const key = JSON.stringify(input);
  const request = `${operation}|${key}|${rev}`;
  const [state, setState] = useState<{ request: string | null; data: R; error: OpError | null }>({ request: null, data: mock, error: null });
  const active = session?.dataSource === "api" && enabled;
  useEffect(() => {
    if (!active) return;
    let live = true;
    callOp<T>(operation, JSON.parse(key))
      .then((d) => live && setState({ request, data: map(d), error: null }))
      .catch((e: unknown) => live && setState((s) => ({ request, data: s.data, error: e instanceof OpError ? e : null })));
    return () => {
      live = false;
    };
    // map is a pure projection supplied inline by the caller; request covers operation, input and revision
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, request]);
  if (session?.dataSource !== "api") return { data: mock, loading: false, error: null, source: session?.dataSource ?? null }; // live mock value
  return { data: state.data, loading: active && state.request !== request, error: state.request === request ? state.error : null, source: "api" };
}

/** The business clock for date ranges: the Core API clock in api mode (demo scenario clock), else the browser clock. */
export function useNow(): Date | null {
  const s = useBffSession();
  if (!s) return null;
  return s.serverNow ? new Date(s.serverNow) : new Date();
}
