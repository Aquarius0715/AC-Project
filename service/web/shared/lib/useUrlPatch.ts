"use client";

import { useCallback, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useReportNavPending } from "@ac/web/lib/navPending";

/** Replaces URL search params in place (API-mode screens keep their scope and selection in the URL); null or ""
removes a key. The server reads the new URL and re-renders. The patch runs in a transition and is reported as pending
until that render is on screen, so the app shell shows a progress line meanwhile (UI/UX loop R76). */
export function useUrlPatch(): (patch: Record<string, string | null>) => void {
  return useUrlPatchPending()[0];
}

/** useUrlPatch with this caller's pending state: `isPending` stays true until the server's new render is on screen — a
control can show the choice at once instead of waiting a second (UI/UX loop R74). */
export function useUrlPatchPending(): [(patch: Record<string, string | null>) => void, boolean] {
  const router = useRouter();
  const pathname = usePathname();
  const [isPending, startTransition] = useTransition();
  useReportNavPending(isPending);
  // stable across renders, so effects and callbacks that depend on it do not re-run on every render
  const run = useCallback((patch: Record<string, string | null>) => {
    const q = new URLSearchParams(location.search);
    for (const [k, v] of Object.entries(patch)) {
      if (v) q.set(k, v);
      else q.delete(k);
    }
    startTransition(() => { router.replace(q.size > 0 ? `${pathname}?${q}` : pathname, { scroll: false }); });
  }, [router, pathname]);
  return [run, isPending];
}
